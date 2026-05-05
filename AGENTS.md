# Redmine CLI

## Development

* Cover implementation changes with tests
* Add and generate documentation when adding or changing functionality
* At the end of implementation, run `make fix gen-docs check`

## Running Individual Checks

```bash
make build     # compile
make lint      # verify all Go files with gofmt/goimports and run go vet
make fix       # auto-fix all Go files formatting and imports
make test      # all unit tests
make gen       # regenerate OpenAPI command metadata and docs
make gen-docs  # regenerate docs
make check     # lint, test, and generated-file checks
make check-gen # verify generated files are current
```

## Documentation conventions

CLI documentation is generated from `openapi/openapi.yaml` by `make gen-docs`.

Use English as the primary language for documentation, commits, comments, etc.
