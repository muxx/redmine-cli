# Redmine CLI

## Development

* Cover implementation changes with tests
* Add and generate documentation when adding or changing functionality
* At the end of implementation, run `make fix test gen-docs`

## Running Individual Checks

```bash
make build     # compile
make lint      # golangci-lint
make fix       # auto-fix lint issues (gofmt + goimports)
make test      # all unit tests
make gen-docs  # regenerate docs
```

## Documentation conventions

CLI documentation is generated from Go source files by `make gen-docs`.

Use English as the primary language for documentation, commits, comments, etc.
