#!/bin/bash

# Run Go AI POI with pprof enabled locally
# This script helps test pprof without Docker

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

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if Go is installed
check_go() {
    if ! command -v go > /dev/null 2>&1; then
        print_error "Go is not installed or not in PATH"
        exit 1
    fi
    print_info "Go version: $(go version)"
}

# Check if .env file exists
check_env() {
    if [ ! -f ".env" ]; then
        print_warning ".env file not found, using default environment variables"
        print_info "Creating basic .env file..."
        cat > .env << EOF
# Basic environment variables for local development
APP_ENV=development
ENABLE_PPROF=true
PPROF_PORT=6060

# Database (adjust if needed)
POSTGRES_HOST=localhost
POSTGRES_PORT=5454
POSTGRES_DB=go_ai_poi
POSTGRES_USER=user
POSTGRES_PASSWORD=password

# JWT (use your actual values)
JWT_SECRET_KEY=your-secret-key-here
JWT_ISSUER=loci
JWT_AUDIENCE=loci-users

# Server
HTTP_PORT=8000
METRICS_PORT=9090
EOF
        print_info "Created .env file with defaults. Please review and update as needed."
    else
        print_info "Found .env file"
    fi
}

# Start PostgreSQL if needed
start_postgres() {
    if ! pg_isready -h localhost -p 5454 > /dev/null 2>&1; then
        print_warning "PostgreSQL is not running on localhost:5454"
        print_info "You can start it with Docker:"
        echo "  docker run -d --name postgres-dev -p 5454:5432 -e POSTGRES_DB=go_ai_poi -e POSTGRES_USER=user -e POSTGRES_PASSWORD=password pgvector/pgvector:pg16"
        echo ""
        print_info "Or start the monitoring stack which includes PostgreSQL:"
        echo "  ./scripts/monitoring-setup.sh start"
        echo ""
        read -p "Continue anyway? (y/n): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    else
        print_info "PostgreSQL is running on localhost:5454"
    fi
}

# Download dependencies
download_deps() {
    print_header "Downloading Dependencies"
    go mod download
    go mod verify
}

# Run the application
run_app() {
    print_header "Starting Go AI POI with pprof"
    
    # Set environment variables
    export ENABLE_PPROF=true
    export PPROF_PORT=6060
    export APP_ENV=development
    
    print_info "Environment variables:"
    echo "  ENABLE_PPROF=$ENABLE_PPROF"
    echo "  PPROF_PORT=$PPROF_PORT" 
    echo "  APP_ENV=$APP_ENV"
    echo ""
    
    print_info "Starting application..."
    print_info "pprof will be available at: http://localhost:6060/debug/pprof/"
    print_info "API will be available at: http://localhost:8000"
    print_info "Metrics will be available at: http://localhost:9090/metrics"
    echo ""
    print_info "Press Ctrl+C to stop"
    echo ""
    
    # Run the application
    go run main.go
}

# Main execution
main() {
    print_header "Go AI POI - Local Development with pprof"
    
    check_go
    check_env
    start_postgres
    download_deps
    run_app
}

# Handle script arguments
case "${1:-}" in
    "run")
        main
        ;;
    "deps")
        download_deps
        ;;
    "check")
        check_go
        check_env
        start_postgres
        print_info "All checks passed!"
        ;;
    *)
        echo "Usage: $0 {run|deps|check}"
        echo ""
        echo "Commands:"
        echo "  run   - Run the application with pprof enabled"
        echo "  deps  - Download Go dependencies"
        echo "  check - Check prerequisites"
        echo ""
        echo "Quick start:"
        echo "  $0 run"
        exit 1
        ;;
esac