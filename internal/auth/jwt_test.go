package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGenerateAndValidateToken(t *testing.T) {
	mgr := NewJWTManager("test-secret-key", time.Hour)
	userID := uuid.New()

	token, err := mgr.GenerateToken(userID)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	got, err := mgr.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if got != userID {
		t.Errorf("expected user ID %s, got %s", userID, got)
	}
}

func TestValidateTokenExpired(t *testing.T) {
	mgr := NewJWTManager("test-secret-key", -time.Hour)
	userID := uuid.New()

	token, err := mgr.GenerateToken(userID)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	_, err = mgr.ValidateToken(token)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestValidateTokenWrongSecret(t *testing.T) {
	mgr1 := NewJWTManager("secret-one", time.Hour)
	mgr2 := NewJWTManager("secret-two", time.Hour)
	userID := uuid.New()

	token, err := mgr1.GenerateToken(userID)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	_, err = mgr2.ValidateToken(token)
	if err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestValidateTokenInvalid(t *testing.T) {
	mgr := NewJWTManager("test-secret-key", time.Hour)

	_, err := mgr.ValidateToken("not-a-valid-token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestGenerateTokenDifferentUsers(t *testing.T) {
	mgr := NewJWTManager("test-secret-key", time.Hour)
	user1 := uuid.New()
	user2 := uuid.New()

	token1, err := mgr.GenerateToken(user1)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	token2, err := mgr.GenerateToken(user2)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	if token1 == token2 {
		t.Error("tokens for different users should differ")
	}

	got1, _ := mgr.ValidateToken(token1)
	got2, _ := mgr.ValidateToken(token2)

	if got1 != user1 {
		t.Errorf("token1: expected %s, got %s", user1, got1)
	}
	if got2 != user2 {
		t.Errorf("token2: expected %s, got %s", user2, got2)
	}
}
