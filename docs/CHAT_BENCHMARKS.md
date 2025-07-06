# Chat Streaming Benchmarks

This document describes the comprehensive benchmark suite for testing the chat streaming endpoints in the Go AI POI server.

## Overview

The benchmark suite tests two main endpoints:
- **StartChatMessageStream**: Initiates new chat sessions with streaming responses
- **ContinueChatSessionStream**: Continues existing chat sessions with streaming responses

## Test Scenarios

### StartChatMessageStream Tests
- **"Plan Esposende"**: Tests city planning functionality
- **"Restaurant in Povoa de Varzim"**: Tests restaurant recommendation functionality

### ContinueChatSessionStream Tests
- **"Add Stadium"**: Tests adding POIs to itineraries
- **"Remove Stadium"**: Tests removing POIs from itineraries  
- **"Add Ibis Hotel"**: Tests adding hotels to itineraries
- **"Remove Ibis Hotel"**: Tests removing hotels from itineraries

## Running Benchmarks

### Quick Start

```bash
# Run basic benchmarks
make benchmark-chat-quick

# Run comprehensive benchmarks
make benchmark-chat

# Run load testing
make benchmark-chat-load
```

### Manual Execution

```bash
# Basic benchmark
./scripts/run_chat_benchmark.sh

# Custom configuration
./scripts/run_chat_benchmark.sh \
  --url http://localhost:8080/api/v1 \
  --iterations 10 \
  --concurrent 5 \
  --verbose \
  --timeout 60s
```

### Go Benchmark Tests

```bash
# Run Go benchmark tests
make benchmark-go

# Run concurrent tests
make benchmark-concurrent

# Run integration tests
make test-integration
```

## Benchmark Configuration

### Command Line Options

| Option | Description | Default |
|--------|-------------|---------|
| `--url` | Base URL for the API | `http://localhost:8080/api/v1` |
| `--iterations` | Number of iterations per test | `5` |
| `--concurrent` | Number of concurrent users | `1` |
| `--timeout` | Request timeout | `60s` |
| `--verbose` | Enable verbose output | `false` |
| `--token` | Authentication token | auto-generated |

### Environment Variables

```bash
export CHAT_BENCHMARK_URL="http://localhost:8080/api/v1"
export CHAT_BENCHMARK_TOKEN="your-jwt-token"
export CHAT_BENCHMARK_ITERATIONS="10"
export CHAT_BENCHMARK_CONCURRENT="5"
```

## Benchmark Results

The benchmark provides detailed metrics including:

### Performance Metrics
- **Total Requests**: Number of requests executed
- **Success Rate**: Percentage of successful requests
- **Average Duration**: Mean response time
- **Min/Max Duration**: Fastest and slowest response times
- **Requests per Second**: Throughput measurement
- **First Event Time**: Time to first SSE event
- **Last Event Time**: Time to final SSE event

### Streaming Metrics
- **Events Received**: Total number of SSE events
- **Event Types**: Breakdown of event types received
- **Response Size**: Total bytes transferred
- **Session ID Extraction**: Validation of session management

### Example Output

```
🚀 Starting Chat Streaming Benchmark
📍 Base URL: http://localhost:8080/api/v1
🔁 Iterations: 5
👥 Concurrent Users: 2
⏱️  Timeout: 60s

🎯 Testing StartChatMessageStream...
  📝 'Plan Esposende': 2.345s (15 events) ✅
  📝 'Restaurant in Povoa de Varzim': 1.876s (12 events) ✅

📊 StartChatMessageStream Results:
  📈 Total Requests: 10
  ✅ Successful: 10
  ❌ Failed: 0
  📊 Success Rate: 100.00%
  ⏱️  Average Duration: 2.110s
  🏃 Min Duration: 1.654s
  🐌 Max Duration: 2.567s
  🚀 Requests/Second: 4.74
  📡 Total Events: 127
  📦 Total Response Size: 45632 bytes

🔄 Testing ContinueChatSessionStream...
  📝 'Add Stadium': 1.234s (8 events) ✅
  📝 'Remove Stadium': 1.123s (6 events) ✅
  📝 'Add Ibis Hotel': 1.345s (9 events) ✅
  📝 'Remove Ibis Hotel': 1.098s (7 events) ✅

📊 ContinueChatSessionStream Results:
  📈 Total Requests: 20
  ✅ Successful: 20
  ❌ Failed: 0
  📊 Success Rate: 100.00%
  ⏱️  Average Duration: 1.200s
  🏃 Min Duration: 1.045s
  🐌 Max Duration: 1.456s
  🚀 Requests/Second: 16.67
  📡 Total Events: 150
  📦 Total Response Size: 32450 bytes

✅ Benchmark completed!
```

## File Structure

```
go-ai-poi-server/
├── internal/api/chat_prompt/
│   ├── chat_benchmark_test.go     # Go benchmark tests
│   ├── chat_test.go              # Unit tests
│   └── chat_integration_test.go  # Integration tests
├── cmd/benchmark/
│   └── chat_benchmark.go         # Standalone benchmark tool
├── scripts/
│   └── run_chat_benchmark.sh     # Benchmark runner script
├── docs/
│   └── CHAT_BENCHMARKS.md        # This documentation
└── Makefile                      # Build targets
```

