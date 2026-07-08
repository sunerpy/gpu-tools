SHELL = /bin/bash

PROJECT_NAME := gpu-tools
MODULE_NAME := github.com/sunerpy/gpu-tools
PROJECT_ROOT := $(abspath .)
DIST_DIR := dist
PLATFORMS := linux/amd64 linux/arm64 windows/amd64 windows/arm64 darwin/amd64 darwin/arm64

TAG ?= $(shell tag=$$(git describe --tags --abbrev=0 2>/dev/null || true); if [ -z "$$tag" ]; then tag=v0.0.0; fi; printf '%s' "$$tag" | tr -s '[:space:]' '-')
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT_ID := $(shell commit=$$(git rev-parse --verify --short HEAD 2>/dev/null || true); if [ -z "$$commit" ]; then commit=unknown; fi; printf '%s' "$$commit" | tr -s '[:space:]' '-')
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
BUILD_GOOS := $(shell printf '%s' "$(GOOS)" | tr -s '[:space:]' '-')
BUILD_GOARCH := $(shell printf '%s' "$(GOARCH)" | tr -s '[:space:]' '-')

GO_FILES := $(shell find . -name "*.go" -not -path "./vendor/*")
OXFMT_IGNORE := --ignore-path "$(PROJECT_ROOT)/.oxfmtignore"

# oxfmt is invoked as a standalone binary. Install it with:
#   cargo install oxfmt
# or:
#   npm install -g oxfmt

.DEFAULT_GOAL := help

.PHONY: help build build-binaries fmt fmt-go fmt-oxfmt fmt-check lint test coverage coverage-gate coverage-parity clean

help:
	@echo "Available targets:"
	@echo "  build          Build static local binary into dist/gpu-tools"
	@echo "  build-binaries Build static binaries for $(PLATFORMS)"
	@echo "  fmt            Format Go and YAML/JSON/Markdown files"
	@echo "  fmt-check      Verify formatting without writing"
	@echo "  lint           Run golangci-lint v2, fallback to go vet if missing"
	@echo "  test           Run race tests with atomic coverage"
	@echo "  coverage       Generate coverage.html from coverage.out"
	@echo "  coverage-gate  Enforce filtered total coverage >= 95%"
	@echo "  coverage-parity Verify Codecov ignore set mirrors coverage-gate exclusions"
	@echo "  clean          Remove build and coverage artifacts"

build:
	@echo "Building $(PROJECT_NAME) for $(BUILD_GOOS)/$(BUILD_GOARCH) with CGO_ENABLED=0"
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 go build -ldflags "-s -w \
		-X $(MODULE_NAME)/version.Version=$(TAG) \
		-X $(MODULE_NAME)/version.BuildTime=$(BUILD_TIME) \
		-X $(MODULE_NAME)/version.CommitID=$(COMMIT_ID) \
		-X $(MODULE_NAME)/version.BuildOS=$(BUILD_GOOS) \
		-X $(MODULE_NAME)/version.BuildArch=$(BUILD_GOARCH)" \
		-o $(DIST_DIR)/$(PROJECT_NAME) .

