OPENAPI_SPEC_URL ?= https://raw.githubusercontent.com/d-yoshi/redmine-openapi/main/openapi.yaml
OPENAPI_SPEC_DIR ?= openapi
OPENAPI_SPEC ?= $(OPENAPI_SPEC_DIR)/openapi.yaml
GO ?= go
BINARY ?= redmine
BIN_DIR ?= bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS ?= -s -w -X main.version=$(VERSION)
GOFILES := $(shell find . -name '*.go' -not -path './vendor/*')

.DEFAULT_GOAL := help

.PHONY: help download-openapi-spec build install lint fix test gen gen-docs check-docs check-gen check clean
help:
	@echo "Targets:"
	@echo "  download-openapi-spec  Download latest Redmine OpenAPI spec to $(OPENAPI_SPEC)"
	@echo "  build                  Compile $(BINARY)"
	@echo "  install                Install $(BINARY)"
	@echo "  lint                   Run golangci-lint when available, otherwise go vet"
	@echo "  fix                    Run gofmt and goimports"
	@echo "  test                   Run unit tests"
	@echo "  gen                    Regenerate OpenAPI command metadata and docs"
	@echo "  gen-docs               Regenerate docs/usage.md and generated metadata"
	@echo "  check-docs             Verify docs/usage.md is current"
	@echo "  check-gen              Verify generated files are current"
	@echo "  check                  Run lint, tests, and generated-file checks"

download-openapi-spec:
	@mkdir -p "$(dir $(OPENAPI_SPEC))"
	@tmp=$$(mktemp); \
	trap 'rm -f "$$tmp"' EXIT; \
	curl -fsSL "$(OPENAPI_SPEC_URL)" -o "$$tmp"; \
	grep -q '^openapi: 3\.' "$$tmp"; \
	mv "$$tmp" "$(OPENAPI_SPEC)"; \
	echo "Downloaded $(OPENAPI_SPEC)"

build:
	@mkdir -p "$(BIN_DIR)"
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o "$(BIN_DIR)/$(BINARY)" ./cmd/redmine

install:
	$(GO) install -trimpath -ldflags "$(LDFLAGS)" ./cmd/redmine

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found; running go vet"; \
		$(GO) vet ./...; \
	fi

fix:
	@gofmt -w $(GOFILES)
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w $(GOFILES); \
	else \
		$(GO) run golang.org/x/tools/cmd/goimports@latest -w $(GOFILES); \
	fi

test:
	$(GO) test ./...

gen:
	$(GO) run ./cmd/redmine-openapi-gen -spec "$(OPENAPI_SPEC)" -out internal/openapi/generated.go -docs docs/usage.md

gen-docs: gen

check-docs:
	@tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp"' EXIT; \
	$(GO) run ./cmd/redmine-openapi-gen -spec "$(OPENAPI_SPEC)" -out "$$tmp/generated.go" -docs "$$tmp/usage.md"; \
	diff -u docs/usage.md "$$tmp/usage.md"

check-gen:
	@tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp"' EXIT; \
	$(GO) run ./cmd/redmine-openapi-gen -spec "$(OPENAPI_SPEC)" -out "$$tmp/generated.go" -docs "$$tmp/usage.md"; \
	diff -u internal/openapi/generated.go "$$tmp/generated.go"; \
	diff -u docs/usage.md "$$tmp/usage.md"

check: lint test check-gen

clean:
	rm -rf "$(BIN_DIR)" dist
