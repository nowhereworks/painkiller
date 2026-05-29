package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

func main() {
	direction := flag.String("direction", "up", "up or down")
	steps := flag.Int("steps", 0, "number of steps (0 = all)")
	migrationsDir := flag.String("dir", "migrations", "path to migrations directory")
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://localhost:5432/painkiller?sslmode=disable"
	}

	if *direction == "up" {
		if err := runAppMigrations(dbURL, *migrationsDir, *direction, *steps); err != nil {
			log.Fatalf("application migration failed: %v", err)
		}
		if err := runRiverMigrations(context.Background(), dbURL, *direction, *steps); err != nil {
			log.Fatalf("river migration failed: %v", err)
		}
		return
	}

	if *direction == "down" {
		if err := runRiverMigrations(context.Background(), dbURL, *direction, *steps); err != nil {
			log.Fatalf("river migration failed: %v", err)
		}
		if err := runAppMigrations(dbURL, *migrationsDir, *direction, *steps); err != nil {
			log.Fatalf("application migration failed: %v", err)
		}
		return
	}

	log.Fatalf("invalid direction: %s", *direction)
}

func runAppMigrations(dbURL, migrationsDir, direction string, steps int) error {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create postgres driver: %w", err)
	}

	absDir, err := filepath.Abs(migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to resolve migrations path: %w", err)
	}

	fsys := os.DirFS(absDir)
	sourceDriver, err := iofs.New(fsys, ".")
	if err != nil {
		return fmt.Errorf("failed to create iofs driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", driver)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	switch direction {
	case "up":
		if steps > 0 {
			err = m.Steps(steps)
		} else {
			err = m.Up()
		}
	case "down":
		if steps > 0 {
			err = m.Steps(-steps)
		} else {
			err = m.Down()
		}
	default:
		return fmt.Errorf("invalid direction: %s", direction)
	}

	if err != nil && err != migrate.ErrNoChange {
		return err
	}

	if err == migrate.ErrNoChange {
		fmt.Println("application migrations: no changes")
	} else {
		fmt.Printf("application migration %s complete\n", direction)
	}

	return nil
}

func runRiverMigrations(ctx context.Context, dbURL, direction string, steps int) error {
	dbPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("failed to create pgx pool: %w", err)
	}
	defer dbPool.Close()

	migrator, err := rivermigrate.New(riverpgxv5.New(dbPool), nil)
	if err != nil {
		return fmt.Errorf("failed to create river migrator: %w", err)
	}

	migrateDirection := rivermigrate.DirectionUp
	migrateOpts := &rivermigrate.MigrateOpts{}
	if direction == "down" {
		migrateDirection = rivermigrate.DirectionDown
		if steps > 0 {
			migrateOpts.MaxSteps = steps
		} else {
			migrateOpts.TargetVersion = -1
		}
	} else if direction != "up" {
		return fmt.Errorf("invalid direction: %s", direction)
	}

	if direction == "up" && steps > 0 {
		migrateOpts.MaxSteps = steps
	}

	result, err := migrator.Migrate(ctx, migrateDirection, migrateOpts)
	if err != nil {
		return err
	}

	if len(result.Versions) == 0 {
		fmt.Println("river migrations: no changes")
	} else {
		fmt.Printf("river migration %s complete (%d versions)\n", direction, len(result.Versions))
	}

	return nil
}
