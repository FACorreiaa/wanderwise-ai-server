.PHONY: swag-init migrate-db testifylint testifylint-fix benchmark-chat benchmark-chat-quick benchmark-chat-load test-integration
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
