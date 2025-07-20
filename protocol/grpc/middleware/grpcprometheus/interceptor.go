package grpcprometheus

import (
	"context"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"

	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware"
)

func Interceptors(collectors *Collectors) (*middleware.ClientInterceptor, *middleware.ServerInterceptor, error) {
	if collectors == nil {
		return nil, nil, errors.New("must provide Prometheus collectors")
	}

	var (
		clientInterceptor *middleware.ClientInterceptor
		serverInterceptor *middleware.ServerInterceptor
	)

	// Note: We're set up to collect on clients but it comes with a caveat that only one instance
	// of the collector can be registered by prometheus.
	//
	// i.e. if we have more than one upstream in a service, we'll panic at launch due to prom
	// refusing to overwrite collectors.
	//
	// We include the setup here for completion, but in practice, we're only collecting metrics
	// on the server side. If you 100% need point-to-point metrics, then manually set collectors
	//
	// see: https://github.com/grpc-ecosystem/go-grpc-middleware/blob/9e92fd5063c9cd61a08d6723aa7f2af0e73de07e/providers/prometheus/client_metrics.go#L34
	cc := collectors.Client
	if cc != nil {
		clientInterceptor = &middleware.ClientInterceptor{
			Unary:  cc.UnaryClientInterceptor(grpcprom.WithExemplarFromContext(contextExemplar)),
			Stream: cc.StreamClientInterceptor(grpcprom.WithExemplarFromContext(contextExemplar)),
		}
	}

	sc := collectors.Server
	if sc != nil {
		serverInterceptor = &middleware.ServerInterceptor{
			Unary:  sc.UnaryServerInterceptor(grpcprom.WithExemplarFromContext(contextExemplar)),
			Stream: sc.StreamServerInterceptor(grpcprom.WithExemplarFromContext(contextExemplar)),
		}
	}

	return clientInterceptor, serverInterceptor, nil
}

// contextExemplar returns a set of labels for the given context.
func contextExemplar(ctx context.Context) prometheus.Labels {
	if span := trace.SpanContextFromContext(ctx); span.IsSampled() {
		return prometheus.Labels{"traceID": span.TraceID().String()}
	}
	return nil
}
