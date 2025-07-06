#!/bin/bash

# Chat Streaming Benchmark Runner
# This script runs comprehensive benchmarks for the chat streaming endpoints

set -e

# Default configuration
BASE_URL="http://localhost:8080/api/v1"
ITERATIONS=5
CONCURRENT_USERS=1
TIMEOUT="60s"
VERBOSE=false
AUTH_TOKEN=""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

# Function to show usage
show_usage() {
    cat << EOF
Chat Streaming Benchmark Runner

Usage: $0 [OPTIONS]

OPTIONS:
    -u, --url URL               Base URL for the API (default: http://localhost:8080/api/v1)
    -i, --iterations N          Number of iterations to run (default: 5)
    -c, --concurrent N          Number of concurrent users (default: 1)
    -t, --timeout DURATION      Request timeout (default: 60s)
    -v, --verbose              Enable verbose output
    -k, --token TOKEN           Authentication token (optional)
    -h, --help                 Show this help message

EXAMPLES:
    # Basic benchmark
    $0

    # High-load benchmark with 10 concurrent users
    $0 --concurrent 10 --iterations 10

    # Quick benchmark with custom URL
    $0 --url http://staging.example.com/api/v1 --iterations 3

    # Verbose benchmark with custom timeout
    $0 --verbose --timeout 30s

    # Benchmark with custom auth token
    $0 --token "your-jwt-token-here"

ENVIRONMENT VARIABLES:
    CHAT_BENCHMARK_URL          Override default base URL
    CHAT_BENCHMARK_TOKEN        Override default auth token
    CHAT_BENCHMARK_ITERATIONS   Override default iterations
    CHAT_BENCHMARK_CONCURRENT   Override default concurrent users

EOF
}

# Function to check if server is running
check_server() {
    local url="$1"
    local health_url="${url%/api/v1}/health"
    
    print_info "Checking if server is running at $health_url..."
    
    if curl -s -f "$health_url" > /dev/null 2>&1; then
        print_success "Server is running and accessible"
        return 0
    else
        print_warning "Server health check failed at $health_url"
        print_info "Continuing anyway - server might not have a health endpoint"
        return 0
    fi
}

# Function to validate URL format
validate_url() {
    local url="$1"
    if [[ ! "$url" =~ ^https?:// ]]; then
        print_error "Invalid URL format: $url"
        print_info "URL must start with http:// or https://"
        exit 1
    fi
}

# Function to run Go benchmark tests
run_go_benchmarks() {
    print_info "Running Go benchmark tests..."
    
    cd "$(dirname "$0")/.."
    
    # Set environment variables for the benchmark
    export BENCHMARK_BASE_URL="$BASE_URL"
    export BENCHMARK_AUTH_TOKEN="$AUTH_TOKEN"
    export BENCHMARK_ITERATIONS="$ITERATIONS"
    export BENCHMARK_CONCURRENT="$CONCURRENT_USERS"
    
    # Run the benchmark tests
    if go test -bench=BenchmarkChatStreamingRoutes -benchmem -timeout=300s ./internal/api/chat_prompt/; then
        print_success "Go benchmarks completed successfully"
    else
        print_warning "Go benchmarks failed or had issues"
    fi
}

# Function to run standalone benchmark
run_standalone_benchmark() {
    print_info "Running standalone benchmark..."
    
    cd "$(dirname "$0")/.."
    
    local cmd_args=(
        "--url" "$BASE_URL"
        "--iterations" "$ITERATIONS"
        "--concurrent" "$CONCURRENT_USERS"
        "--timeout" "$TIMEOUT"
    )
    
    if [ "$VERBOSE" = true ]; then
        cmd_args+=("--verbose")
    fi
    
    if [ -n "$AUTH_TOKEN" ]; then
        cmd_args+=("--token" "$AUTH_TOKEN")
    fi
    
    # Build and run the standalone benchmark
    if go run cmd/benchmark/chat_benchmark.go "${cmd_args[@]}"; then
        print_success "Standalone benchmark completed successfully"
    else
        print_error "Standalone benchmark failed"
        exit 1
    fi
}

# Function to run integration tests
run_integration_tests() {
    print_info "Running integration tests..."
    
    cd "$(dirname "$0")/.."
    
    # Set environment variables
    export BENCHMARK_BASE_URL="$BASE_URL"
    export BENCHMARK_AUTH_TOKEN="$AUTH_TOKEN"
    
    # Run integration tests
    if go test -v -tags=integration ./internal/api/chat_prompt/ -run TestChatStreamingRoutes; then
        print_success "Integration tests completed successfully"
    else
        print_warning "Integration tests failed or had issues"
    fi
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -u|--url)
            BASE_URL="$2"
            shift 2
            ;;
        -i|--iterations)
            ITERATIONS="$2"
            shift 2
            ;;
        -c|--concurrent)
            CONCURRENT_USERS="$2"
            shift 2
            ;;
        -t|--timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -k|--token)
            AUTH_TOKEN="$2"
            shift 2
            ;;
        -h|--help)
            show_usage
            exit 0
            ;;
        *)
            print_error "Unknown option: $1"
            show_usage
            exit 1
            ;;
    esac
