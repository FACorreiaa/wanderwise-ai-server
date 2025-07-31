package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	database "github.com/FACorreiaa/go-poi-au-suggestions/app/db"
	"github.com/FACorreiaa/go-poi-au-suggestions/app/logger"
	"github.com/FACorreiaa/go-poi-au-suggestions/app/observability/metrics"
	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/broker"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/clients"
)

func initializeLogger() error {
	return logger.Init(
		zap.DebugLevel,
		zap.String("service", "go-ai-poi"),
		zap.String("version", "v1.0.0"),
		zap.Strings("maintainers", []string{"@FACorreiaa"}),
	)
}

func setupDatabases(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	slogLogger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	dbConfig, err := database.NewDatabaseConfig(cfg, slogLogger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database configuration: %w", err)
	}

	pool, err := database.Init(dbConfig.ConnectionURL, slogLogger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database pool: %w", err)
	}

	database.WaitForDB(ctx, pool, slogLogger)
	logger.Log.Info("Connected to Postgres",
		zap.String("host", cfg.Repositories.Postgres.Host),
		zap.String("port", cfg.Repositories.Postgres.Port))

	if err = database.RunMigrations(dbConfig.ConnectionURL, slogLogger); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return pool, nil
}

func startServices(ctx context.Context, cfg *config.Config, app *internal.Application, reg *prometheus.Registry) error {
	errChan := make(chan error, 2)

	// Start gRPC server
	go func() {
		grpcPort := cfg.Server.GrpcPort
		if grpcPort == "" {
			grpcPort = os.Getenv("GRPC_PORT")
			if grpcPort == "" {
				grpcPort = "9000"
			}
		}

		if err := internal.ServeGRPC(ctx, grpcPort, app, reg); err != nil {
			logger.Log.Error("gRPC server error", zap.Error(err))
			errChan <- err
		}
	}()

	// Start HTTP server
	go func() {
		httpPort := cfg.Server.HTTPPort
		if httpPort == "" {
			httpPort = "8080"
		}

		if err := internal.ServeHTTP(httpPort, app, reg); err != nil {
			logger.Log.Error("HTTP server error", zap.Error(err))
			errChan <- err
		}
	}()

	// Wait for shutdown signal or error
	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func run(ctx context.Context, cfg *config.Config, reg *prometheus.Registry) (*internal.Application, error) {
	pool, err := setupDatabases(ctx, cfg)
	if err != nil {
		logger.Log.Error("failed to setup databases", zap.Error(err))
		return nil, err
	}

	// Initialize broker
	serviceBroker := broker.NewBroker(cfg, logger.Log, pool, reg)

	// Create and register all services
	serviceFactory := broker.NewServiceFactory(serviceBroker, cfg, logger.Log, pool)
	if err := serviceFactory.CreateAndRegisterServices(); err != nil {
		pool.Close()
		logger.Log.Error("failed to create services", zap.Error(err))
		return nil, err
	}

	// Start broker services
	if err := serviceBroker.Start(ctx); err != nil {
		pool.Close()
		logger.Log.Error("failed to start broker", zap.Error(err))
		return nil, err
	}

	// Initialize HTTP client for inter-service communication (if needed)
	httpClient := clients.NewHTTPClient(cfg, logger.Log)

	return &internal.Application{
		DB:         pool,
		Broker:     serviceBroker,
		HTTPClient: httpClient,
	}, nil
}

func main() {
	println("Go AI POI microservices platform starting...")
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Load .env file
	if err := godotenv.Load(); err != nil {
		println("Warning: .env file not found or error loading:", err.Error())
	}

	reg := prometheus.NewRegistry()
	println("Loaded prometheus registry")

	if err := initializeLogger(); err != nil {
		panic("failed to initialize logging")
	}

	cfg, err := config.InitConfig()
	if err != nil {
		logger.Log.Error("failed to initialize config", zap.Error(err))
		return
	}

	app, err := run(ctx, cfg, reg)
	if err != nil {
		logger.Log.Error("failed to run the application", zap.Error(err))
		return
	}
	defer func() {
		app.DB.Close()
		if err := app.Broker.Stop(); err != nil {
			logger.Log.Error("failed to stop broker", zap.Error(err))
		}
	}()

	// Initialize metrics
	metrics.InitAppMetrics()

	if err = startServices(ctx, cfg, app, reg); err != nil {
		logger.Log.Error("service error", zap.Error(err))
	}
}
