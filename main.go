package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // Import pprof
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/markbates/goth"
	"github.com/markbates/goth/providers/google"

	"github.com/go-chi/httprate"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "github.com/FACorreiaa/go-poi-au-suggestions/docs" // Import for swagger docs

	"github.com/FACorreiaa/go-poi-au-suggestions/app/observability/metrics"
	"github.com/FACorreiaa/go-poi-au-suggestions/app/observability/tracer"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/auth"
	router "github.com/FACorreiaa/go-poi-au-suggestions/internal/router"

	database "github.com/FACorreiaa/go-poi-au-suggestions/app/db"
	l "github.com/FACorreiaa/go-poi-au-suggestions/app/logger"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/container"
)

// @title           Loci API
// @version         1.0
// @description     API for personalized city discovery and recommendations.
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8000 // Adjust to your actual host/port
// @BasePath  /api/v1        // Base path for all API routes

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

func init() {
	goth.UseProviders(
		google.New(
			os.Getenv("GOOGLE_CLIENT_ID"),
			os.Getenv("GOOGLE_CLIENT_SECRET"),
			os.Getenv("GOOGLE_CALLBACK_URL"),
		),
	)
}

// setupPprof initializes the pprof profiling server
func setupPprof(logger *slog.Logger) {
	pprofEnabled := os.Getenv("ENABLE_PPROF")
	if pprofEnabled == "true" || pprofEnabled == "1" {
		pprofPort := os.Getenv("PPROF_PORT")
		if pprofPort == "" {
			pprofPort = "6060"
		}

		logger.Info("🔬 Starting pprof server", slog.String("port", pprofPort))
		logger.Info("📊 pprof endpoints available:",
			slog.String("index", fmt.Sprintf("http://localhost:%s/debug/pprof/", pprofPort)),
			slog.String("heap", fmt.Sprintf("http://localhost:%s/debug/pprof/heap", pprofPort)),
			slog.String("goroutine", fmt.Sprintf("http://localhost:%s/debug/pprof/goroutine", pprofPort)),
			slog.String("profile", fmt.Sprintf("http://localhost:%s/debug/pprof/profile", pprofPort)),
			slog.String("trace", fmt.Sprintf("http://localhost:%s/debug/pprof/trace", pprofPort)))

		go func() {
			pprofServer := &http.Server{
				Addr:    fmt.Sprintf(":%s", pprofPort),
				Handler: http.DefaultServeMux, // pprof registers itself on DefaultServeMux
			}

			if err := pprofServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("pprof server failed", slog.Any("error", err))
			}
		}()
	} else {
		logger.Info("pprof disabled. Set ENABLE_PPROF=true to enable profiling")
	}
}

func main() {
	// --- Initial Loading ---
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found or error loading:", err)
	}
	cfg, err := config.InitConfig()
	if err != nil {
		log.Fatalf("FATAL: Error initializing config: %v", err)
	}

	// --- Logger Setup ---
	logger := setupLogger()
	slog.SetDefault(logger)

	// --- Initialize Tracing and Metrics ---
	// change port
	otelShutdown, err := tracer.InitOtelProviders("Loci", ":9090")
	if err != nil {
		logger.Error("Failed to initialize OpenTelemetry providers", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		// Shutdown OTel at the very end
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			logger.Error("OpenTelemetry shutdown failed", slog.Any("error", err))
		} else {
			logger.Info("OpenTelemetry shut down successfully.")
		}
	}()

	metrics.InitAppMetrics()

	// --- pprof Setup ---
	setupPprof(logger)

	// --- Application Context & Shutdown ---
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel() // Ensure cancel is called eventually

	// --- Initialize Container ---
	c, err := container.NewContainer(cfg, logger)
	if err != nil {
		logger.Error("Failed to initialize container", slog.Any("error", err))
		os.Exit(1)
	}
	defer c.Close()

	// --- Run Migrations ---
	dbConfig, err := database.NewDatabaseConfig(cfg, logger)
	if err != nil {
		logger.Error("Failed to generate database config", slog.Any("error", err))
		os.Exit(1)
	}

	// --- Wait for Database ---
	if !c.WaitForDB(ctx) {
		logger.Error("Database not ready after waiting, exiting.")
		os.Exit(1)
	}

	err = c.RunMigrations(dbConfig.ConnectionURL)
	if err != nil {
		logger.Error("Failed to run database migrations", slog.Any("error", err))
		os.Exit(1)
	}

	authenticateMiddleware := auth.Authenticate(logger, cfg.JWT)
	// --- Router Setup ---
	routerConfig := &router.Config{
		AuthHandler:             c.AuthHandler,
		UserHandler:             c.UserHandler,
		InterestHandler:         c.InterestHandler,
		SearchProfileHandler:    c.SearchProfileHandler,
		TagsHandler:             c.TagsHandler,
		LLMInteractionHandler:   c.LLMInteractionHandlerImpl,
		PointsOfInterestHandler: c.POIHandler,
		ItineraryListHandler:    c.ItineraryListHandler,
		CityHandler:             c.CityHandler,
		RecentsHandler:          c.RecentsHandler,
		StatisticsHandler:       c.StatisticsHandler,
		AuthenticateMiddleware:  authenticateMiddleware,
		RateLimiter:             c.RateLimiter,
		Logger:                  logger,
	}
	apiRouter := router.SetupRouter(routerConfig)

	// --- Server-Wide Middleware Setup ---
	r := chi.NewMux()
	//r.Use(authenticateMiddleware)
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(l.StructuredLogger(logger))
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.StripSlashes)
	r.Use(chiMiddleware.Timeout(60 * time.Second))
	r.Use(chiMiddleware.Compress(5, "application/json"))
	r.Use(httprate.LimitByIP(100, time.Minute))

	r.Get("/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte("pong"))
		if err != nil {
			return
		}
	})
	r.Get("/swagger/*", httpSwagger.WrapHandler)
	r.Mount("/", apiRouter)

	// --- HTTP Server Setup ---
	serverAddress := fmt.Sprintf(":%s", cfg.Server.HTTPPort)
	srv := &http.Server{
		Addr:         serverAddress,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
		ErrorLog:     slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	// --- Start Server Goroutine & Graceful Shutdown ---
	go func() {
		logger.Info("Starting HTTP server", slog.String("address", serverAddress))
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server ListenAndServe error", slog.Any("error", err))
			cancel()
		}
	}()

	<-ctx.Done()

	logger.Info("Shutdown signal received, starting graceful shutdown...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server graceful shutdown failed", slog.Any("error", err))
	} else {
		logger.Info("HTTP server gracefully stopped")
	}

	logger.Info("Application shut down complete.")
}

func setupLogger() *slog.Logger {
	var logger *slog.Logger
	env := os.Getenv("APP_ENV")
	if env == "development" || env == "" {
		tintOpts := &tint.Options{
			Level:      slog.LevelDebug,
			TimeFormat: time.Kitchen,
			AddSource:  true,
			NoColor:    false,
		}
		logger = slog.New(tint.NewHandler(os.Stdout, tintOpts))
		log.Println("Initialized development logger (tint)")
	} else {
		jsonOpts := &slog.HandlerOptions{
			Level:     slog.LevelInfo,
			AddSource: false,
		}
		logger = slog.New(slog.NewJSONHandler(os.Stdout, jsonOpts))
		log.Println("Initialized production logger (JSON)")
	}
	return logger
}
