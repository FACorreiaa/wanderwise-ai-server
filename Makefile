.PHONY: swag-init migrate-db testifylint testifylint-fix benchmark-chat benchmark-chat-quick benchmark-chat-load test-integration
.PHONY: dev debug prod build clean docker-dev docker-debug docker-prod docker-clean
# This Makefile is used to manage various tasks related to the loci AI project.

swag-init:
	swag init -g ./main.go -o ./docs

migrate-db:
	migrate -database "postgresql://postgres:postgres@localhost:5454/loci-dev?sslmode=disable" -path ./app/db/migrations up

testifylint:
	testifylint ./...

testifylint-fix:
	testifylint -fix ./...

static:
	staticcheck ./...

lint: ## Runs linter for .go files
	golangci-lint run --config .golangci.yml
	@echo "Go lint passed successfully"

# Benchmark targets
benchmark-chat: ## Run comprehensive chat streaming benchmarks
	@echo "Running comprehensive chat streaming benchmarks..."
	./scripts/run_chat_benchmark.sh --verbose --iterations 5 --concurrent 2

benchmark-chat-quick: ## Run quick chat streaming benchmarks
	@echo "Running quick chat streaming benchmarks..."
	./scripts/run_chat_benchmark.sh --iterations 2 --concurrent 1

benchmark-chat-load: ## Run load testing with multiple concurrent users
	@echo "Running chat streaming load tests..."
	./scripts/run_chat_benchmark.sh --iterations 10 --concurrent 5 --timeout 120s

benchmark-go: ## Run Go benchmark tests
	@echo "Running Go benchmark tests..."
	go test -bench=BenchmarkChatStreamingRoutes -benchmem -timeout=300s ./internal/api/chat_prompt/

benchmark-concurrent: ## Run concurrent benchmark tests
	@echo "Running concurrent benchmark tests..."
	go test -bench=BenchmarkConcurrentChatRequests -benchmem -timeout=300s ./internal/api/chat_prompt/

test-integration: ## Run integration tests for chat streaming
	@echo "Running chat streaming integration tests..."
	go test -v -tags=integration ./internal/api/chat_prompt/ -run TestChatStreamingRoutes

# === Docker Development Commands ===

dev: ## Run development environment with hot reload
	@echo "Starting development environment with hot reload..."
	docker-compose -f docker-compose.dev.yaml up loci-dev --build

debug: ## Run debug environment with Delve debugger
	@echo "Starting debug environment with Delve debugger on port 2345..."
	docker-compose -f docker-compose.dev.yaml --profile debug up loci-debug --build

prod: ## Run production environment
	@echo "Starting production environment..."
	docker-compose -f docker-compose.yaml up --build

build: ## Build all Docker images
	@echo "Building Docker images..."
	docker-compose -f docker-compose.dev.yaml build
	docker build --target prod -t loci-api:latest .

clean: ## Clean up Docker containers and images
	@echo "Cleaning up Docker containers and images..."
	docker-compose -f docker-compose.dev.yaml down -v
	docker-compose -f docker-compose.yaml down -v
	docker-compose -f docker-compose.microservices.yaml down -v
	docker system prune -f

docker-dev: ## Start development containers in background
	@echo "Starting development containers in background..."
	docker-compose -f docker-compose.dev.yaml up -d

docker-debug: ## Start debug containers in background
	@echo "Starting debug containers in background..."
	docker-compose -f docker-compose.dev.yaml --profile debug up -d

docker-prod: ## Start production containers in background
	@echo "Starting production containers in background..."
	docker-compose -f docker-compose.yaml up -d

docker-clean: ## Stop and remove all containers
	@echo "Stopping and removing all containers..."
	docker-compose -f docker-compose.dev.yaml down
	docker-compose -f docker-compose.yaml down
	docker-compose -f docker-compose.microservices.yaml down

logs: ## Show logs for development containers
	docker-compose -f docker-compose.dev.yaml logs -f loci-dev

logs-debug: ## Show logs for debug containers
	docker-compose -f docker-compose.dev.yaml logs -f loci-debug

# === Microservices Commands ===

microservices: ## Start all microservices in development mode
	@echo "Starting all microservices with hot reload..."
	docker-compose -f docker-compose.microservices.yaml up --build

microservices-bg: ## Start all microservices in background
	@echo "Starting all microservices in background..."
	docker-compose -f docker-compose.microservices.yaml up -d --build

microservices-gateway: ## Start microservices with API gateway
	@echo "Starting microservices with API gateway..."
	docker-compose -f docker-compose.microservices.yaml --profile gateway up --build

microservices-monitoring: ## Start microservices with monitoring stack
	@echo "Starting microservices with monitoring (Prometheus, Jaeger)..."
	docker-compose -f docker-compose.microservices.yaml --profile monitoring up --build

