set shell := ["bash", "-euo", "pipefail", "-c"]

tools_dir := justfile_directory() / ".tools"
export PATH := tools_dir + ":" + env_var("PATH")

# renovate: datasource=github-tags depName=golangci/golangci-lint
golangci_lint_version := "v2.13.2"
# renovate: datasource=github-tags depName=golang/vuln
govulncheck_version := "v1.3.0"
# renovate: datasource=github-tags depName=norwoodj/helm-docs
helm_docs_version := "v1.14.2"

version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
commit := `git rev-parse HEAD 2>/dev/null || echo unknown`
build_date := `date -u +%Y-%m-%dT%H:%M:%SZ`
ldflags := "-s -w -X github.com/rknightion/polylens2otel/internal/version.Version=" + version + " -X github.com/rknightion/polylens2otel/internal/version.Commit=" + commit + " -X github.com/rknightion/polylens2otel/internal/version.BuildDate=" + build_date

# Show the task surface.
default:
    @just --list

# Install idempotent repository-local development tooling.
setup:
    mkdir -p {{ tools_dir }}
    test -x {{ tools_dir }}/golangci-lint || GOBIN={{ tools_dir }} go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@{{ golangci_lint_version }}
    test -x {{ tools_dir }}/govulncheck || GOBIN={{ tools_dir }} go install golang.org/x/vuln/cmd/govulncheck@{{ govulncheck_version }}
    test -x {{ tools_dir }}/helm-docs || GOBIN={{ tools_dir }} go install github.com/norwoodj/helm-docs/cmd/helm-docs@{{ helm_docs_version }}

# Format Go source in place.
[group('check')]
fmt: setup
    golangci-lint fmt

# Verify formatting without modifying files.
[group('check')]
[no-exit-message]
fmt-check: setup
    just --fmt --check
    golangci-lint fmt --diff

# Run static analysis.
[group('check')]
[no-exit-message]
lint: setup
    go vet ./...
    golangci-lint run

# Scan dependencies for known vulnerabilities.
[group('check')]
[no-exit-message]
audit: setup
    govulncheck ./...

# Verify module metadata is tidy.
[group('check')]
[no-exit-message]
tidy-check:
    go mod tidy -diff

# Run the Grafana asset test suite.
[no-exit-message]
[private]
_grafana-test:
    cd grafana && python3 -m unittest discover -s tests -t . -q

# Run the complete Go, task-surface, and Grafana test suites.
[group('check')]
test filter="": _grafana-test
    go test -race {{ if filter == "" { "./..." } else { "-run " + filter + " ./..." } }}
    python3 -m unittest discover -s scripts -p 'test_*.py' -q

# Scan tracked files for secrets and sensitive local paths.
[group('check')]
[no-exit-message]
secret-hygiene:
    ./scripts/check-secret-hygiene.sh

# Verify documented commands resolve to supported entry points.
[group('check')]
[no-exit-message]
doc-commands:
    python3 scripts/check_doc_commands.py

# Compile every package without producing a release binary.
[no-exit-message]
[private]
_compile-check:
    go build ./...

# Verify generated configuration documentation has no drift.
[no-exit-message]
[private]
_configdoc-check:
    go run ./internal/configdoc --check

# Verify generated documentation, dashboards, and rules have no drift.
[group('gen')]
[no-exit-message]
gen-check: _configdoc-check
    cd grafana && python3 build_dashboard.py --check
    cd grafana && python3 build_rules.py --check

# Run the complete local pre-commit gate.
[group('check')]
check: fmt-check lint audit tidy-check gen-check test secret-hygiene doc-commands _compile-check

# Compile the release binary to bin/polylens2otel.
[group('build')]
build:
    go build -trimpath -ldflags "{{ ldflags }}" -o bin/polylens2otel ./cmd/polylens2otel

# Build the development container image without pushing it.
[group('build')]
image tag="polylens2otel:dev":
    docker build --build-arg VERSION={{ version }} --build-arg COMMIT={{ commit }} --build-arg BUILD_DATE={{ build_date }} -t {{ tag }} .

# Generate the SPDX SBOM at dist/polylens2otel.spdx.json.
[group('build')]
sbom:
    command -v syft >/dev/null || { echo "syft is required" >&2; exit 2; }
    mkdir -p dist
    syft dir:. -o spdx-json=dist/polylens2otel.spdx.json

# Validate the dependency inventory.
[group('build')]
notices:
    go list -m -json all > /dev/null
    @echo "dependency inventory validated; install go-licenses to generate THIRD_PARTY_NOTICES.md"

# Regenerate committed configuration and Grafana artifacts.
[group('gen')]
gen:
    go generate ./internal/configdoc
    cd grafana && python3 build_dashboard.py
    cd grafana && python3 build_rules.py

# Regenerate the Helm chart README.
[group('gen')]
docs: setup
    helm-docs --chart-search-root charts

# Apply Go module metadata updates.
# Point git at the tracked .githooks/ pre-commit gate.
[group('dev')]
install-hooks:
    git config core.hooksPath .githooks
    @echo "core.hooksPath -> .githooks"

[group('dev')]
deps-update:
    go mod tidy

# Produce a race-enabled Go coverage report.
[group('check')]
coverage:
    go test -race -coverprofile=coverage.out ./...
    go tool cover -func=coverage.out

# Remove reproducible build and tooling artifacts.
[group('build')]
clean:
    rm -rf bin dist coverage.out {{ tools_dir }}
