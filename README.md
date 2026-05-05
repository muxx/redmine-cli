# redmine-cli

A Redmine CLI tool bringing Redmine to your command line.

Built based on https://d-yoshi.github.io/redmine-openapi/ ([repo](https://github.com/d-yoshi/redmine-openapi)).

## Install

### Homebrew

```bash
brew install muxx/tap/redmine-cli
redmine --version
```

The Homebrew formula is named `redmine-cli`; it installs the `redmine` executable.

### GitHub release archive

Download the archive for your OS and architecture from the [latest release](https://github.com/muxx/redmine-cli/releases/latest).

Archive names use this format:

```text
redmine_<version>_<os>_<arch>.tar.gz
redmine_<version>_windows_<arch>.zip
```

Example for macOS Apple Silicon:

```bash
VERSION=v0.2.0
curl -LO "https://github.com/muxx/redmine-cli/releases/download/${VERSION}/redmine_${VERSION}_darwin_arm64.tar.gz"
tar -xzf "redmine_${VERSION}_darwin_arm64.tar.gz"
sudo install -m 0755 "redmine_${VERSION}_darwin_arm64/redmine" /usr/local/bin/redmine
redmine --version
```

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
