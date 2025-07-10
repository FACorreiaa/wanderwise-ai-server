// Package database internal/platform/database/db.go (or your db package path)
package database // Assuming package name is database

import (
	"context"
	"embed"
	"fmt"
	"log/slog" // Use slog
	"net/url"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // Required for postgres driver registration
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	uuid "github.com/vgarvardt/pgx-google-uuid/v5"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
)

//go:embed migrations
var migrationFS embed.FS

const defaultRetries = 5 // Renamed from retries to avoid conflict if needed elsewhere

type DatabaseConfig struct {
	ConnectionURL string
}

// WaitForDB waits for the database connection pool to be available..
func WaitForDB(ctx context.Context, pgpool *pgxpool.Pool, logger *slog.Logger) bool {
	maxAttempts := defaultRetries
	for attempts := 1; attempts <= maxAttempts; attempts++ {
		err := pgpool.Ping(ctx)
		if err == nil {
			logger.InfoContext(ctx, "Database connection successful")
			return true // Connection successful
		}

		waitDuration := time.Duration(attempts) * 200 * time.Millisecond // Increased base wait time slightly
		logger.WarnContext(ctx, "Database ping failed, retrying...",
			slog.Int("attempt", attempts),
			slog.Int("max_attempts", maxAttempts),
			slog.Duration("wait_duration", waitDuration),
			slog.String("error", err.Error()),
		)
		// Don't wait after the last attempt
		if attempts < maxAttempts {
			time.Sleep(waitDuration)
		}
	}
	logger.ErrorContext(ctx, "Database connection failed after multiple retries")
	return false // Failed to connect after retries
}

func RunMigrations(databaseURL string, logger *slog.Logger) error {
	logger.Info("Running database migrations...")

	entries, err := migrationFS.ReadDir(".")
	if err != nil {
		logger.Error("Failed to read embedded migrations directory", slog.Any("error", err))
		return fmt.Errorf("failed to read embedded migrations directory: %w", err)
	}
	if len(entries) == 0 {
		logger.Warn("No migration files found in embedded migrations directory")
	}
	for _, entry := range entries {
		logger.Info("Found embedded migration file", slog.String("name", entry.Name()))
	}

	sourceDriver, err := iofs.New(migrationFS, "migrations")
	if err != nil {
		logger.Error("Failed to create migration source driver", slog.Any("error", err))
		return fmt.Errorf("failed to create migration source driver: %w", err)
	}

	if !strings.HasPrefix(databaseURL, "postgres://") && !strings.HasPrefix(databaseURL, "postgresql://") {
		errMsg := "invalid database URL scheme for migrate, ensure it starts with postgresql://"
		logger.Error(errMsg, slog.String("url", databaseURL))
		return fmt.Errorf("%s", errMsg)
	}

	m, err := migrate.NewWithSourceInstance("iofs", sourceDriver, databaseURL)
	if err != nil {
		logger.Error("Failed to initialize migrate instance", slog.Any("error", err))
		return fmt.Errorf("failed to initialize migrate instance: %w", err)
	}

	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		logger.Error("Failed to apply migrations", slog.Any("error", err))
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	version, dirty, err := m.Version()
	if err != nil {
		logger.Warn("Could not determine migration version", slog.Any("error", err))
	} else if dirty {
		logger.Error("DATABASE MIGRATION STATE IS DIRTY!", slog.Uint64("version", uint64(version)))
	} else if err == migrate.ErrNoChange {
		logger.Info("No new migrations to apply.", slog.Uint64("current_version", uint64(version)))
	} else {
		logger.Info("Database migrations applied successfully.", slog.Uint64("new_version", uint64(version)))
	}

	srcErr, dbErr := m.Close()
	if srcErr != nil {
		logger.Warn("Error closing migration source", slog.Any("error", srcErr))
	}
	if dbErr != nil {
		logger.Warn("Error closing migration database connection", slog.Any("error", dbErr))
	}

	return nil
}

// NewDatabaseConfig generates the database connection URL from configuration.
func NewDatabaseConfig(cfg *config.Config, logger *slog.Logger) (*DatabaseConfig, error) {
	// Assume cfg is already loaded and valid if passed here
	if cfg == nil || cfg.Repositories.Postgres.Host == "" {
		errMsg := "Postgres configuration is missing or invalid"
		logger.Error(errMsg)
		return nil, fmt.Errorf("%s", errMsg)
	}

	// schema := os.Getenv("POSTGRES_SCHEMA") // Get schema if needed, maybe from cfg instead?
	// if schema != "" {
	// 	query.Add("search_path", schema)
	// }

	query := url.Values{}
	query.Set("sslmode", "disable") // Or get from config: cfg.Repositories.Postgres.SSLMode
	query.Set("timezone", "utc")

	connURL := url.URL{
		Scheme:   "postgresql", // Use postgresql:// for migrate compatibility
		User:     url.UserPassword(cfg.Repositories.Postgres.Username, cfg.Repositories.Postgres.Password),
		Host:     fmt.Sprintf("%s:%s", cfg.Repositories.Postgres.Host, cfg.Repositories.Postgres.Port),
		Path:     cfg.Repositories.Postgres.DB,
		RawQuery: query.Encode(),
	}

	connStr := connURL.String()
	// Avoid logging password in production logs if possible
	logger.Info("Database connection URL generated", slog.String("host", connURL.Host), slog.String("database", connURL.Path))
	// fmt.Printf("Connection URL: %s\n", connStr) // Keep for local dev if helpful

	return &DatabaseConfig{
		ConnectionURL: connStr,
	}, nil
}

// Init initializes the pgxpool connection pool.
func Init(connectionURL string, logger *slog.Logger) (*pgxpool.Pool, error) {
	logger.Info("Initializing database connection pool...")
	cfg, err := pgxpool.ParseConfig(connectionURL)
	if err != nil {
		logger.Error("Failed to parse database config", slog.Any("error", err))
		return nil, fmt.Errorf("failed parsing db config: %w", err)
	}

	// Register UUID type HandlerImpl after connecting
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		uuid.Register(conn.TypeMap())
		logger.DebugContext(ctx, "Registered UUID type HandlerImpl")
		return nil
	}

	// Consider adjusting pool settings from config
	// cfg.MaxConns = cfg.Repositories.Postgres.MaxConns
	// cfg.MinConns = cfg.Repositories.Postgres.MinConns
	// cfg.MaxConnLifetime = ...
	// cfg.MaxConnIdleTime = ...

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		logger.Error("Failed to create database connection pool", slog.Any("error", err))
		return nil, fmt.Errorf("failed creating db pool: %w", err)
	}

	logger.Info("Database connection pool initialized")
	return pool, nil
}
