package grpcspan

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware"
)

// Handlers wraps OpenTelemetry gRPC stats handlers (recommended).
// We use a wrapper for this as the OpenTelemetry ecosystem changes
// *very* frequently, so we want to contain this change to a single point
// rather than having to update the servers and clients individually.
func Handlers() (middleware.ClientHandler, middleware.ServerHandler) {
	return middleware.ClientHandler{
		Handler: otelgrpc.NewClientHandler(),
	}, middleware.ServerHandler{
		Handler: otelgrpc.NewServerHandler(),
	}
}

// Deprecated: Use Handlers() instead
// Interceptors wraps OpenTelemetry gRPC interceptors (deprecated).
// The interceptor functions no longer exist in otelgrpc v0.62.0+
func Interceptors() (middleware.ClientInterceptor, middleware.ServerInterceptor) {
	// Return empty interceptors since the modern approach uses stats handlers
	return middleware.ClientInterceptor{}, middleware.ServerInterceptor{}
}

// NewClientHandler creates OpenTelemetry client stats handler with options
func NewClientHandler(opts ...otelgrpc.Option) middleware.ClientHandler {
	return middleware.ClientHandler{
		Handler: otelgrpc.NewClientHandler(opts...),
	}
}

// NewServerHandler creates OpenTelemetry server stats handler with options
func NewServerHandler(opts ...otelgrpc.Option) middleware.ServerHandler {
	return middleware.ServerHandler{
		Handler: otelgrpc.NewServerHandler(opts...),
	}
}
