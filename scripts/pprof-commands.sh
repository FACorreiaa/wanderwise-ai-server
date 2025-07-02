#!/bin/bash

# pprof Commands and Heat Map Generation Script
# This script provides easy access to Go profiling tools and generates flame graphs

set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

print_header() {
    echo -e "${BLUE}=== $1 ===${NC}"
}

print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

check_pprof_running() {
    if ! curl -s http://localhost:6060/debug/pprof/ > /dev/null 2>&1; then
        print_warning "pprof server is not running on localhost:6060"
        echo "Start your Go application with ENABLE_PPROF=true environment variable"
        return 1
    fi
    return 0
}

# CPU Profiling
profile_cpu() {
    print_header "CPU Profiling"
    
    if ! check_pprof_running; then
        return 1
    fi
    
    duration="${1:-30}"
    output_file="cpu_profile_$(date +%Y%m%d_%H%M%S).prof"
    
    print_info "Collecting CPU profile for ${duration} seconds..."
    go tool pprof -raw -output="${output_file}" "http://localhost:6060/debug/pprof/profile?seconds=${duration}"
    
    print_info "CPU profile saved to: ${output_file}"
    print_info "View with: go tool pprof ${output_file}"
    print_info "Web UI: go tool pprof -http=:8081 ${output_file}"
    
    # Generate flame graph if available
    if command -v go-torch > /dev/null 2>&1; then
        print_info "Generating flame graph..."
        go-torch -file="cpu_flame_$(date +%Y%m%d_%H%M%S).svg" -url="http://localhost:6060" -seconds="${duration}"
    fi
}

# Memory Profiling (Heap)
profile_memory() {
    print_header "Memory Profiling (Heap)"
    
    if ! check_pprof_running; then
        return 1
    fi
    
    output_file="heap_profile_$(date +%Y%m%d_%H%M%S).prof"
    
    print_info "Collecting heap profile..."
    go tool pprof -raw -output="${output_file}" "http://localhost:6060/debug/pprof/heap"
    
    print_info "Heap profile saved to: ${output_file}"
    print_info "View with: go tool pprof ${output_file}"
    print_info "Web UI: go tool pprof -http=:8082 ${output_file}"
    
    # Show top memory consumers
    print_info "Top memory consumers:"
    go tool pprof -top "${output_file}"
}

# Goroutine Profiling
profile_goroutines() {
    print_header "Goroutine Profiling"
    
    if ! check_pprof_running; then
        return 1
    fi
    
    output_file="goroutine_profile_$(date +%Y%m%d_%H%M%S).prof"
    
    print_info "Collecting goroutine profile..."
    go tool pprof -raw -output="${output_file}" "http://localhost:6060/debug/pprof/goroutine"
    
    print_info "Goroutine profile saved to: ${output_file}"
    print_info "View with: go tool pprof ${output_file}"
    
    # Show goroutine summary
    print_info "Goroutine summary:"
    go tool pprof -top "${output_file}"
}

# Block Profiling
profile_blocking() {
    print_header "Block Profiling"
    
    if ! check_pprof_running; then
        return 1
    fi
    
    output_file="block_profile_$(date +%Y%m%d_%H%M%S).prof"
    
    print_info "Collecting block profile..."
    go tool pprof -raw -output="${output_file}" "http://localhost:6060/debug/pprof/block"
    
    print_info "Block profile saved to: ${output_file}"
    print_info "View with: go tool pprof ${output_file}"
}

# Mutex Profiling
profile_mutex() {
    print_header "Mutex Profiling"
    
    if ! check_pprof_running; then
        return 1
    fi
    
    output_file="mutex_profile_$(date +%Y%m%d_%H%M%S).prof"
    
    print_info "Collecting mutex profile..."
    go tool pprof -raw -output="${output_file}" "http://localhost:6060/debug/pprof/mutex"
    
    print_info "Mutex profile saved to: ${output_file}"
    print_info "View with: go tool pprof ${output_file}"
}

# Trace Profiling
profile_trace() {
    print_header "Execution Trace"
    
    if ! check_pprof_running; then
        return 1
    fi
    
    duration="${1:-10}"
    output_file="trace_$(date +%Y%m%d_%H%M%S).trace"
    
    print_info "Collecting execution trace for ${duration} seconds..."
    curl -o "${output_file}" "http://localhost:6060/debug/pprof/trace?seconds=${duration}"
    
    print_info "Trace saved to: ${output_file}"
    print_info "View with: go tool trace ${output_file}"
}

# Interactive pprof session
interactive_pprof() {
    print_header "Interactive pprof Session"
    
    if ! check_pprof_running; then
        return 1
    fi
    
    profile_type="${1:-profile}"
    
    case $profile_type in
        "cpu"|"profile")
            print_info "Starting interactive CPU profiling session..."
            go tool pprof "http://localhost:6060/debug/pprof/profile"
            ;;
        "heap"|"memory")
            print_info "Starting interactive heap profiling session..."
            go tool pprof "http://localhost:6060/debug/pprof/heap"
            ;;
        "goroutine")
            print_info "Starting interactive goroutine profiling session..."
            go tool pprof "http://localhost:6060/debug/pprof/goroutine"
            ;;
        "block")
            print_info "Starting interactive block profiling session..."
            go tool pprof "http://localhost:6060/debug/pprof/block"
            ;;
        "mutex")
            print_info "Starting interactive mutex profiling session..."
            go tool pprof "http://localhost:6060/debug/pprof/mutex"
            ;;
        *)
            print_warning "Unknown profile type: $profile_type"
            echo "Available types: cpu, heap, goroutine, block, mutex"
            return 1
            ;;
    esac
}