microservices-full: ## Start microservices with all optional services
	@echo "Starting microservices with gateway, monitoring, and service discovery..."
	docker-compose -f docker-compose.microservices.yaml --profile gateway --profile monitoring --profile discovery up --build

# Individual service commands - Core Services
auth-service: ## Start only auth service
	@echo "Starting auth service..."
	docker-compose -f docker-compose.microservices.yaml up auth-service postgres-loci redis --build

poi-service: ## Start only POI service
	@echo "Starting POI service..."
	docker-compose -f docker-compose.microservices.yaml up poi-service postgres-loci redis --build

chat-service: ## Start only chat service
	@echo "Starting chat service..."
	docker-compose -f docker-compose.microservices.yaml up chat-service postgres-loci redis --build

lists-service: ## Start only lists service
	@echo "Starting lists service..."
	docker-compose -f docker-compose.microservices.yaml up lists-service postgres-loci redis --build

users-service: ## Start only users service
	@echo "Starting users service..."
	docker-compose -f docker-compose.microservices.yaml up users-service postgres-loci redis --build

# Individual service commands - Supporting Services
admin-service: ## Start only admin service
	@echo "Starting admin service..."
	docker-compose -f docker-compose.microservices.yaml up admin-service postgres-loci redis --build

city-service: ## Start only city service
	@echo "Starting city service..."
	docker-compose -f docker-compose.microservices.yaml up city-service postgres-loci redis --build

interests-service: ## Start only interests service
	@echo "Starting interests service..."
	docker-compose -f docker-compose.microservices.yaml up interests-service postgres-loci redis --build

profiles-service: ## Start only profiles service
	@echo "Starting profiles service..."
	docker-compose -f docker-compose.microservices.yaml up profiles-service postgres-loci redis --build

recents-service: ## Start only recents service
	@echo "Starting recents service..."
	docker-compose -f docker-compose.microservices.yaml up recents-service postgres-loci redis --build

reviews-service: ## Start only reviews service
	@echo "Starting reviews service..."
	docker-compose -f docker-compose.microservices.yaml up reviews-service postgres-loci redis --build

statistics-service: ## Start only statistics service
	@echo "Starting statistics service..."
	docker-compose -f docker-compose.microservices.yaml up statistics-service postgres-loci redis --build

tags-service: ## Start only tags service
	@echo "Starting tags service..."
	docker-compose -f docker-compose.microservices.yaml up tags-service postgres-loci redis --build

# Build individual services - Core Services
build-auth: ## Build auth service Docker image
	docker-compose -f docker-compose.microservices.yaml build auth-service

build-poi: ## Build POI service Docker image
	docker-compose -f docker-compose.microservices.yaml build poi-service

build-chat: ## Build chat service Docker image
	docker-compose -f docker-compose.microservices.yaml build chat-service

build-lists: ## Build lists service Docker image
	docker-compose -f docker-compose.microservices.yaml build lists-service

build-users: ## Build users service Docker image
	docker-compose -f docker-compose.microservices.yaml build users-service

# Build individual services - Supporting Services
build-admin: ## Build admin service Docker image
	docker-compose -f docker-compose.microservices.yaml build admin-service

build-city: ## Build city service Docker image
	docker-compose -f docker-compose.microservices.yaml build city-service

build-interests: ## Build interests service Docker image
	docker-compose -f docker-compose.microservices.yaml build interests-service

build-profiles: ## Build profiles service Docker image
	docker-compose -f docker-compose.microservices.yaml build profiles-service

build-recents: ## Build recents service Docker image
	docker-compose -f docker-compose.microservices.yaml build recents-service

build-reviews: ## Build reviews service Docker image
	docker-compose -f docker-compose.microservices.yaml build reviews-service

build-statistics: ## Build statistics service Docker image
	docker-compose -f docker-compose.microservices.yaml build statistics-service

build-tags: ## Build tags service Docker image
	docker-compose -f docker-compose.microservices.yaml build tags-service

build-all-services: ## Build all microservice images
	@echo "Building all microservice images..."
	docker-compose -f docker-compose.microservices.yaml build

# Service logs - Core Services
logs-auth: ## Show auth service logs
	docker-compose -f docker-compose.microservices.yaml logs -f auth-service

logs-poi: ## Show POI service logs
	docker-compose -f docker-compose.microservices.yaml logs -f poi-service

logs-chat: ## Show chat service logs
	docker-compose -f docker-compose.microservices.yaml logs -f chat-service

logs-lists: ## Show lists service logs
	docker-compose -f docker-compose.microservices.yaml logs -f lists-service

logs-users: ## Show users service logs
	docker-compose -f docker-compose.microservices.yaml logs -f users-service

# Service logs - Supporting Services
logs-admin: ## Show admin service logs
	docker-compose -f docker-compose.microservices.yaml logs -f admin-service

logs-city: ## Show city service logs
	docker-compose -f docker-compose.microservices.yaml logs -f city-service

