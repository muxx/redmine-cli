# redmine-cli

A Redmine CLI tool bringing Redmine to your command line.

Built based on https://d-yoshi.github.io/redmine-openapi/ ([repo](https://github.com/d-yoshi/redmine-openapi)).

## Usage

```bash
redmine auth login --host https://redmine.example.com --api-key "$REDMINE_API_KEY"
redmine issue list --limit 20
redmine issue show 123 --include journals
redmine issue create --project-id my-project --subject "Fix checkout"
```

Full generated usage documentation is in [docs/usage.md](docs/usage.md).

## Development

The command metadata and usage docs are generated from `openapi/openapi.yaml`.

```bash
make download-openapi-spec
make gen
make check
```
