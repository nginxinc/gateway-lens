# renovate: datasource=github-tags depName=golangci/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.12.1
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || printf 'none')
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
BINDIR ?= bin
BINARY ?= gateway-lens
MODULE ?= $(shell go list -m)
CMD_PACKAGE ?= $(MODULE)/cmd/gateway-lens
DASHBOARD_DIR ?= dashboard
IMAGE ?= gateway-lens
TAG ?= $(VERSION)
GO_LDFLAGS = -X main.version=$(VERSION) -X main.commit=$(COMMIT)
GO_LINKER_FLAGS_OPTIMIZATIONS = -s -w
GO_LINKER_FLAGS = $(GO_LINKER_FLAGS_OPTIMIZATIONS) $(GO_LDFLAGS)

.PHONY: help unit-test lint build image dashboard-build dashboard-lint dashboard-test

.DEFAULT_GOAL := help

help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

unit-test: ## Run Go unit tests with coverage report
	go test ./cmd/... ./internal/... -race -shuffle=on -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out -o cover.html

lint: ## Run golangci-lint against Go source
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --fix ./cmd/... ./internal/...

dashboard-build: ## Build the dashboard frontend
	cd $(DASHBOARD_DIR) && npm ci && npm run build

dashboard-lint: ## Lint the dashboard frontend
	cd $(DASHBOARD_DIR) && npm ci && npm run lint

dashboard-test: ## Run dashboard frontend tests
	cd $(DASHBOARD_DIR) && npm ci && npm run test

build: dashboard-build ## Build the gateway-lens binary
	mkdir -p $(BINDIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags "$(GO_LINKER_FLAGS)" -o $(BINDIR)/$(BINARY) $(CMD_PACKAGE)

image: ## Build a local container image
	$(MAKE) build GOOS=linux
	docker build --platform linux/$(GOARCH) --target local -t $(IMAGE):$(TAG) .
