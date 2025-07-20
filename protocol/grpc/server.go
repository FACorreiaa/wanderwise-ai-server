package grpc

import (
	"fmt"
	"net"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware"
	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware/grpclog"
	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware/grpcprometheus"
	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware/grpcratelimit"
	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware/grpcrecovery"
	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware/grpcrequest"
	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware/grpcspan"

	//"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware/grpcspan"
	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware/session"
)

// BootstrapServer creates a gRPC server preconfigured with interceptors for
// tracing, Prometheus metrics, logging, rate limiting, etc.
func BootstrapServer(
	port string,
	log *zap.Logger,
	registry *prometheus.Registry,
	traceProvider trace.TracerProvider, // [currently not used directly, but available if needed]
	opts ...grpc.ServerOption,
) (*grpc.Server, net.Listener, error) {

	// Create a TCP listener on the specified port.
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create listener")
	}

	// Prometheus interceptors setup.
	promCollectors := grpcprometheus.NewPrometheusMetricsCollectors()
	if err := grpcprometheus.RegisterMetrics(registry, promCollectors); err != nil {
		return nil, nil, errors.Wrap(err, "failed to register Prometheus metrics")
	}
	_, promInterceptor, err := grpcprometheus.Interceptors(promCollectors)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create Prometheus interceptors")
	}

	// OpenTelemetry tracing stats handlers. (Span creation, context propagation)
	_, spanHandler := grpcspan.Handlers()

	// Additional interceptors:
	_, logInterceptor := grpclog.Interceptors(log)
	_, recoveryInterceptor := grpcrecovery.Interceptors(grpcrecovery.RegisterMetrics(registry))
	sessionInterceptor := session.InterceptorSession()
	requestIDInterceptor := grpcrequest.RequestIDMiddleware()

	// Simple rate limiter for demonstration (10 requests/sec, 20 burst).
	rateLimiter := grpcratelimit.NewRateLimiter(10, 20)

	// Base gRPC server options.
	serverOptions := []grpc.ServerOption{
		// Adjust keepalive.
		grpc.KeepaliveEnforcementPolicy(middleware.KeepaliveEnforcementPolicy()),
		grpc.KeepaliveParams(middleware.KeepAliveServerParams()),

		// Add OpenTelemetry stats handler (recommended approach)
		grpc.StatsHandler(spanHandler.Handler),

		// Chain all unary interceptors in an order that ensures correct context propagation.
		grpc.ChainUnaryInterceptor(
			promInterceptor.Unary,                // Prometheus
			logInterceptor.Unary,                 // Logging
			sessionInterceptor,                   // Session management
			requestIDInterceptor,                 // Request ID injection
			recoveryInterceptor.Unary,            // Recovery from panics
			rateLimiter.UnaryServerInterceptor(), // Basic rate limiting
		),

		// Chain all stream interceptors.
		grpc.ChainStreamInterceptor(
			promInterceptor.Stream,
			logInterceptor.Stream,
			recoveryInterceptor.Stream,
		),
	}

	// Include any additional options passed in.
	serverOptions = append(serverOptions, opts...)

	// Create the gRPC server.
	server := grpc.NewServer(serverOptions...)

	return server, listener, nil
}
