.PHONY: build run test clean docker-up docker-down migrate bench help

APP_NAME=location-tracker
BUILD_DIR=bin

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build: ## Build the producer and consumer
	@echo "Building producer and consumer..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/producer ./api
	go build -o $(BUILD_DIR)/consumer ./worker

run: build ## Run the stack locally (requires Kafka/Redis/Cassandra)
	@echo "Starting the application..."
	./$(BUILD_DIR)/producer & ./$(BUILD_DIR)/consumer

test: ## Run unit tests
	@echo "Running tests..."
	go test -v ./...

clean: ## Remove build artifacts
	@echo "Cleaning gems..."
	rm -rf $(BUILD_DIR)

docker-up: ## Start infrastructure with Docker Compose
	@echo "Starting Docker containers..."
	docker-compose up -d

docker-down: ## Stop Docker containers
	@echo "Stopping Docker containers..."
	docker-compose down

migrate: ## Run Cassandra migrations manually
	@echo "Running Cassandra migrations..."
	docker exec -i cassandra cqlsh < migrations.cql

bench: ## Run benchmarks
	@echo "Running benchmarks..."
	go test -bench=. ./...

mock-data: ## Send some mock GPS data
	@echo "Sending mock GPS data..."
	curl -X POST http://localhost:8080/location -d '{"driver_id":"d1", "lat":12.97, "lng":77.59, "speed":40}'
	curl -X POST http://localhost:8080/location -d '{"driver_id":"d2", "lat":12.98, "lng":77.60, "speed":45}'