# Web UI for profiles
web_ui() {
    print_header "pprof Web UI"
    
    if ! check_pprof_running; then
        return 1
    fi
    
    profile_type="${1:-profile}"
    port="${2:-8081}"
    
    case $profile_type in
        "cpu"|"profile")
            print_info "Starting CPU profile web UI on :${port}..."
            go tool pprof -http=":${port}" "http://localhost:6060/debug/pprof/profile"
            ;;
        "heap"|"memory")
            print_info "Starting heap profile web UI on :${port}..."
            go tool pprof -http=":${port}" "http://localhost:6060/debug/pprof/heap"
            ;;
        "goroutine")
            print_info "Starting goroutine profile web UI on :${port}..."
            go tool pprof -http=":${port}" "http://localhost:6060/debug/pprof/goroutine"
            ;;
        *)
            print_warning "Unknown profile type: $profile_type"
            echo "Available types: cpu, heap, goroutine"
            return 1
            ;;
    esac
}

# Generate load for testing
generate_load() {
    print_header "Generating Load for Testing"
    
    duration="${1:-60}"
    concurrent="${2:-10}"
    
    print_info "Generating load for ${duration} seconds with ${concurrent} concurrent requests..."
    
    # Function to make requests
    make_requests() {
        local end_time=$((SECONDS + duration))
        while [ $SECONDS -lt $end_time ]; do
            curl -s http://localhost:8000/ping > /dev/null 2>&1 &
            curl -s http://localhost:8000/api/v1/cities > /dev/null 2>&1 &
            curl -s http://localhost:8000/metrics > /dev/null 2>&1 &
            sleep 0.1
        done
    }
    
    # Start concurrent load generators
    for i in $(seq 1 $concurrent); do
        make_requests &
    done
    
    print_info "Load generation started. Wait ${duration} seconds then profile."
}

# Show pprof endpoints
show_endpoints() {
    print_header "Available pprof Endpoints"
    
    if ! check_pprof_running; then
        print_warning "pprof server not running. Available endpoints:"
    else
        print_info "Active pprof endpoints:"
    fi
    
    echo ""
    echo "🔬 Profile Types:"
    echo "  • http://localhost:6060/debug/pprof/          - Index page"
    echo "  • http://localhost:6060/debug/pprof/profile   - CPU profile"
    echo "  • http://localhost:6060/debug/pprof/heap      - Heap memory profile"
    echo "  • http://localhost:6060/debug/pprof/goroutine - Goroutine profile"
    echo "  • http://localhost:6060/debug/pprof/block     - Block profile"
    echo "  • http://localhost:6060/debug/pprof/mutex     - Mutex profile"
    echo "  • http://localhost:6060/debug/pprof/trace     - Execution trace"
    echo ""
    echo "📊 Runtime Info:"
    echo "  • http://localhost:6060/debug/pprof/cmdline   - Command line"
    echo "  • http://localhost:6060/debug/pprof/symbol    - Symbol table"
    echo ""
}

# Help function
show_help() {
    echo "Go AI POI pprof Profiling Tool"
    echo ""
    echo "Usage: $0 <command> [options]"
    echo ""
    echo "Commands:"
    echo "  cpu [duration]         - CPU profile (default: 30s)"
    echo "  memory                 - Memory/heap profile"
    echo "  goroutines            - Goroutine profile"
    echo "  block                 - Block profile"
    echo "  mutex                 - Mutex profile"
    echo "  trace [duration]      - Execution trace (default: 10s)"
    echo "  interactive [type]    - Interactive pprof session"
    echo "  web [type] [port]     - Web UI for profiles"
    echo "  load [duration] [concurrent] - Generate test load"
    echo "  endpoints             - Show available endpoints"
    echo "  help                  - Show this help"
    echo ""
    echo "Examples:"
    echo "  $0 cpu 60             - 60-second CPU profile"
    echo "  $0 web heap 8082      - Heap profile web UI on port 8082"
    echo "  $0 interactive cpu    - Interactive CPU profiling"
    echo "  $0 load 120 20        - Generate load for 2 minutes with 20 workers"
    echo ""
}

# Main execution
case "${1:-}" in
    "cpu")
        profile_cpu "${2:-30}"
        ;;
    "memory"|"heap")
        profile_memory
        ;;
    "goroutines"|"goroutine")
        profile_goroutines
        ;;
    "block")
        profile_blocking
        ;;
    "mutex")
        profile_mutex
        ;;
    "trace")
        profile_trace "${2:-10}"
        ;;
    "interactive")
        interactive_pprof "${2:-profile}"
        ;;
    "web")
        web_ui "${2:-profile}" "${3:-8081}"
        ;;
    "load")
        generate_load "${2:-60}" "${3:-10}"
        ;;
    "endpoints")
        show_endpoints
        ;;
    "help"|"--help"|"-h")
        show_help
        ;;
    *)
        echo "Unknown command: ${1:-}"
        echo "Use '$0 help' for usage information"
        exit 1
        ;;
esac