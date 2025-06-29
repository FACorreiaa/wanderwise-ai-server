package tracer

import (
	"context"
	"errors"
	"fmt" // Added fmt
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"

	// Import OTLP exporter for traces if sending to Tempo/Jaeger
	// "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	// "go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
)

// InitOtelProviders initializes OpenTelemetry tracing and metrics providers.
// Returns a shutdown function.
func InitOtelProviders(serviceName string, metricsAddr string) (func(context.Context) error, error) {
	// --- Common Resource ---
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTel resource: %w", err)
	}

	// --- Tracer Provider Setup ---
	// Configure OTLP exporter for traces (example assumes gRPC endpoint)
	// Adjust endpoint and options (WithInsecure() for dev only!) as needed.
	/*
	   traceExporter, err := otlptracegrpc.New(context.Background(),
	       otlptracegrpc.WithEndpoint("otel-collector:4317"), // Your OTLP endpoint for Tempo/Jaeger
	       otlptracegrpc.WithInsecure(), // ONLY for local testing without TLS
	   )
	   if err != nil {
	       return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	   }
	   bsp := sdktrace.NewBatchSpanProcessor(traceExporter)
	   tp := sdktrace.NewTracerProvider(
	       sdktrace.WithResource(res),
	       sdktrace.WithSpanProcessor(bsp),
	   )
	   log.Println("Set up OpenTelemetry Tracer Provider with OTLP Exporter")
	*/
	// Using NoOp Tracer Provider if no trace backend is configured yet
	tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res))
	log.Println("Set up OpenTelemetry Tracer Provider (NoOp Exporter)")

	otel.SetTracerProvider(tp)
	// otel.SetTextMapPropagator(...) // Setup propagator if needed

	// --- Metrics Provider Setup (Prometheus) ---
	promExporter, err := prometheus.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create Prometheus exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(promExporter),
	)
	otel.SetMeterProvider(mp) // Set the global MeterProvider
	log.Println("Set up OpenTelemetry Meter Provider with Prometheus Exporter")

	// --- Start Prometheus Metrics Endpoint ---
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{Addr: metricsAddr, Handler: mux}
	go func() {
		log.Printf("Starting Prometheus metrics server on %s", metricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	// --- Return Shutdown Function ---
	shutdown := func(ctx context.Context) error {
		var shutdownErr error
		log.Println("Shutting down OpenTelemetry providers and metrics server...")
		// Shutdown metrics server first
		if err := metricsServer.Shutdown(ctx); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("metrics server shutdown error: %w", err))
		} else {
			log.Println("Metrics server stopped.")
		}
		// Shutdown MeterProvider
		if err := mp.Shutdown(ctx); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("OTel Meter Provider shutdown error: %w", err))
		} else {
			log.Println("OTel Meter Provider stopped.")
		}
		// Shutdown TracerProvider
		if err := tp.Shutdown(ctx); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("OTel Tracer Provider shutdown error: %w", err))
		} else {
			log.Println("OTel Tracer Provider stopped.")
		}
		return shutdownErr
	}

	return shutdown, nil
}
