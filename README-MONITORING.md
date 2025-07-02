# 🔬 Go AI POI - Monitoring & Profiling Setup

This guide covers the complete observability stack for the Go AI POI application, including pprof profiling, Prometheus metrics, distributed tracing with Jaeger/Tempo, and log aggregation with Loki.

## 🚀 Quick Start

```bash
# Start the complete monitoring stack
./scripts/monitoring-setup.sh start

# Start pprof profiling
ENABLE_PPROF=true go run main.go

# Generate CPU profile
./scripts/pprof-commands.sh cpu 30

# View profiles in web UI
./scripts/pprof-commands.sh web heap 8081
```

## 📊 Monitoring Stack Overview

### Services and Ports

| Service | Port | Purpose | URL |
|---------|------|---------|-----|
| **API Server** | 8000 | Main application | http://localhost:8000 |
| **Metrics** | 9090 | Prometheus metrics | http://localhost:9090/metrics |
| **pprof** | 6060 | Go profiling | http://localhost:6060/debug/pprof/ |
| **Prometheus** | 9092 | Metrics collection | http://localhost:9092 |
| **Grafana** | 3001 | Visualization | http://localhost:3001 |
| **Tempo** | 3200 | Distributed tracing | http://localhost:3200 |
| **Jaeger** | 16686 | Tracing UI | http://localhost:16686 |
| **Loki** | 3100 | Log aggregation | http://localhost:3100 |
| **Node Exporter** | 9100 | System metrics | http://localhost:9100 |
| **cAdvisor** | 8080 | Container metrics | http://localhost:8080 |

### Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Go AI POI     │───▶│ OpenTelemetry    │───▶│   Prometheus    │
│   Application   │    │   Collector      │    │                 │
│                 │    │                  │    │                 │
│ • HTTP Metrics  │    │ • Metrics        │    │ • Time Series   │
│ • Custom Metrics│    │ • Traces         │    │ • Alerting      │
│ • pprof         │    │ • Logs           │    │                 │
└─────────────────┘    └──────────────────┘    └─────────────────┘
         │                       │                       │
         │                       ▼                       ▼
         │              ┌─────────────────┐    ┌─────────────────┐
         │              │     Tempo       │    │    Grafana      │
         │              │                 │    │                 │
         │              │ • Trace Storage │    │ • Dashboards    │
         │              │ • Span Metrics  │    │ • Visualization │
         │              │                 │    │ • Alerting      │
         │              └─────────────────┘    └─────────────────┘
         │                       │
         ▼                       ▼
