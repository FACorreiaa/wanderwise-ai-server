package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"

	pba "github.com/FACorreiaa/loci-proto/modules/auth/generated"
	pbc "github.com/FACorreiaa/loci-proto/modules/city/generated"
	pbch "github.com/FACorreiaa/loci-proto/modules/chat/generated"
	pbi "github.com/FACorreiaa/loci-proto/modules/interests/generated"
	pbl "github.com/FACorreiaa/loci-proto/modules/list/generated"
	pbp "github.com/FACorreiaa/loci-proto/modules/profiles/generated"
	pbt "github.com/FACorreiaa/loci-proto/modules/tags/generated"
	pbu "github.com/FACorreiaa/loci-proto/modules/user/generated"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/FACorreiaa/go-poi-au-suggestions/app/logger"
	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/broker"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/clients"
	grpcServer "github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc"
	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware/grpctracing"
)

// Application struct holds the core application components
type Application struct {
	DB         *pgxpool.Pool
	Broker     *broker.Broker
	HTTPClient *clients.HTTPClient
}

// isReady is used for kube liveness probes, it's only latched to true once
// the gRPC server is ready to handle requests
var isReady atomic.Value

// ServeGRPC starts the gRPC server with auth service registration
func ServeGRPC(ctx context.Context, port string, app *Application, reg *prometheus.Registry) error {
	log := logger.Log

	err := grpctracing.InitOTELToCollector(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to configure OpenTelemetry trace provider")
	}

	tp := otel.GetTracerProvider()

	server, listener, err := grpcServer.BootstrapServer(port, log, reg, tp)
	if err != nil {
		return errors.Wrap(err, "failed to configure gRPC server")
	}

	// Enable gRPC reflection for easier debugging
	reflection.Register(server)

	// Register services with gRPC server
	if err := registerAuthService(server, app.Broker, log); err != nil {
		log.Error("failed to register auth service", zap.Error(err))
		return err
	}

	if err := registerUserService(server, app.Broker, log); err != nil {
		log.Error("failed to register user service", zap.Error(err))
		return err
	}

	if err := registerTagsService(server, app.Broker, log); err != nil {
		log.Error("failed to register tags service", zap.Error(err))
		return err
	}

	if err := registerInterestsService(server, app.Broker, log); err != nil {
		log.Error("failed to register interests service", zap.Error(err))
		return err
	}

	if err := registerProfilesService(server, app.Broker, log); err != nil {
		log.Error("failed to register profiles service", zap.Error(err))
		return err
	}

	if err := registerCityService(server, app.Broker, log); err != nil {
		log.Error("failed to register city service", zap.Error(err))
		return err
	}

	if err := registerListsService(server, app.Broker, log); err != nil {
		log.Error("failed to register lists service", zap.Error(err))
		return err
	}

	if err := registerChatPromptService(server, app.Broker, log); err != nil {
		log.Error("failed to register chat prompt service", zap.Error(err))
		return err
	}

	// Handle graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			log.Warn("shutting down grpc server")
			server.GracefulStop()
			<-ctx.Done()
		}
	}()

	// Start serving
	log.Info("gRPC server starting", zap.String("port", port))
	isReady.Store(true)

	if err = server.Serve(listener); err != nil {
		return errors.Wrap(err, "gRPC server failed to serve")
	}

	return nil
}

// ServeHTTP creates a simple server to serve Prometheus metrics, health checks, and broker endpoints
func ServeHTTP(port string, app *Application, reg *prometheus.Registry) error {
	log := logger.Log
	log.Info("HTTP server starting", zap.String("port", port))

	cfg, err := config.InitConfig()
	if err != nil {
		log.Error("failed to initialize config", zap.Error(err))
		return err
	}

	mux := http.NewServeMux()

	// metrics.InitPprof(mux) // TODO: Add proper metrics import

	// Health check endpoints
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		// Check if gRPC server is ready
		if isReady.Load() == true {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Ready"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Not Ready"))
		}
	})

	// Prometheus metrics
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{EnableOpenMetrics: true}))

	// Broker service endpoints
	mux.HandleFunc("/broker/services", func(w http.ResponseWriter, r *http.Request) {
		services := app.Broker.GetAllServices()
		serviceTypes := make([]string, 0, len(services))
		for serviceType := range services {
			serviceTypes = append(serviceTypes, string(serviceType))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"services": serviceTypes,
			"count":    len(serviceTypes),
		})
	})

	mux.HandleFunc("/broker/health", func(w http.ResponseWriter, r *http.Request) {
		healthCheck := app.Broker.HealthCheck()
		healthStatus := make(map[string]string)
		allHealthy := true

		for serviceType, err := range healthCheck {
			if err != nil {
				healthStatus[string(serviceType)] = "unhealthy: " + err.Error()
				allHealthy = false
			} else {
				healthStatus[string(serviceType)] = "healthy"
			}
		}

		w.Header().Set("Content-Type", "application/json")
		status := "healthy"
		if !allHealthy {
			status = "degraded"
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   status,
			"services": healthStatus,
		})
	})

	server := &http.Server{
		Addr:              fmt.Sprintf(":%s", port),
		ReadHeaderTimeout: cfg.Server.Timeout,
		Handler:           mux,
	}

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return errors.Wrap(err, "failed to create HTTP server")
	}

	return nil
}

