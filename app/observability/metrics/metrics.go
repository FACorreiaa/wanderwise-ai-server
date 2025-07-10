package metrics

import (
	"log"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// AppMetrics holds the application's metric instruments.
// Make fields public so they can be accessed from other packages.
type AppMetrics struct {
	RegisterRequestsTotal   metric.Int64Counter
	RegisterDurationSeconds metric.Float64Histogram
	DBQueryDurationSeconds  metric.Float64Histogram
	DBQueryErrorsTotal      metric.Int64Counter
	// Add more metrics here (e.g., LoginRequestsTotal, ActiveSessionsGauge)
}

var (
	// Global instance of AppMetrics (initialized once)
	appMetrics *AppMetrics
	once       sync.Once
)

// InitAppMetrics initializes the global metrics instruments ONLY ONCE.
// It gets the Meter from the globally configured MeterProvider.
func InitAppMetrics() {
	once.Do(func() { // Ensure this only runs once
		meter := otel.GetMeterProvider().Meter("Loci") // Get meter from global provider
		var err error
		m := &AppMetrics{}

		m.RegisterRequestsTotal, err = meter.Int64Counter(
			"register_requests_total",
			metric.WithDescription("Total number of register requests completed"), // Clarify description
			metric.WithUnit("{request}"),                                          // Use semantic units if possible
		)
		if err != nil {
			log.Fatalf("Metrics: Failed to create register_requests_total: %v", err)
		}

		m.RegisterDurationSeconds, err = meter.Float64Histogram(
			"register_duration_seconds",
			metric.WithDescription("Duration of register requests in seconds"),
			metric.WithUnit("s"),
		)
		if err != nil {
			log.Fatalf("Metrics: Failed to create register_duration_seconds: %v", err)
		}

		m.DBQueryDurationSeconds, err = meter.Float64Histogram(
			"db_query_duration_seconds",
			metric.WithDescription("Duration of database queries in seconds"),
			metric.WithUnit("s"),
		)
		if err != nil {
			log.Fatalf("Metrics: Failed to create db_query_duration_seconds: %v", err)
		}

		m.DBQueryErrorsTotal, err = meter.Int64Counter(
			"db_query_errors_total",
			metric.WithDescription("Total number of database query errors"),
			metric.WithUnit("{error}"),
		)
		if err != nil {
			log.Fatalf("Metrics: Failed to create db_query_errors_total: %v", err)
		}

		log.Println("Application metrics instruments initialized.")
		appMetrics = m // Assign to global variable
	})
}

// Get returns the globally initialized AppMetrics instance.
// Panics if InitAppMetrics was not called first.
func Get() *AppMetrics {
	if appMetrics == nil {
		// This indicates a programming error - InitAppMetrics must be called at startup.
		panic("metrics instruments not initialized. Call metrics.InitAppMetrics() first.")
	}
	return appMetrics
}
