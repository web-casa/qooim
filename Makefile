SHELL := /usr/bin/env bash

GO        ?= go
PKG       := ./...
BIN_DIR   := bin
SERVER    := $(BIN_DIR)/qooim-server
QOOIM     := $(BIN_DIR)/qooim

VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -X github.com/web-casa/qooim/internal/cli.Version=$(VERSION)

.PHONY: help build run test e2e e2e-pg lint fmt tidy migrate-up migrate-down migrate-status clean

help: ## Show available targets
	@awk 'BEGIN{FS=":.*##"; printf "\nTargets:\n"} /^[a-zA-Z_-]+:.*##/{printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build qooim-server and qooim CLI into ./bin
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(SERVER) ./cmd/server
	$(GO) build -ldflags "$(LDFLAGS)" -o $(QOOIM)  ./cmd/qooim

run: ## Run server with sensible dev defaults (no DB)
	QOOIM_HTTP_ADDR=":18080" $(GO) run ./cmd/server

test: ## Run unit + in-process e2e tests (no Docker required)
	$(GO) test -race -timeout 120s $(PKG)

e2e: test ## Alias for `make test`

e2e-pg: ## Run Postgres-backed integration tests (requires Docker or QOOIM_TEST_DSN)
	$(GO) test -tags pg -race -timeout 5m ./tests/...

lint: ## Run golangci-lint
	@command -v golangci-lint >/dev/null || { echo "install golangci-lint: https://golangci-lint.run/usage/install/"; exit 1; }
	golangci-lint run

fmt: ## Format code
	$(GO) fmt $(PKG)
	@command -v goimports >/dev/null && goimports -w . || true

tidy: ## go mod tidy
	$(GO) mod tidy

migrate-up: ## Apply pending migrations (requires QOOIM_DB_DSN, postgres URL)
	@test -n "$$QOOIM_DB_DSN" || { echo "set QOOIM_DB_DSN, e.g. postgresql://user:pass@host:5432/db?sslmode=disable"; exit 1; }
	$(GO) run github.com/pressly/goose/v3/cmd/goose@latest -dir ./migrations postgres "$$QOOIM_DB_DSN" up

migrate-down: ## Roll back last migration
	@test -n "$$QOOIM_DB_DSN" || { echo "set QOOIM_DB_DSN"; exit 1; }
	$(GO) run github.com/pressly/goose/v3/cmd/goose@latest -dir ./migrations postgres "$$QOOIM_DB_DSN" down

migrate-status: ## Show applied/pending migrations
	@test -n "$$QOOIM_DB_DSN" || { echo "set QOOIM_DB_DSN"; exit 1; }
	$(GO) run github.com/pressly/goose/v3/cmd/goose@latest -dir ./migrations postgres "$$QOOIM_DB_DSN" status

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
