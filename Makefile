SHELL := /usr/bin/env bash

GO        ?= go
PKG       := ./...
BIN_DIR   := bin
SERVER    := $(BIN_DIR)/exam-run
SKCTL     := $(BIN_DIR)/skctl

VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -X github.com/ivmm/exam-run/cmd/skctl/cmd.Version=$(VERSION)

.PHONY: help build run test e2e e2e-mysql lint fmt tidy migrate-up migrate-down clean

help: ## Show available targets
	@awk 'BEGIN{FS=":.*##"; printf "\nTargets:\n"} /^[a-zA-Z_-]+:.*##/{printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build server and skctl into ./bin
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(SERVER) ./cmd/server
	$(GO) build -ldflags "$(LDFLAGS)" -o $(SKCTL) ./cmd/skctl

run: ## Run server with sensible dev defaults (no DB)
	EXAMRUN_HTTP_ADDR=":18080" $(GO) run ./cmd/server

test: ## Run unit + in-process e2e tests (no Docker required)
	$(GO) test -race -timeout 120s $(PKG)

e2e: test ## Alias for `make test`

e2e-mysql: ## Run MySQL-backed integration tests (requires Docker)
	$(GO) test -tags mysql -race -timeout 5m ./tests/...

lint: ## Run golangci-lint
	@command -v golangci-lint >/dev/null || { echo "install golangci-lint: https://golangci-lint.run/usage/install/"; exit 1; }
	golangci-lint run

fmt: ## Format code
	$(GO) fmt $(PKG)
	@command -v goimports >/dev/null && goimports -w . || true

tidy: ## go mod tidy
	$(GO) mod tidy

migrate-up: ## Apply pending migrations (requires DSN env var)
	@test -n "$$EXAMRUN_DB_DSN" || { echo "set EXAMRUN_DB_DSN, e.g. user:pass@tcp(localhost:3306)/examrun?parseTime=true&multiStatements=true"; exit 1; }
	$(GO) run github.com/pressly/goose/v3/cmd/goose@latest -dir ./migrations mysql "$$EXAMRUN_DB_DSN" up

migrate-down: ## Roll back last migration
	@test -n "$$EXAMRUN_DB_DSN" || { echo "set EXAMRUN_DB_DSN"; exit 1; }
	$(GO) run github.com/pressly/goose/v3/cmd/goose@latest -dir ./migrations mysql "$$EXAMRUN_DB_DSN" down

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