## Benchmark Components

### 1. Go Benchmark Tests (`chat_benchmark_test.go`)

Standard Go benchmark tests that integrate with `go test -bench`:

```go
func BenchmarkStartChatMessageStream(b *testing.B)
func BenchmarkContinueChatSessionStream(b *testing.B) 
func BenchmarkConcurrentChatRequests(b *testing.B)
```

### 2. Standalone Benchmark Tool (`chat_benchmark.go`)

Independent benchmark tool that can be run without the full test suite:

```bash
go run cmd/benchmark/chat_benchmark.go --help
```

### 3. Shell Script Runner (`run_chat_benchmark.sh`)

Comprehensive benchmark runner with configuration options and health checks:

```bash
./scripts/run_chat_benchmark.sh --help
```

## Test Data

The benchmarks use realistic test data:

### Location Data
- **Esposende Coordinates**: 41.4901, -8.7853
- **Povoa de Varzim**: Adjacent city for restaurant queries

### Chat Messages
- City planning: "Plan Esposende"
- Restaurant queries: "Restaurant in Povoa de Varzim"
- Itinerary modifications: Add/Remove Stadium and Ibis Hotel

## Metrics Collection

### Request-Level Metrics
- HTTP status codes
- Response times
- Request/response sizes
- Error rates

### Stream-Level Metrics
- SSE event counts
- Event type distribution
- Time to first/last event
- Stream completion rates

### Session-Level Metrics
- Session ID extraction
- Session continuity
- Cross-request session validation

## Performance Expectations

### Typical Performance Targets

| Metric | Target | Notes |
|--------|--------|-------|
| Average Response Time | < 3s | For initial chat responses |
| Success Rate | > 95% | Under normal load |
| Time to First Event | < 500ms | SSE stream initialization |
| Concurrent Users | 10+ | Without degradation |
| Requests per Second | 5+ | Per endpoint |

### Load Testing Recommendations

1. **Light Load**: 1-2 concurrent users, 5 iterations
2. **Normal Load**: 5-10 concurrent users, 10 iterations  
3. **Heavy Load**: 10-20 concurrent users, 20 iterations
4. **Stress Test**: 20+ concurrent users until failure

## Troubleshooting

### Common Issues

#### Server Not Running
```bash
# Check if server is running
curl http://localhost:8080/health

# Start the server
go run main.go
```

#### Authentication Failures
```bash
# Use custom token
./scripts/run_chat_benchmark.sh --token "your-jwt-token"

# Check token expiration
echo "your-jwt-token" | base64 -d | jq .exp
```

#### Timeout Issues
```bash
# Increase timeout
./scripts/run_chat_benchmark.sh --timeout 120s

# Reduce concurrent users
./scripts/run_chat_benchmark.sh --concurrent 1
```

#### Memory Issues
```bash
# Run with memory profiling
go test -bench=BenchmarkChatStreamingRoutes -memprofile=mem.prof ./internal/api/chat_prompt/

# Analyze memory usage
go tool pprof mem.prof
```

### Debug Mode

Enable verbose logging for detailed debugging:

```bash
# Verbose benchmark output
./scripts/run_chat_benchmark.sh --verbose

# Go benchmark with detailed output
go test -bench=. -v ./internal/api/chat_prompt/
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Chat Benchmark
on: [push, pull_request]

jobs:
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v3
        with:
          go-version: 1.21
      
      - name: Start test server
        run: |
          go run main.go &
          sleep 10
      
      - name: Run benchmarks
        run: make benchmark-chat-quick
      
      - name: Run Go benchmarks
        run: make benchmark-go
```

### Performance Regression Detection

Monitor key metrics across builds:
- Average response time increases > 20%
- Success rate drops below 95%
- Memory usage increases > 50%
- Requests per second decreases > 30%

## Contributing

When adding new benchmark tests:

1. Follow the existing pattern in `chat_benchmark_test.go`
2. Add realistic test data and scenarios
3. Include both success and failure cases
4. Document expected performance characteristics
5. Update this README with new test scenarios

### Adding New Test Cases

```go
func BenchmarkNewChatFeature(b *testing.B) {
    config := NewBenchmarkConfig()
    config.AuthToken = generateTestAuthToken(config.UserID)
    
    for i := 0; i < b.N; i++ {
        result := benchmarkStartChat(config, "Your test message")
        if !result.Success {
            b.Errorf("Test failed: %s", result.Error)
        }
    }
}
```

## References

- [Go Benchmark Documentation](https://golang.org/pkg/testing/#hdr-Benchmarks)
- [Server-Sent Events Specification](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events)
- [HTTP Load Testing Best Practices](https://github.com/wg/wrk)
- [Go Performance Optimization](https://github.com/golang/go/wiki/Performance)