build-binaries:
	@echo "Building binaries for platforms: $(PLATFORMS)"
	@mkdir -p $(DIST_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$$(echo $$platform | cut -d/ -f1); \
		GOARCH=$$(echo $$platform | cut -d/ -f2); \
		OUTPUT=$(DIST_DIR)/$(PROJECT_NAME)-$$GOOS-$$GOARCH; \
		if [ "$$GOOS" = "windows" ]; then OUTPUT=$$OUTPUT.exe; fi; \
		echo "Building for $$platform -> $$OUTPUT"; \
		GOOS=$$GOOS GOARCH=$$GOARCH CGO_ENABLED=0 go build -ldflags "-s -w \
			-X $(MODULE_NAME)/version.Version=$(TAG) \
			-X $(MODULE_NAME)/version.BuildTime=$(BUILD_TIME) \
			-X $(MODULE_NAME)/version.CommitID=$(COMMIT_ID) \
			-X $(MODULE_NAME)/version.BuildOS=$$GOOS \
			-X $(MODULE_NAME)/version.BuildArch=$$GOARCH" \
			-o $$OUTPUT . || exit 1; \
	done

fmt: fmt-go fmt-oxfmt
	@echo "Formatting complete."

fmt-go:
	@echo "Formatting Go code..."
	@if command -v goimports > /dev/null 2>&1; then \
		echo "$(GO_FILES)" | tr ' ' '\n' | xargs -r -P 4 goimports -w -local $(MODULE_NAME); \
	else \
		echo "goimports not found. Install with:"; \
		echo "  go install golang.org/x/tools/cmd/goimports@latest"; \
	fi
	@if command -v gofumpt > /dev/null 2>&1; then \
		echo "$(GO_FILES)" | tr ' ' '\n' | xargs -r -P 4 gofumpt -extra -w; \
	else \
		echo "gofumpt not found. Install with:"; \
		echo "  go install mvdan.cc/gofumpt@latest"; \
	fi

fmt-oxfmt:
	@echo "Formatting YAML/JSON/Markdown with oxfmt..."
	@if command -v oxfmt > /dev/null 2>&1; then \
		oxfmt --write --no-error-on-unmatched-pattern $(OXFMT_IGNORE) "$(PROJECT_ROOT)"; \
	else \
		echo "oxfmt not found. Install with:"; \
		echo "  cargo install oxfmt"; \
		echo "  # or: npm install -g oxfmt"; \
	fi

fmt-check:
	@echo "Checking Go format..."
	@if ! command -v goimports > /dev/null 2>&1; then \
		echo "goimports not found. Install with: go install golang.org/x/tools/cmd/goimports@latest"; \
		exit 1; \
	fi
	@unformatted=$$(echo "$(GO_FILES)" | tr ' ' '\n' | xargs -r gofmt -l); \
	if [ -n "$$unformatted" ]; then \
		echo "Go files need gofmt:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	@imports=$$(echo "$(GO_FILES)" | tr ' ' '\n' | xargs -r goimports -l -local $(MODULE_NAME)); \
	if [ -n "$$imports" ]; then \
		echo "Go files need goimports:"; \
		echo "$$imports"; \
		exit 1; \
	fi
	@echo "Checking YAML/JSON/Markdown format with oxfmt..."
	@if ! command -v oxfmt > /dev/null 2>&1; then \
		echo "oxfmt not found. Install with: cargo install oxfmt  # or: npm install -g oxfmt"; \
		exit 1; \
	fi
	@oxfmt --check --no-error-on-unmatched-pattern $(OXFMT_IGNORE) "$(PROJECT_ROOT)"

lint:
	@echo "Running Go linters..."
	@if command -v golangci-lint > /dev/null 2>&1; then \
		golangci-lint run --no-config --timeout=5m --tests=false --enable-only=errcheck,govet,ineffassign,staticcheck,unused,misspell,unconvert,bodyclose; \
	else \
		echo "golangci-lint not found. Install with:"; \
		echo "  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"; \
		echo "Running go vet instead..."; \
		go vet ./...; \
	fi

test:
	@echo "Running tests with race detector..."
	# Race detector requires cgo; shipped build artifacts stay CGO_ENABLED=0.
	CGO_ENABLED=1 go test ./... -count=1 -race -covermode=atomic -coverprofile=coverage.out

coverage: test
	@echo "Generating coverage.html..."
	go tool cover -html=coverage.out -o coverage.html

coverage-gate: test
	@echo "Filtering coverage data..."
	@grep -Ev '(_test\.go|/mocks/|(^|/)main\.go:|(^|/)version/|internal/gpu/nvml/purego_lib\.go)' coverage.out > coverage.filtered.out
	@echo "=== Filtered coverage (excluding _test.go, /mocks/, main.go, version/, internal/gpu/nvml/purego_lib.go) ==="
	@go tool cover -func=coverage.filtered.out | tee coverage.filtered.txt
	@awk '/^total:/ { gsub(/%/, "", $$3); if (($$3 + 0) < 95) { printf "coverage gate failed: %.1f%% < 95%%\n", $$3; exit 1 } printf "coverage gate passed: %.1f%% >= 95%%\n", $$3 }' coverage.filtered.txt

coverage-parity:
	@echo "Checking Codecov/coverage-gate exclusion parity..."
	@set -euo pipefail; \
		codecov_expected=('**/mocks/**' 'main.go' 'version/**' '**/*_test.go' 'internal/gpu/nvml/purego_lib.go'); \
		makefile_expected=('_test\.go' '/mocks/' 'main\.go' 'version/' 'internal/gpu/nvml/purego_lib\.go'); \
		for pattern in "$${codecov_expected[@]}"; do \
			if ! grep -F -- "$$pattern" codecov.yml > /dev/null; then \
				echo "coverage parity failed: codecov.yml missing $$pattern"; \
				exit 1; \
			fi; \
		done; \
		for pattern in "$${makefile_expected[@]}"; do \
			if ! grep -F -- "$$pattern" Makefile > /dev/null; then \
				echo "coverage parity failed: Makefile coverage-gate missing $$pattern"; \
				exit 1; \
			fi; \
		done; \
		ignore_count=$$(grep -E '^[[:space:]]*-[[:space:]]+"' codecov.yml | wc -l); \
		if [ "$$ignore_count" -ne 5 ]; then \
			echo "coverage parity failed: codecov.yml ignore count $$ignore_count != 5"; \
			exit 1; \
		fi; \
		echo "coverage parity passed: Codecov ignore set mirrors Makefile coverage-gate exclusions"

clean:
	@echo "Cleaning build and coverage artifacts..."
	rm -rf $(DIST_DIR) coverage.out coverage.html coverage.filtered.out coverage.filtered.txt
