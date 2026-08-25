# renovate: datasource=github-tags depName=golangci/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.13.1
# renovate: datasource=github-tags depName=norwoodj/helm-docs
HELM_DOCS_VERSION ?= v1.14.2
# renovate: datasource=github-tags depName=dadav/helm-schema
HELM_SCHEMA_VERSION ?= 0.23.5
# renovate: datasource=docker depName=helmunittest/helm-unittest
HELM_UNITTEST_IMAGE ?= helmunittest/helm-unittest:4.2.3-1.1.2
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

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: unit-test
unit-test: ## Run Go unit tests with coverage report
	go test ./cmd/... ./internal/... -race -shuffle=on -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out -o cover.html

.PHONY: lint
lint: ## Run golangci-lint against Go source
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --fix ./cmd/... ./internal/...

.PHONY: helm-lint
helm-lint: ## Lint the Helm chart
	helm lint charts/gateway-lens

.PHONY: helm-unittest
helm-unittest: ## Run the Helm chart's unit tests (helm-unittest, via Docker)
	docker run --rm --user "$(shell id -u):$(shell id -g)" -e HOME=/tmp -v $(CURDIR):/apps -w /apps $(HELM_UNITTEST_IMAGE) charts/gateway-lens

.PHONY: generate-manifests
generate-manifests: ## Regenerate deploy/*.yaml from the Helm chart's default values
	./scripts/generate-manifests.sh

.PHONY: generate-helm-docs
generate-helm-docs: ## Regenerate the Helm chart's README.md from values.yaml comments
	go run github.com/norwoodj/helm-docs/cmd/helm-docs@$(HELM_DOCS_VERSION) --chart-search-root=charts --template-files README.md.gotmpl

.PHONY: generate-helm-schema
generate-helm-schema: ## Regenerate the Helm chart's values.schema.json from values.yaml comments
	go run github.com/dadav/helm-schema/cmd/helm-schema@$(HELM_SCHEMA_VERSION) --chart-search-root=charts --add-schema-reference "--skip-auto-generation=required,additionalProperties" --append-newline

.PHONY: generate
generate: generate-manifests generate-helm-docs generate-helm-schema ## Regenerate all generated files (manifests, Helm docs, Helm schema)

.PHONY: dashboard-build
dashboard-build: ## Build the dashboard frontend
	cd $(DASHBOARD_DIR) && npm ci && npm run build

.PHONY: dashboard-lint
dashboard-lint: ## Lint the dashboard frontend
	cd $(DASHBOARD_DIR) && npm ci && npm run lint

.PHONY: dashboard-test
dashboard-test: ## Run dashboard frontend tests
	cd $(DASHBOARD_DIR) && npm ci && npm run test

.PHONY: build
build: dashboard-build ## Build the gateway-lens binary
	mkdir -p $(BINDIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags "$(GO_LINKER_FLAGS)" -o $(BINDIR)/$(BINARY) $(CMD_PACKAGE)

.PHONY: image
image: ## Build a local container image
	$(MAKE) build GOOS=linux
	docker build --platform linux/$(GOARCH) --target local -t $(IMAGE):$(TAG) .
