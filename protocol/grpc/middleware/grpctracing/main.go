package grpctracing

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"

	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/FACorreiaa/go-poi-au-suggestions/logger"
)

//func newTracerProvider(endpoint, apiKey, caCertPath string, insecure bool) (*trace.TracerProvider, error) {
//	var opts []otlptracegrpc.Option
//
//	// Set endpoint
//	opts = append(opts, otlptracegrpc.WithEndpoint(endpoint))
//	//opts = append(opts, otlptracegrpc.WithGRPCConn(conn))
//	// Handle insecure or TLS configuration
//	if insecure {
//		opts = append(opts, otlptracegrpc.WithInsecure())
//	} else {
//		c, err := credentials.NewClientTLSFromFile(caCertPath, "")
//		if err != nil {
//			return nil, fmt.Errorf("failed to create TLS credentials: %w", err)
//		}
//		opts = append(opts, otlptracegrpc.WithTLSCredentials(c))
//	}
//
//	// Add authorization header if needed (uncomment if using API keys)
//	// opts = append(opts, otlptracegrpc.WithHeaders(map[string]string{
//	// 	"Authorization": "Bearer " + apiKey,
//	// }))
//
//	exp, err := otlptracegrpc.New(context.Background(), opts...)
//	if err != nil {
//		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
//	}
//
//	res := resource.NewWithAttributes(
//		semconv.SchemaURL,
//		semconv.ServiceNameKey.String("fitmeapp"),
//		semconv.ServiceName("fitmeapp"),
//		semconv.ServiceVersionKey.String("0.1"),
//	)
//
//	tp := trace.NewTracerProvider(
//		trace.WithBatcher(exp),
//		trace.WithResource(res),
//	)
//
//	otel.SetTracerProvider(tp)
//	tp.Tracer("DeezNuts")
//	otel.SetTextMapPropagator(propagation.TraceContext{})
//
//	return tp, nil
//}
//
//func InitTracer() (*trace.TracerProvider, error) {
//	log := logger.Log
//	otlpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
//	if otlpEndpoint == "" {
//		log.Error("You MUST set OTEL_EXPORTER_OTLP_TRACES_ENDPOINT env variable!")
//	}
//
//	tp, err := newTracerProvider(otlpEndpoint, "", "", true)
//
//	if err != nil {
//		return nil, fmt.Errorf("failed to create trace provider: %w", err)
//	}
//
//	// Ensure TracerProvider shuts down properly on exit
//	go func() {
//		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
//		defer cancel()
//		if err = tp.Shutdown(ctx); err != nil {
//			log.Error("failed to shut down trace provider")
//		}
//	}()
//
//	return tp, nil
//}

//type multiExporter struct {
//	exporters []sdktrace.SpanExporter
//}
//
//func (m *multiExporter) Shutdown(ctx context.Context) error {
//	var lastErr error
//	for _, exp := range m.exporters {
//		if err := exp.Shutdown(ctx); err != nil {
//			// You could choose to combine errors or log them.
//			lastErr = err
//		}
//	}
//	return lastErr
//}

//func NewMultiExporter(exporters ...sdktrace.SpanExporter) sdktrace.SpanExporter {
//	return &multiExporter{exporters: exporters}
//}
//
//func (m *multiExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
//	var lastErr error
//	for _, exp := range m.exporters {
//		if err := exp.ExportSpans(ctx, spans); err != nil {
//			// You could choose to combine errors or log them.
//			lastErr = err
//		}
//	}
//	return lastErr
//}

//func NewOTLPExporter(ctx context.Context) (trace.SpanExporter, error) {
//	// Change default HTTPS -> HTTP
//	log := logger.Log
//	otlpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
//	fmt.Printf("otlp endpoint %s\n", otlpEndpoint)
//	if otlpEndpoint == "" {
//		log.Error("You MUST set OTEL_EXPORTER_OTLP_TRACES_ENDPOINT env variable!")
//	}
//
//	insecureOpt := sdktrace.WithInsecure()
//
//	// Update default OTLP reciver endpoint
//	endpointOpt := sdktrace.WithEndpoint(otlpEndpoint)
//
//	//timeout := otlptracehttp.WithTimeout(30 * time.Second)
//
//	return sdktrace.New(ctx, insecureOpt, endpointOpt)
//}

func NewTraceProvider(exp trace.SpanExporter) *trace.TracerProvider {
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("fitmeapp")))

	if err != nil {
		panic(err)
	}

	return trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithResource(r))
}

func InitOTELToCollector(ctx context.Context) error {
	log := logger.Log
	otlpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
	fmt.Printf("otlp endpoint %s\n", otlpEndpoint)
	if otlpEndpoint == "" {
		log.Error("You MUST set OTEL_EXPORTER_OTLP_TRACES_ENDPOINT env variable!")
	}

	exp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(otlpEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return fmt.Errorf("failed to create otlp trace exporter: %w", err)
	}

	// Create a Resource describing this application/service.
	// This adds standard attributes like service.name, service.version, etc.
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("score-app"),
			semconv.ServiceVersionKey.String("1.0.0"),
		),
	)
	if err != nil {
		return fmt.Errorf("failed creating resource: %w", err)
	}

	// Create a TracerProvider with a batch span processor and the OTLP exporter.
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithResource(res),
	)

	// Finally, set the global TracerProvider.
	otel.SetTracerProvider(tp)
	return nil
}
