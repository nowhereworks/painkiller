package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"painkiller-shell/internal/admin"
	"painkiller-shell/internal/attempts"
	"painkiller-shell/internal/auth"
	"painkiller-shell/internal/billing"
	"painkiller-shell/internal/catalog"
	"painkiller-shell/internal/config"
	"painkiller-shell/internal/entitlements"
	"painkiller-shell/internal/grading"
	"painkiller-shell/internal/httpx"
	"painkiller-shell/internal/jobs"
	applog "painkiller-shell/internal/log"
	"painkiller-shell/internal/orchestrator"
	"painkiller-shell/internal/provider/mock"
	"painkiller-shell/internal/provisioner/ansible"
	"painkiller-shell/internal/scoring"
	"painkiller-shell/internal/store"
	"painkiller-shell/internal/terminal"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	logger := applog.New(cfg.LogLevel)

	db, err := sqlx.Connect("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	pgxPool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to create pgx pool: %v", err)
	}
	defer pgxPool.Close()

	dataStore := store.New(db)

	jwtManager := auth.NewJWTManager(cfg.JWTSecret, cfg.JWTExpiry)
	authService := auth.NewService(dataStore, jwtManager)
	authHandler := auth.NewHandler(authService, jwtManager)

	billingService := billing.NewService(dataStore, cfg.StripeSecretKey, cfg.StripeSuccessURL, cfg.StripeCancelURL)
	webhookHandler := billing.NewWebhookHandler(billingService, cfg.StripeWebhookSecret, logger)
	billingHandler := billing.NewHandler(billingService, webhookHandler)

	catalogHandler := catalog.NewHandler(dataStore)
	entitlementsHandler := entitlements.NewHandler(dataStore)

	queue, err := jobs.NewQueue(jobs.QueueConfig{
		DBPool: pgxPool,
		Logger: logger,
	})
	if err != nil {
		log.Fatalf("failed to create job queue: %v", err)
	}

	attemptsService := attempts.NewService(dataStore, queue)
	attemptsHandler := attempts.NewHandler(attemptsService)

	provider := mock.New(100 * time.Millisecond)
	provisioner := ansible.New(logger)

	orch := orchestrator.New(orchestrator.OrchestratorConfig{
		Provider:    provider,
		Provisioner: provisioner,
		Store:       dataStore,
		Queue:       queue,
		Attempts:    attemptsService,
		Logger:      logger,
	})

	gradingEngine := grading.NewEngine(dataStore, attemptsService, queue, logger)
	orch.Worker().RegisterHandler(jobs.JobKindGradeAttempt, gradingEngine.GradeAttempt)
	orch.Worker().RegisterHandler(jobs.JobKindExpireAttempt, attemptsService.HandleExpireAttempt)

	terminalGateway := terminal.NewGateway(dataStore, attemptsService, logger)
	scoringHandler := scoring.NewHandler(dataStore)
	adminHandler := admin.NewHandler(dataStore, attemptsService, queue)

	reconciler := orchestrator.NewReconciler(orch, 60*time.Second)

	srv := httpx.NewServer(cfg.HTTPAddr, logger)

	srv.Router().Route("/api/v1", func(r chi.Router) {
		r.Route("/auth", authHandler.RegisterRoutes)
		r.Post("/webhooks/stripe", webhookHandler.Handle)
		r.Route("/catalog", catalogHandler.RegisterRoutes)

		r.Group(func(r chi.Router) {
			r.Use(jwtManager.Middleware)
			r.Route("/billing", billingHandler.RegisterRoutes)
			r.Route("/entitlements", entitlementsHandler.RegisterRoutes)
			r.Route("/attempts", attemptsHandler.RegisterRoutes)
			r.Route("/scoring", scoringHandler.RegisterRoutes)
			r.Route("/terminal", terminalGateway.RegisterRoutes)
			r.Route("/admin", func(r chi.Router) {
				r.Use(admin.Middleware(dataStore))
				adminHandler.RegisterRoutes(r)
			})
		})
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := queue.Start(ctx); err != nil {
		log.Fatalf("failed to start job queue: %v", err)
	}

	go reconciler.Run(ctx)

	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("received shutdown signal")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := queue.Stop(); err != nil {
		logger.Error("job queue shutdown error", "error", err)
	}

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}
