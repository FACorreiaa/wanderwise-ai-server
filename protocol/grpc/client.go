package grpc

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware/grpclog"
	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware/grpcspan"
)

// BootstrapClient creates a gRPC client connection with basic OTel + logging interceptors.
func BootstrapClient(
	address string,
	log *zap.Logger,
	traceProvider trace.TracerProvider, // [currently not used directly, but available if needed]
	promRegistry *prometheus.Registry, // [not used, but can be integrated if you'd like client metrics]
	opts ...grpc.DialOption,
) (*grpc.ClientConn, error) {

	// OpenTelemetry and logging setup.
	spanHandler, _ := grpcspan.Handlers()
	logInterceptor, _ := grpclog.Interceptors(log)

	// Base gRPC dial options.
	connOptions := []grpc.DialOption{
		// For local dev/demo, we use insecure. For production, use TLS.
		grpc.WithTransportCredentials(insecure.NewCredentials()),

		// Basic load balancing config (round_robin).
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),

		// Add OpenTelemetry stats handler (recommended approach)
		grpc.WithStatsHandler(spanHandler.Handler),

		// Chain the unary interceptors.
		grpc.WithChainUnaryInterceptor(
			logInterceptor.Unary,
		),

		// Chain the stream interceptors.
		grpc.WithChainStreamInterceptor(
			logInterceptor.Stream,
		),
	}

	// Append any additional dial options.
	connOptions = append(connOptions, opts...)

	return grpc.NewClient(address, connOptions...)
}