┌─────────────────┐    ┌─────────────────┐
│     Jaeger      │    │      Loki       │
│                 │    │                 │
│ • Trace UI      │    │ • Log Storage   │
│ • Search        │    │ • Log Query     │
│ • Analysis      │    │ • Log Streams   │
└─────────────────┘    └─────────────────┘
```

## 🔧 Setup Instructions

### 1. Start Monitoring Stack

```bash
# Make scripts executable
chmod +x scripts/*.sh

# Start the complete stack
./scripts/monitoring-setup.sh start

# Check service status
./scripts/monitoring-setup.sh status

# View logs
./scripts/monitoring-setup.sh logs grafana
```

### 2. Enable pprof in Go Application

Add to your `main.go`:

```go
import (
    _ "net/http/pprof"
    "net/http"
)

func init() {
    if os.Getenv("ENABLE_PPROF") == "true" {
        go func() {
            log.Println("Starting pprof server on :6060")
            log.Println(http.ListenAndServe(":6060", nil))
        }()
    }
}
```

Or use the provided setup:

```bash
ENABLE_PPROF=true go run main.go
```

### 3. Access Dashboards

- **Grafana**: http://localhost:3001 (admin/admin)
- **Prometheus**: http://localhost:9092
- **Jaeger**: http://localhost:16686
- **pprof**: http://localhost:6060/debug/pprof/

## 🔬 pprof Profiling

### Available Profiles

| Profile Type | Endpoint | Purpose |
|--------------|----------|---------|
| **CPU** | `/debug/pprof/profile` | CPU usage and bottlenecks |
| **Heap** | `/debug/pprof/heap` | Memory allocation |
| **Goroutines** | `/debug/pprof/goroutine` | Goroutine analysis |
| **Block** | `/debug/pprof/block` | Blocking operations |
| **Mutex** | `/debug/pprof/mutex` | Mutex contention |
| **Trace** | `/debug/pprof/trace` | Execution timeline |

### Quick Commands

```bash
# CPU profiling (30 seconds)
./scripts/pprof-commands.sh cpu 30

# Memory profiling
./scripts/pprof-commands.sh memory

# Goroutine analysis
./scripts/pprof-commands.sh goroutines

# Interactive session
./scripts/pprof-commands.sh interactive cpu

# Web UI
./scripts/pprof-commands.sh web heap 8081

# Generate test load
./scripts/pprof-commands.sh load 60 10
```

### Manual pprof Commands

```bash
# CPU profile with flame graph
go tool pprof -http=:8081 http://localhost:6060/debug/pprof/profile

# Memory analysis
go tool pprof -http=:8082 http://localhost:6060/debug/pprof/heap

# Save profile to file
go tool pprof -output=cpu.prof http://localhost:6060/debug/pprof/profile

# Interactive analysis
go tool pprof http://localhost:6060/debug/pprof/heap
```

### pprof Web UI Commands

Once in the pprof web interface:

- **Flame Graph**: Best for CPU analysis
- **Top**: Shows functions using most resources
- **Graph**: Visual call graph
- **Peek**: Source code view
- **Source**: Annotated source code

## 📈 Metrics Collection

### Available Metrics

#### Built-in Go Metrics
- `go_memstats_*` - Memory statistics
- `go_goroutines` - Number of goroutines
- `go_gc_*` - Garbage collection metrics

#### Application Metrics
- `http_requests_total` - HTTP request count
- `http_request_duration_seconds` - Request latency
- `db_query_duration_seconds` - Database query time
- `register_requests_total` - Registration attempts
- `register_duration_seconds` - Registration latency

#### System Metrics (Node Exporter)
- CPU usage
- Memory usage
- Disk I/O
- Network traffic
- Load average

#### Container Metrics (cAdvisor)
- Container CPU usage
- Container memory usage
- Container network I/O
- Container filesystem usage

### Custom Metrics

Add custom metrics to your Go application:

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name: "http_request_duration_seconds",
        Help: "Duration of HTTP requests.",
    }, []string{"path", "method"})
    
    requestCount = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "http_requests_total",
        Help: "Total number of HTTP requests.",
    }, []string{"path", "method", "status"})
)

// Usage in middleware
func metricsMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        
        next.ServeHTTP(w, r)
        
        duration := time.Since(start).Seconds()
        httpDuration.WithLabelValues(r.URL.Path, r.Method).Observe(duration)
        requestCount.WithLabelValues(r.URL.Path, r.Method, "200").Inc()
    })
}
```

## 🔍 Distributed Tracing

### OpenTelemetry Setup

The application includes OpenTelemetry for distributed tracing:

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/trace"
)

// Create spans in your handlers
func yourHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    tracer := otel.Tracer("your-service")
    
    ctx, span := tracer.Start(ctx, "handler-operation")
    defer span.End()
    
    // Your handler logic
    span.SetAttributes(attribute.String("user.id", userID))
    span.AddEvent("Processing request")
}
```

### Viewing Traces

1. **Jaeger UI**: http://localhost:16686
   - Search by service, operation, tags
   - View trace timeline
   - Analyze span relationships

2. **Grafana**: http://localhost:3001
   - Integrated trace view
   - Correlation with metrics
   - Custom trace queries

## 📋 Log Aggregation

### Loki Configuration

Logs are collected and stored in Loki:

- **Application logs**: Structured JSON logs
- **Container logs**: Docker container logs
- **System logs**: System-level logs

### Log Queries

In Grafana, use LogQL to query logs:

```logql
# Application logs
{job="go-ai-poi-app"} |= "error"

# Filter by level
{job="go-ai-poi-app"} | json | level="error"

# Rate of errors
rate({job="go-ai-poi-app"} | json | level="error"[5m])

# Logs with specific request ID
{job="go-ai-poi-app"} | json | request_id="abc123"
```

## 🚨 Alerting

### Prometheus Alerts

Create alerting rules in `observability/prometheus-rules.yml`:

```yaml
groups:
  - name: go-ai-poi-alerts
    rules:
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.1
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High error rate detected"
          
      - alert: HighMemoryUsage
        expr: go_memstats_alloc_bytes > 1000000000  # 1GB
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "High memory usage"
```

## 🔧 Troubleshooting

### Common Issues

1. **pprof not accessible**
   ```bash
   # Check if ENABLE_PPROF is set
   echo $ENABLE_PPROF
   
   # Check if port 6060 is open
   netstat -tlnp | grep 6060
   ```

2. **Services not starting**
   ```bash
   # Check Docker logs
   docker-compose -f docker-compose.monitoring.yml logs [service]
   
   # Check service health
   ./scripts/monitoring-setup.sh status
   ```

3. **No metrics in Prometheus**
   ```bash
   # Check metrics endpoint
   curl http://localhost:9090/metrics
   
   # Check Prometheus targets
   # Visit: http://localhost:9092/targets
   ```

4. **Missing traces**
   ```bash
   # Check OpenTelemetry collector logs
   docker-compose -f docker-compose.monitoring.yml logs otel-collector
   
   # Verify OTEL environment variables
   echo $OTEL_EXPORTER_OTLP_ENDPOINT
   ```

### Performance Impact

- **pprof**: Minimal overhead when not actively profiling
- **Metrics**: ~1-2% CPU overhead
- **Tracing**: ~2-5% overhead depending on sampling rate
- **Logging**: ~1-3% overhead with structured logging

### Best Practices

1. **Profile regularly** during development and load testing
2. **Use sampling** for traces in production (10-20%)
3. **Monitor the monitors** - watch resource usage of observability stack
4. **Correlate data** - use request IDs to link metrics, traces, and logs
5. **Set up alerts** for critical business and technical metrics

## 📚 Additional Resources

- [Go pprof Documentation](https://golang.org/pkg/net/http/pprof/)
- [Prometheus Best Practices](https://prometheus.io/docs/practices/)
- [OpenTelemetry Go](https://opentelemetry.io/docs/instrumentation/go/)
- [Grafana Dashboards](https://grafana.com/grafana/dashboards/)
- [Jaeger Documentation](https://www.jaegertracing.io/docs/)

---

## 🎯 Quick Reference

### Start Everything
```bash
./scripts/monitoring-setup.sh start
```

### Profile Performance
```bash
./scripts/pprof-commands.sh cpu 60
./scripts/pprof-commands.sh web heap 8081
```

### Check Status
```bash
./scripts/monitoring-setup.sh status
```

### View Logs
```bash
./scripts/monitoring-setup.sh logs api
```

### Stop Everything
```bash
./scripts/monitoring-setup.sh stop
```