done

# Override with environment variables if set
BASE_URL="${CHAT_BENCHMARK_URL:-$BASE_URL}"
AUTH_TOKEN="${CHAT_BENCHMARK_TOKEN:-$AUTH_TOKEN}"
ITERATIONS="${CHAT_BENCHMARK_ITERATIONS:-$ITERATIONS}"
CONCURRENT_USERS="${CHAT_BENCHMARK_CONCURRENT:-$CONCURRENT_USERS}"

# Validate inputs
validate_url "$BASE_URL"

if ! [[ "$ITERATIONS" =~ ^[0-9]+$ ]] || [ "$ITERATIONS" -lt 1 ]; then
    print_error "Iterations must be a positive integer"
    exit 1
fi

if ! [[ "$CONCURRENT_USERS" =~ ^[0-9]+$ ]] || [ "$CONCURRENT_USERS" -lt 1 ]; then
    print_error "Concurrent users must be a positive integer"
    exit 1
fi

# Print configuration
echo "🚀 Chat Streaming Benchmark Configuration"
echo "============================================"
echo "Base URL: $BASE_URL"
echo "Iterations: $ITERATIONS"
echo "Concurrent Users: $CONCURRENT_USERS"
echo "Timeout: $TIMEOUT"
echo "Verbose: $VERBOSE"
echo "Auth Token: $([ -n "$AUTH_TOKEN" ] && echo "***PROVIDED***" || echo "auto-generated")"
echo "============================================"
echo

# Check if server is running
check_server "$BASE_URL"

# Run the benchmarks
print_info "Starting benchmark execution..."

# Run standalone benchmark (main test)
run_standalone_benchmark

# Optionally run Go benchmarks if requested
if [ "$VERBOSE" = true ]; then
    echo
    print_info "Running additional Go benchmarks for detailed profiling..."
    run_go_benchmarks
    
    echo
    print_info "Running integration tests..."
    run_integration_tests
fi

echo
print_success "All benchmarks completed!"
print_info "Check the output above for detailed results and performance metrics."

# Summary
echo
echo "📋 Test Summary:"
echo "  📝 StartChatMessageStream tests:"
echo "    - 'Plan Esposende'"
echo "    - 'Restaurant in Povoa de Varzim'"
echo "  🔄 ContinueChatSessionStream tests:"
echo "    - 'Add Stadium'"
echo "    - 'Remove Stadium'"
echo "    - 'Add Ibis Hotel'"
echo "    - 'Remove Ibis Hotel'"
echo "  👥 Concurrent users: $CONCURRENT_USERS"
echo "  🔁 Iterations per test: $ITERATIONS"
echo