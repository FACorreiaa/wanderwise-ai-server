package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	database "github.com/FACorreiaa/go-poi-au-suggestions/app/db"
	"github.com/FACorreiaa/go-poi-au-suggestions/app/observability/metrics"
	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/clients"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/container"
	"github.com/FACorreiaa/go-poi-au-suggestions/logger"
	grpcServer "github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc"
)

type Dependencies struct {
	DB              *pgxpool.Pool
	Container       *container.Container
	ServiceRegistry *clients.ServiceRegistry
	HTTPClient      *clients.HTTPClient
}

func initializeLogger() error {
	return logger.Init(
		zap.DebugLevel,
		zap.String("service", "go-ai-poi"),
		zap.String("version", "v1.0.0"),
		zap.Strings("maintainers", []string{"@FACorreiaa"}),
	)
}

func setupDatabases(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	// Create a temporary slog logger for database operations
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

	//// Initialize Redis
	//redisHost := cfg.Repositories.Redis.Host
	//if redisHost == "" {
	//	redisHost = os.Getenv("REDIS_HOST")
	//	if redisHost == "" {
	//		redisHost = "localhost"
	//	}
	//}
	//
	//redisPort := cfg.Repositories.Redis.Port
	//if redisPort == "" {
	//	redisPort = os.Getenv("REDIS_PORT")
	//	if redisPort == "" {
	//		redisPort = "6379"
	//	}
	//}
	//
	//redisPassword := cfg.Repositories.Redis.Password
	//if redisPassword == "" {
	//	redisPassword = os.Getenv("REDIS_PASSWORD")
	//}
	//
	//redisClient := redis.NewClient(&redis.Options{
	//	Addr:     fmt.Sprintf("%s:%s", redisHost, redisPort),
	//	Password: redisPassword,
	//	DB:       cfg.Repositories.Redis.DB,
	//})
	//
	//// Test Redis connection
	//_, err = redisClient.Ping(ctx).Result()
	//if err != nil {
	//	pool.Close()
	//	return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	//}

	//logger.Log.Info("Connected to Redis",
	//	zap.String("host", redisHost),
	//	zap.String("port", redisPort))

	if err = database.RunMigrations(dbConfig.ConnectionURL, slogLogger); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return pool, nil
}

func startServices(ctx context.Context, cfg *config.Config, deps *Dependencies, reg *prometheus.Registry) error {
	errChan := make(chan error, 2)

	// Start gRPC server
	go func() {
		grpcPort := cfg.Server.GrpcPort
		if grpcPort == "" {
			grpcPort = os.Getenv("GRPC_PORT")
			if grpcPort == "" {
				grpcPort = "9000" // default gRPC port
			}
		}

		// Get tracer provider from OpenTelemetry
		// traceProvider := otel.GetTracerProvider()

		server, listener, err := grpcServer.BootstrapServer(grpcPort, logger.Log, reg, nil)
		if err != nil {
			logger.Log.Error("Failed to bootstrap gRPC server", zap.Error(err))
			errChan <- err
			return
		}

		// Here you would register your gRPC services
		// Example: pb.RegisterYourServiceServer(server, yourServiceImpl)

		logger.Log.Info("Starting gRPC server", zap.String("port", grpcPort))
		if err := server.Serve(listener); err != nil {
			logger.Log.Error("gRPC server error", zap.Error(err))
			errChan <- err
		}
	}()

	// Start HTTP server (metrics, health checks, etc.)
	go func() {
		httpPort := cfg.Server.HTTPPort
		if httpPort == "" {
			httpPort = "8080" // default HTTP port
		}

		// Simple HTTP server for metrics and health
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte("OK"))
		})
		
		// Service registry endpoints
		mux.HandleFunc("/services", func(w http.ResponseWriter, r *http.Request) {
			services := deps.ServiceRegistry.GetAllServices()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(services)
		})
		
		mux.HandleFunc("/services/stats", func(w http.ResponseWriter, r *http.Request) {
			stats := deps.ServiceRegistry.GetServiceStats()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(stats)
		})
		
		mux.HandleFunc("/services/healthy", func(w http.ResponseWriter, r *http.Request) {
			services := deps.ServiceRegistry.GetHealthyServices()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(services)
		})

		logger.Log.Info("Starting HTTP server", zap.String("port", httpPort))
		if err := http.ListenAndServe(":"+httpPort, mux); err != nil {
			logger.Log.Error("HTTP server error", zap.Error(err))
			errChan <- err
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func run(ctx context.Context, cfg *config.Config) (*Dependencies, error) {
	pool, err := setupDatabases(ctx, cfg)
	if err != nil {
		logger.Log.Error("failed to setup databases", zap.Error(err))
		return nil, err
	}

	// For now, we'll create a simple slog adapter from zap
	// In a real microservices setup, you'd want to standardize on one logger
	// Create a minimal slog logger for container compatibility
	slogLogger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	c, err := container.NewContainer(cfg, slogLogger)
	if err != nil {
		pool.Close()
		logger.Log.Error("failed to create container", zap.Error(err))
		return nil, err
	}

	// Initialize service registry and HTTP client for inter-service communication
	serviceRegistry := clients.NewServiceRegistry(cfg, logger.Log)
	serviceRegistry.InitializeServices()
	
	httpClient := clients.NewHTTPClient(cfg, logger.Log)

	// Start health checks in the background
	go serviceRegistry.StartHealthChecks(ctx, 30*time.Second)

	return &Dependencies{
		DB:              pool,
		Container:       c,
		ServiceRegistry: serviceRegistry,
		HTTPClient:      httpClient,
	}, nil
}

func main() {
	println("Go AI POI microservices platform starting...")
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

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

	deps, err := run(ctx, cfg)
	if err != nil {
		logger.Log.Error("failed to run the application", zap.Error(err))
		return
	}

	// Initialize metrics
	metrics.InitAppMetrics()

	if err = startServices(ctx, cfg, deps, reg); err != nil {
		logger.Log.Error("service error", zap.Error(err))
	}

	deps.DB.Close()
}
