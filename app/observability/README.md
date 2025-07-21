# Observability Stack

This directory contains the complete observability stack configuration for local development of the Go AI POI server.

## Components

- **OpenTelemetry Collector**: Central collection point for metrics, traces, and logs
- **Prometheus**: Metrics storage and alerting
- **Tempo**: Distributed tracing backend
- **Loki**: Log aggregation
- **Grafana**: Visualization and dashboards
- **Promtail**: Log collection agent

## Architecture

```
Go App (local via air) → OpenTelemetry Collector → Prometheus/Tempo/Loki → Grafana
                      ↗                         ↘
                 Metrics (port 9090)        Traces & Logs
```

## Usage

### 1. Start Observability Services

```bash
# Start all observability services
docker-compose up -d

# Check status
docker-compose ps
```

### 2. Run Go Application Locally

```bash
# The app should expose metrics on :9090
air  # or go run main.go
```

### 3. Access Services

- **Grafana**: http://localhost:3000 (admin/admin)
- **Prometheus**: http://localhost:9090
- **Tempo**: http://localhost:3200
- **Loki**: http://localhost:3100

### 4. Integration in Go Code

In your main.go or initialization code:

```go
import (
    "context"
    "log"
    "github.com/your-repo/app/observability/metrics"
    "github.com/your-repo/app/observability/tracer"
)

func main() {
    // Initialize OpenTelemetry providers
    shutdown, err := tracer.InitOtelProviders("go-ai-poi", ":9090")
    if err != nil {
        log.Fatalf("Failed to initialize OTel providers: %v", err)
    }
    defer shutdown(context.Background())

    // Initialize application metrics
    metrics.InitAppMetrics()

    // Your application code...
}
```

## Configuration Files

- `prometheus.yml`: Prometheus scraping configuration
- `otel-collector-config.yaml`: OpenTelemetry Collector configuration
- `tempo-config.yaml`: Tempo tracing configuration
- `loki-config.yaml`: Loki log aggregation configuration
- `promtail.yml`: Log collection configuration
- `grafana/`: Grafana dashboards and datasource provisioning

## Key Features

### Metrics
- Application metrics exposed on `/metrics` endpoint
- Custom business metrics via OpenTelemetry Go SDK
- Automatic service discovery for locally running app

### Tracing
- Distributed tracing via OpenTelemetry
- Automatic trace correlation with logs and metrics
- Service map visualization in Grafana

### Logging
- Structured logging with trace correlation
- Log aggregation from containers and application
- Full-text search capabilities

### Dashboards
- Pre-configured Go application dashboard
- Infrastructure monitoring
- Custom alerting rules

## Development Tips

1. **Hot Reload**: Configuration changes require service restart:
   ```bash
   docker-compose restart <service-name>
   ```

2. **Debug**: Check collector logs for data flow:
   ```bash
   docker-compose logs otel-collector
   ```

3. **Local Testing**: App metrics should be available at:
   ```bash
   curl http://localhost:9090/metrics
   ```

## Troubleshooting

### App Metrics Not Showing
- Verify app is running on port 9090
- Check `host.docker.internal` connectivity
- Review Prometheus targets at http://localhost:9090/targets

### No Traces
- Verify OTLP exporter configuration in Go app
- Check OTel Collector logs
- Ensure Tempo is receiving data

### Missing Logs
- Check Promtail configuration
- Verify log file paths
- Review Loki ingestion at http://localhost:3100/ready