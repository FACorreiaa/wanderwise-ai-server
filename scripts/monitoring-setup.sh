#!/bin/bash

# Go AI POI Monitoring Setup Script
# This script sets up the complete observability stack

set -e

echo "🚀 Setting up Go AI POI Monitoring Stack..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_header() {
    echo -e "${BLUE}=== $1 ===${NC}"
}

# Check if Docker is running
check_docker() {
    print_header "Checking Docker"
    if ! docker info > /dev/null 2>&1; then
        print_error "Docker is not running. Please start Docker and try again."
        exit 1
    fi
    print_status "Docker is running"
}

# Create necessary directories
create_directories() {
    print_header "Creating Directories"

    directories=(
        "observability/grafana/provisioning/datasources"
        "observability/grafana/provisioning/dashboards"
        "observability/grafana/dashboards"
        "logs"
    )

    for dir in "${directories[@]}"; do
        if [ ! -d "$dir" ]; then
            mkdir -p "$dir"
            print_status "Created directory: $dir"
        fi
    done
}

# Start the monitoring stack
start_monitoring() {
    print_header "Starting Monitoring Stack"

    # Stop any existing containers
    print_status "Stopping existing containers..."
    docker-compose -f docker-compose.monitoring.yml down > /dev/null 2>&1 || true

    # Start the stack
    print_status "Starting monitoring services..."
    docker-compose -f docker-compose.monitoring.yml up --build

    # Wait for services to be ready
    print_status "Waiting for services to start..."
    sleep 10
}

# Check service health
check_services() {
    print_header "Checking Service Health"

    services=(
        "http://localhost:8000/ping:API Server"
        "http://localhost:9092:Prometheus"
        "http://localhost:3001:Grafana"
        "http://localhost:3200:Tempo"
        "http://localhost:3100/ready:Loki"
        "http://localhost:16686:Jaeger"
        "http://localhost:6060/debug/pprof/:pprof"
    )

    for service in "${services[@]}"; do
        url="${service%:*}"
        name="${service#*:}"

        if curl -s "$url" > /dev/null 2>&1; then
            print_status "$name is healthy ✓"
        else
            print_warning "$name is not responding ⚠️"
        fi
    done
}

# Display access URLs
show_urls() {
    print_header "Access URLs"
    echo ""
    echo -e "${BLUE}📊 Monitoring Dashboards:${NC}"
    echo "  • Grafana:    http://localhost:3001 (admin/admin)"
    echo "  • Prometheus: http://localhost:9092"
    echo "  • Jaeger:     http://localhost:16686"
    echo ""
    echo -e "${BLUE}🔬 Profiling:${NC}"
    echo "  • pprof:      http://localhost:6060/debug/pprof/"
    echo "  • Heap:       http://localhost:6060/debug/pprof/heap"
    echo "  • Goroutines: http://localhost:6060/debug/pprof/goroutine"
    echo "  • CPU:        http://localhost:6060/debug/pprof/profile"
    echo ""
    echo -e "${BLUE}📋 API Endpoints:${NC}"
    echo "  • API:        http://localhost:8000"
    echo "  • Metrics:    http://localhost:9090/metrics"
    echo "  • Health:     http://localhost:8000/ping"
    echo ""
    echo -e "${BLUE}📈 System Metrics:${NC}"
    echo "  • Node Exporter: http://localhost:9100"
    echo "  • cAdvisor:      http://localhost:8080"
    echo ""
}

# Generate load for testing
generate_load() {
    print_header "Generating Test Load"

    if command -v curl > /dev/null; then
        print_status "Generating sample requests..."
        for i in {1..10}; do
            curl -s http://localhost:8000/ping > /dev/null &
            curl -s http://localhost:8000/api/v1/cities > /dev/null &
        done
        wait
        print_status "Test load generated"
    else
        print_warning "curl not found. Skipping load generation."
    fi
}

# Main execution
main() {
    print_header "Go AI POI Monitoring Setup"

    check_docker
    create_directories
    start_monitoring
    sleep 15  # Give services time to start
    check_services
    generate_load
    show_urls

    echo ""
    print_status "🎉 Monitoring stack is ready!"
    print_status "Check the Grafana dashboard for metrics and traces."
    echo ""
    echo -e "${YELLOW}💡 Quick Commands:${NC}"
    echo "  • View logs: docker-compose -f docker-compose.monitoring.yml logs -f [service]"
    echo "  • Stop stack: docker-compose -f docker-compose.monitoring.yml down"
    echo "  • Restart: docker-compose -f docker-compose.monitoring.yml restart [service]"
    echo ""
    echo -e "${YELLOW}🔍 pprof Commands:${NC}"
    echo "  • CPU profile: go tool pprof http://localhost:6060/debug/pprof/profile"
    echo "  • Memory heap: go tool pprof http://localhost:6060/debug/pprof/heap"
    echo "  • Goroutines:  go tool pprof http://localhost:6060/debug/pprof/goroutine"
    echo "  • Web UI:      go tool pprof -http=:8081 http://localhost:6060/debug/pprof/profile"
}

# Handle script arguments
case "${1:-}" in
    "start")
        main
        ;;
    "stop")
        print_status "Stopping monitoring stack..."
        docker-compose -f docker-compose.monitoring.yml down
        ;;
    "restart")
        print_status "Restarting monitoring stack..."
        docker-compose -f docker-compose.monitoring.yml restart
        ;;
    "logs")
        service="${2:-}"
        if [ -n "$service" ]; then
            docker-compose -f docker-compose.monitoring.yml logs -f "$service"
        else
            docker-compose -f docker-compose.monitoring.yml logs -f
        fi
        ;;
    "status")
        check_services
        ;;
    *)
        echo "Usage: $0 {start|stop|restart|logs [service]|status}"
        echo ""
        echo "Commands:"
        echo "  start   - Start the complete monitoring stack"
        echo "  stop    - Stop all monitoring services"
        echo "  restart - Restart all monitoring services"
        echo "  logs    - Show logs (optionally for specific service)"
        echo "  status  - Check service health"
        exit 1
        ;;
esac
