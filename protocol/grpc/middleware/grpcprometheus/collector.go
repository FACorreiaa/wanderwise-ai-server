package grpcprometheus

import (
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
)

type Collectors struct {
	Client *grpcprom.ClientMetrics
	Server *grpcprom.ServerMetrics
}

func NewPrometheusMetricsCollectors() *Collectors {
	return &Collectors{
		Client: clientMetrics(),
		Server: serverMetrics(),
	}
}

// RegisterMetrics must be called before the Prometheus interceptor is used.
func RegisterMetrics(registry *prometheus.Registry, collectors *Collectors) error {
	if registry == nil {
		return errors.New("must provide a Prometheus registry")
	}

	if collectors == nil {
		return errors.New("must provide Prometheus collectors")
	}

	if collectors.Client != nil {
		registry.MustRegister(collectors.Client)
	}

	if collectors.Server != nil {
		registry.MustRegister(collectors.Server)
	}

	return nil
}

// grpcHandlingTimeHistogramBuckets is the default set of buckets used by both
// server and client histograms.
var grpcHandlingTimeHistogramBuckets = []float64{
	0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
}

func clientMetrics() *grpcprom.ClientMetrics {
	return grpcprom.NewClientMetrics(
		grpcprom.WithClientHandlingTimeHistogram(
			grpcprom.WithHistogramBuckets(grpcHandlingTimeHistogramBuckets),
		),
	)
}

func serverMetrics() *grpcprom.ServerMetrics {
	return grpcprom.NewServerMetrics(
		grpcprom.WithServerHandlingTimeHistogram(
			grpcprom.WithHistogramBuckets(grpcHandlingTimeHistogramBuckets),
		),
	)
}