logs-interests: ## Show interests service logs
	docker-compose -f docker-compose.microservices.yaml logs -f interests-service

logs-profiles: ## Show profiles service logs
	docker-compose -f docker-compose.microservices.yaml logs -f profiles-service

logs-recents: ## Show recents service logs
	docker-compose -f docker-compose.microservices.yaml logs -f recents-service

logs-reviews: ## Show reviews service logs
	docker-compose -f docker-compose.microservices.yaml logs -f reviews-service

logs-statistics: ## Show statistics service logs
	docker-compose -f docker-compose.microservices.yaml logs -f statistics-service

logs-tags: ## Show tags service logs
	docker-compose -f docker-compose.microservices.yaml logs -f tags-service

logs-all: ## Show all microservice logs
	docker-compose -f docker-compose.microservices.yaml logs -f

# Debug individual services (each gets unique debug port)
debug-auth: ## Debug auth service (port 2345)
	@echo "Starting auth service in debug mode (Delve on port 2345)..."
	docker-compose -f docker-compose.microservices.yaml run --rm -p 2345:2345 auth-service dlv debug --headless --listen=:2345 --api-version=2 --accept-multiclient

debug-poi: ## Debug POI service (port 2346)
	@echo "Starting POI service in debug mode (Delve on port 2346)..."
	docker-compose -f docker-compose.microservices.yaml run --rm -p 2346:2345 poi-service dlv debug --headless --listen=:2345 --api-version=2 --accept-multiclient

debug-chat: ## Debug chat service (port 2347)
	@echo "Starting chat service in debug mode (Delve on port 2347)..."
	docker-compose -f docker-compose.microservices.yaml run --rm -p 2347:2345 chat-service dlv debug --headless --listen=:2345 --api-version=2 --accept-multiclient

debug-lists: ## Debug lists service (port 2348)
	@echo "Starting lists service in debug mode (Delve on port 2348)..."
	docker-compose -f docker-compose.microservices.yaml run --rm -p 2348:2345 lists-service dlv debug --headless --listen=:2345 --api-version=2 --accept-multiclient

debug-users: ## Debug users service (port 2349)
	@echo "Starting users service in debug mode (Delve on port 2349)..."
	docker-compose -f docker-compose.microservices.yaml run --rm -p 2349:2345 users-service dlv debug --headless --listen=:2345 --api-version=2 --accept-multiclient

debug-admin: ## Debug admin service (port 2350)
	@echo "Starting admin service in debug mode (Delve on port 2350)..."
	docker-compose -f docker-compose.microservices.yaml run --rm -p 2350:2345 admin-service dlv debug --headless --listen=:2345 --api-version=2 --accept-multiclient

debug-city: ## Debug city service (port 2351)
	@echo "Starting city service in debug mode (Delve on port 2351)..."
	docker-compose -f docker-compose.microservices.yaml run --rm -p 2351:2345 city-service dlv debug --headless --listen=:2345 --api-version=2 --accept-multiclient

debug-interests: ## Debug interests service (port 2352)
	@echo "Starting interests service in debug mode (Delve on port 2352)..."
	docker-compose -f docker-compose.microservices.yaml run --rm -p 2352:2345 interests-service dlv debug --headless --listen=:2345 --api-version=2 --accept-multiclient

debug-profiles: ## Debug profiles service (port 2353)
	@echo "Starting profiles service in debug mode (Delve on port 2353)..."
	docker-compose -f docker-compose.microservices.yaml run --rm -p 2353:2345 profiles-service dlv debug --headless --listen=:2345 --api-version=2 --accept-multiclient

debug-recents: ## Debug recents service (port 2354)
	@echo "Starting recents service in debug mode (Delve on port 2354)..."
	docker-compose -f docker-compose.microservices.yaml run --rm -p 2354:2345 recents-service dlv debug --headless --listen=:2345 --api-version=2 --accept-multiclient

debug-reviews: ## Debug reviews service (port 2355)
	@echo "Starting reviews service in debug mode (Delve on port 2355)..."
	docker-compose -f docker-compose.microservices.yaml run --rm -p 2355:2345 reviews-service dlv debug --headless --listen=:2345 --api-version=2 --accept-multiclient

debug-statistics: ## Debug statistics service (port 2356)
	@echo "Starting statistics service in debug mode (Delve on port 2356)..."
	docker-compose -f docker-compose.microservices.yaml run --rm -p 2356:2345 statistics-service dlv debug --headless --listen=:2345 --api-version=2 --accept-multiclient

debug-tags: ## Debug tags service (port 2357)
	@echo "Starting tags service in debug mode (Delve on port 2357)..."
	docker-compose -f docker-compose.microservices.yaml run --rm -p 2357:2345 tags-service dlv debug --headless --listen=:2345 --api-version=2 --accept-multiclient

help: ## Show this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
