GO ?= go
GOFLAGS ?= -mod=readonly
export GOFLAGS

BINARY := polylens2otel
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X github.com/rknightion/polylens2otel/internal/version.Version=$(VERSION) -X github.com/rknightion/polylens2otel/internal/version.Commit=$(COMMIT) -X github.com/rknightion/polylens2otel/internal/version.BuildDate=$(BUILD_DATE)
TOOLS_DIR := $(CURDIR)/.tools
GOLANGCI_LINT_VERSION ?= v2.12.2
GOVULNCHECK_VERSION ?= v1.1.4
HELM_DOCS_VERSION ?= v1.14.2
export PATH := $(TOOLS_DIR):$(PATH)

.PHONY: build test vet lint fmt govulncheck tidy tidy-check check regen dashboard rules grafana-check helm-docs notices sbom tools coverage install-hooks docker

build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

tools:
	@mkdir -p $(TOOLS_DIR)
	@test -x $(TOOLS_DIR)/golangci-lint || GOBIN=$(TOOLS_DIR) $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@test -x $(TOOLS_DIR)/govulncheck || GOBIN=$(TOOLS_DIR) $(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

lint: tools
	$(TOOLS_DIR)/golangci-lint run

fmt: tools
	$(TOOLS_DIR)/golangci-lint fmt

govulncheck: tools
	$(TOOLS_DIR)/govulncheck ./...

tidy:
	$(GO) mod tidy

tidy-check:
	$(GO) mod tidy -diff

regen:
	$(GO) generate ./internal/configdoc

dashboard:
	cd grafana && python3 build_dashboard.py

rules:
	cd grafana && python3 build_rules.py

grafana-check:
	cd grafana && python3 build_dashboard.py --check
	cd grafana && python3 build_rules.py --check
	cd grafana && python3 -m unittest discover -s tests -t . -q

helm-docs:
	@mkdir -p $(TOOLS_DIR)
	@test -x $(TOOLS_DIR)/helm-docs || GOBIN=$(TOOLS_DIR) $(GO) install github.com/norwoodj/helm-docs/cmd/helm-docs@$(HELM_DOCS_VERSION)
	$(TOOLS_DIR)/helm-docs --chart-search-root charts

notices:
	./scripts/notices.sh

sbom:
	./scripts/sbom.sh

coverage:
	$(GO) test -race -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

install-hooks:
	git config core.hooksPath .githooks

docker:
	docker build --build-arg VERSION=$(VERSION) -t $(BINARY):dev .

check: vet test lint govulncheck tidy-check grafana-check
	$(GO) build ./...
