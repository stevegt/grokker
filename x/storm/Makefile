.PHONY: help build run serve swag clean test

help:
	@echo "Storm - Multi-project LLM chat application"
	@echo ""
	@echo "Available targets:"
	@echo "  make build       - Build the storm binary"
	@echo "  make serve       - Run the storm server on port 8080"
	@echo "  make swag        - Generate Swagger/OpenAPI documentation"
	@echo "  make dev         - Run with automatic documentation generation"
	@echo "  make clean       - Remove build artifacts"
	@echo "  make test        - Run tests"

build:
	go build -o bin/storm .

serve: swag
	./bin/storm serve --port 8080

swag:
	swag init

dev: swag
	@echo "Building and running Storm server..."
	go run . serve --port 8080

clean:
	rm -rf bin/
	rm -rf docs/

test:
	go test -v ./...

.DEFAULT_GOAL := help
