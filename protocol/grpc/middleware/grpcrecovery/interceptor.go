package grpcrecovery

import (
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware"
)

func RegisterMetrics(registry *prometheus.Registry) prometheus.Counter {
	return promauto.With(registry).NewCounter(prometheus.CounterOpts{
		Name: "grpc_req_panics_recovered_total",
		Help: "Total number of gRPC requests recovered from internal panic.",
	})
}

func Interceptors(panicMetric prometheus.Counter) (middleware.ClientInterceptor, middleware.ServerInterceptor) {
	recoveryHandler := func(p any) (err error) {
		panicMetric.Inc()
		return status.Errorf(codes.Internal, "%s", p)
	}

	// Note: Recovery interceptor only exist for server side
	clientInterceptor := middleware.ClientInterceptor{}
	serverInterceptor := middleware.ServerInterceptor{
		Unary:  recovery.UnaryServerInterceptor(recovery.WithRecoveryHandler(recoveryHandler)),
		Stream: recovery.StreamServerInterceptor(recovery.WithRecoveryHandler(recoveryHandler)),
	}

	return clientInterceptor, serverInterceptor
}
