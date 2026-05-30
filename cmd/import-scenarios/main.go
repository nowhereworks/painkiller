package main

import (
	"context"
	"log"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"painkiller-shell/internal/config"
	"painkiller-shell/internal/importer"
	applog "painkiller-shell/internal/log"
	"painkiller-shell/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	logger := applog.New(cfg.LogLevel)

	if cfg.ScenarioRepoPath == "" {
		log.Fatalf("SCENARIO_REPO_PATH is not set")
	}

	db, err := sqlx.Connect("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	dataStore := store.New(db)

	if err := importer.ImportAll(context.Background(), dataStore, cfg.ScenarioRepoPath, logger); err != nil {
		log.Fatalf("import failed: %v", err)
	}

	logger.Info("import complete")
}
