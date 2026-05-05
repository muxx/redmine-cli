OPENAPI_SPEC_URL ?= https://raw.githubusercontent.com/d-yoshi/redmine-openapi/main/openapi.yaml
OPENAPI_SPEC_DIR ?= openapi
OPENAPI_SPEC ?= $(OPENAPI_SPEC_DIR)/openapi.yaml
GO ?= go
BINARY ?= redmine
BIN_DIR ?= bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS ?= -s -w -X main.version=$(VERSION)
ALL_GOFILES := $(shell find . -name '*.go' -not -path './vendor/*')
GOIMPORTS := $(shell if command -v goimports >/dev/null 2>&1; then command -v goimports; fi)
GOIMPORTS_PKG ?= golang.org/x/tools/cmd/goimports@v0.44.0

.DEFAULT_GOAL := help

.PHONY: help download-openapi-spec build install lint fix test gen gen-docs check-gen check clean
help:
	@echo "Targets:"
	@echo "  download-openapi-spec  Download latest Redmine OpenAPI spec to $(OPENAPI_SPEC)"
	@echo "  build                  Compile $(BINARY)"
	@echo "  install                Install $(BINARY)"
	@echo "  lint                   Verify gofmt/goimports on all Go files and run go vet"
	@echo "  fix                    Run gofmt and goimports on all Go files"
	@echo "  test                   Run unit tests"
	@echo "  gen                    Regenerate OpenAPI command metadata and docs"
	@echo "  gen-docs               Regenerate docs/usage.md and generated metadata"
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
	@tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp"' EXIT; \
	for file in $(ALL_GOFILES); do \
		mkdir -p "$$tmp/$$(dirname "$$file")"; \
		cp "$$file" "$$tmp/$$file"; \
	done; \
	gofmt -w $$(find "$$tmp" -name '*.go'); \
	if [ -n "$(GOIMPORTS)" ]; then \
		"$(GOIMPORTS)" -w $$(find "$$tmp" -name '*.go'); \
	else \
		$(GO) run $(GOIMPORTS_PKG) -w $$(find "$$tmp" -name '*.go'); \
	fi; \
	for file in $(ALL_GOFILES); do \
		diff -u "$$file" "$$tmp/$$file" || exit 1; \
	done
	$(GO) vet ./...

fix:
	@gofmt -w $(ALL_GOFILES)
	@if [ -n "$(GOIMPORTS)" ]; then \
		"$(GOIMPORTS)" -w $(ALL_GOFILES); \
	else \
		$(GO) run $(GOIMPORTS_PKG) -w $(ALL_GOFILES); \
	fi

test:
	$(GO) test ./...

gen:
	$(GO) run ./cmd/redmine-openapi-gen -spec "$(OPENAPI_SPEC)" -out internal/openapi/generated.go -docs docs/usage.md

gen-docs: gen

check-gen:
	@tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp"' EXIT; \
	$(GO) run ./cmd/redmine-openapi-gen -spec "$(OPENAPI_SPEC)" -out "$$tmp/generated.go" -docs "$$tmp/usage.md"; \
	diff -u internal/openapi/generated.go "$$tmp/generated.go"; \
	diff -u docs/usage.md "$$tmp/usage.md"

check: lint test check-gen

clean:
	rm -rf "$(BIN_DIR)" dist
