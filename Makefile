APP_NAME := trickster-bot
BIN_DIR  := bin
BINARY   := $(BIN_DIR)/bot
GO       := go
GOFLAGS  := -ldflags="-s -w"

.PHONY: all build run test lint fmt vet clean docker-build docker-up docker-down docker-logs docker-restart help

## — Build & Run ——————————————————————————————

all: build ## Build the binary (default)

build: ## Build binary to bin/bot
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -o $(BINARY) ./cmd/bot/

run: build ## Build and run locally
	$(BINARY)

run-dev: ## Run with go run (no binary)
	$(GO) run ./cmd/bot/

## — Quality ———————————————————————————————————

test: ## Run all tests
	$(GO) test ./... -count=1

test-short: ## Run fast tests (skip long ones)
	$(GO) test ./... -short -count=1

test-cover: ## Run tests with coverage report
	@mkdir -p $(BIN_DIR)
	$(GO) test ./... -count=1 -coverprofile=$(BIN_DIR)/coverage.out
	$(GO) tool cover -func=$(BIN_DIR)/coverage.out | tail -1
	@echo "HTML report: go tool cover -html=$(BIN_DIR)/coverage.out"

lint: ## Run golangci-lint
	golangci-lint run ./...

fmt: ## Format all Go files
	$(GO) fmt ./...
	goimports -w . 2>/dev/null || true

vet: ## Run go vet
	$(GO) vet ./...

check: fmt vet lint test ## Run all checks (fmt + vet + lint + test)

## — Docker ————————————————————————————————————

docker-build: ## Build Docker image
	docker compose build

docker-up: ## Start bot in Docker (detached)
	docker compose up -d

docker-down: ## Stop bot
	docker compose down

docker-restart: ## Restart bot
	docker compose restart

docker-logs: ## Tail bot logs
	docker compose logs -f --tail=50

docker-rebuild: ## Full rebuild and restart
	docker compose down
	docker compose build --no-cache
	docker compose up -d

## — Database ——————————————————————————————————

db-backup: ## Backup SQLite database
	@mkdir -p data/backups
	@cp data/trickster.db "data/backups/trickster-$$(date +%Y%m%d-%H%M%S).db" 2>/dev/null && \
		echo "Backup saved to data/backups/" || echo "No database found"

db-shell: ## Open SQLite shell
	sqlite3 data/trickster.db

## — Misc ——————————————————————————————————————

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)

deps: ## Download dependencies
	$(GO) mod download
	$(GO) mod tidy

update-deps: ## Update all dependencies
	$(GO) get -u ./...
	$(GO) mod tidy

install-tools: ## Install dev tools
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	pip install edge-tts 2>/dev/null || echo "pip not available for edge-tts"

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