func registerAuthService(server *grpc.Server, brokerInstance *broker.Broker, log *zap.Logger) error {
	service, exists := brokerInstance.GetService(broker.AuthService)
	if !exists {
		log.Error("Auth service not found in broker")
		return errors.New("auth service not found in broker")
	}

	authImpl, ok := service.(*broker.AuthServiceImpl)
	if !ok {
		log.Error("Failed to cast auth service to AuthServiceImpl")
		return errors.New("failed to cast auth service")
	}

	pba.RegisterAuthServiceServer(server, authImpl.GetGRPCService())
	log.Info("Auth service registered with gRPC server")
	return nil
}

func registerUserService(server *grpc.Server, brokerInstance *broker.Broker, log *zap.Logger) error {
	service, exists := brokerInstance.GetService(broker.UserService)
	if !exists {
		log.Error("User service not found in broker")
		return errors.New("user service not found in broker")
	}

	userImpl, ok := service.(*broker.UserServiceImpl)
	if !ok {
		log.Error("Failed to cast user service to UserServiceImpl")
		return errors.New("failed to cast user service")
	}

	pbu.RegisterUserServiceServer(server, userImpl.GetGRPCService())
	log.Info("User service registered with gRPC server")
	return nil
}

func registerTagsService(server *grpc.Server, brokerInstance *broker.Broker, log *zap.Logger) error {
	service, exists := brokerInstance.GetService(broker.TagsService)
	if !exists {
		log.Error("Tags service not found in broker")
		return errors.New("tags service not found in broker")
	}

	tagsImpl, ok := service.(*broker.TagServiceImpl)
	if !ok {
		log.Error("Failed to cast tags service to TagsServiceImpl")
		return errors.New("failed to cast tags service")
	}

	pbt.RegisterTagsServiceServer(server, tagsImpl.GetGRPCService())
	log.Info("Tags service registered with gRPC server")
	return nil
}

func registerInterestsService(server *grpc.Server, brokerInstance *broker.Broker, log *zap.Logger) error {
	service, exists := brokerInstance.GetService(broker.InterestsService)
	if !exists {
		log.Error("Interests service not found in broker")
		return errors.New("interests service not found in broker")
	}

	interestsImpl, ok := service.(*broker.InterestsServiceImpl)
	if !ok {
		log.Error("Failed to cast interests service to InterestsServiceImpl")
		return errors.New("failed to cast interests service")
	}

	pbi.RegisterInterestsServiceServer(server, interestsImpl.GetGRPCService())
	log.Info("Interests service registered with gRPC server")
	return nil
}

func registerProfilesService(server *grpc.Server, brokerInstance *broker.Broker, log *zap.Logger) error {
	service, exists := brokerInstance.GetService(broker.ProfilesService)
	if !exists {
		log.Error("Profiles service not found in broker")
		return errors.New("profiles service not found in broker")
	}

	profilesImpl, ok := service.(*broker.ProfilesServiceImpl)
	if !ok {
		log.Error("Failed to cast profiles service to ProfilesServiceImpl")
		return errors.New("failed to cast profiles service")
	}

	pbp.RegisterProfilesServiceServer(server, profilesImpl.GetGRPCService())
	log.Info("Profiles service registered with gRPC server")
	return nil
}

func registerCityService(server *grpc.Server, brokerInstance *broker.Broker, log *zap.Logger) error {
	service, exists := brokerInstance.GetService(broker.CityService)
	if !exists {
		log.Error("City service not found in broker")
		return errors.New("city service not found in broker")
	}

	cityImpl, ok := service.(*broker.CityServiceImpl)
	if !ok {
		log.Error("Failed to cast city service to CityServiceImpl")
		return errors.New("failed to cast city service")
	}

	pbc.RegisterCityServiceServer(server, cityImpl.GetGRPCService())
	log.Info("City service registered with gRPC server")
	return nil
}

func registerListsService(server *grpc.Server, brokerInstance *broker.Broker, log *zap.Logger) error {
	service, exists := brokerInstance.GetService(broker.ListsService)
	if !exists {
		log.Error("Lists service not found in broker")
		return errors.New("lists service not found in broker")
	}

	listsImpl, ok := service.(*broker.ListsServiceImpl)
	if !ok {
		log.Error("Failed to cast lists service to ListsServiceImpl")
		return errors.New("failed to cast lists service")
	}

	pbl.RegisterListServiceServer(server, listsImpl.GetGRPCService())
	log.Info("Lists service registered with gRPC server")
	return nil
}

func registerChatPromptService(server *grpc.Server, brokerInstance *broker.Broker, log *zap.Logger) error {
	service, exists := brokerInstance.GetService(broker.ChatPromptService)
	if !exists {
		log.Error("Chat prompt service not found in broker")
		return errors.New("chat prompt service not found in broker")
	}

	chatPromptImpl, ok := service.(*broker.ChatPromptServiceImpl)
	if !ok {
		log.Error("Failed to cast chat prompt service to ChatPromptServiceImpl")
		return errors.New("failed to cast chat prompt service")
	}

	pbch.RegisterChatServiceServer(server, chatPromptImpl.GetGRPCService())
	log.Info("Chat prompt service registered with gRPC server")
	return nil
}
