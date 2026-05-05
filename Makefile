OPENAPI_SPEC_URL ?= https://raw.githubusercontent.com/d-yoshi/redmine-openapi/main/openapi.yaml
OPENAPI_SPEC_DIR ?= openapi
OPENAPI_SPEC ?= $(OPENAPI_SPEC_DIR)/openapi.yaml

.DEFAULT_GOAL := help

.PHONY: help download-openapi-spec
help:
	@echo "Targets:"
	@echo "  download-openapi-spec  Download latest Redmine OpenAPI spec to $(OPENAPI_SPEC)"

download-openapi-spec:
	@mkdir -p "$(dir $(OPENAPI_SPEC))"
	@tmp=$$(mktemp); \
	trap 'rm -f "$$tmp"' EXIT; \
	curl -fsSL "$(OPENAPI_SPEC_URL)" -o "$$tmp"; \
	grep -q '^openapi: 3\.' "$$tmp"; \
	mv "$$tmp" "$(OPENAPI_SPEC)"; \
	echo "Downloaded $(OPENAPI_SPEC)"
