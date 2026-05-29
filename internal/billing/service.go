package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"
	"painkiller-shell/internal/models"
	"painkiller-shell/internal/store"
)

type Service struct {
	store       *store.Store
	stripeKey   string
	successURL  string
	cancelURL   string
}

func NewService(store *store.Store, stripeKey, successURL, cancelURL string) *Service {
	stripe.Key = stripeKey
	return &Service{
		store:      store,
		stripeKey:  stripeKey,
		successURL: successURL,
		cancelURL:  cancelURL,
	}
}

func (s *Service) CreateCheckoutSession(ctx context.Context, userID uuid.UUID, testID uuid.UUID) (string, error) {
	test, err := s.store.Tests().GetByID(ctx, testID)
	if err != nil {
		return "", fmt.Errorf("test not found: %w", err)
	}

	product, err := s.store.Products().GetByID(ctx, test.ProductID)
	if err != nil {
		return "", fmt.Errorf("product not found: %w", err)
	}

	params := &stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModePayment)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(product.StripePriceID),
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL: stripe.String(s.successURL),
		CancelURL:  stripe.String(s.cancelURL),
		Metadata: map[string]string{
			"user_id": userID.String(),
			"test_id": testID.String(),
		},
	}

	sess, err := session.New(params)
	if err != nil {
		return "", fmt.Errorf("failed to create checkout session: %w", err)
	}

	return sess.URL, nil
}

func (s *Service) HandleCheckoutCompleted(ctx context.Context, sess *stripe.CheckoutSession) error {
	userIDStr, ok := sess.Metadata["user_id"]
	if !ok {
		return fmt.Errorf("missing user_id in metadata")
	}
	testIDStr, ok := sess.Metadata["test_id"]
	if !ok {
		return fmt.Errorf("missing test_id in metadata")
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}
	testID, err := uuid.Parse(testIDStr)
	if err != nil {
		return fmt.Errorf("invalid test_id: %w", err)
	}

	test, err := s.store.Tests().GetByID(ctx, testID)
	if err != nil {
		return fmt.Errorf("test not found: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(test.AccessWindowHours) * time.Hour)

	purchase := &models.PurchasedTest{
		ID:                uuid.New(),
		UserID:            userID,
		TestID:            testID,
		StripeSessionID:   sess.ID,
		ExpiresAt:         expiresAt,
		AttemptsRemaining: test.AttemptsAllowed,
		CreatedAt:         time.Now(),
	}

	if err := s.store.Purchases().Create(ctx, purchase); err != nil {
		return fmt.Errorf("failed to create purchase: %w", err)
	}

	return nil
}
