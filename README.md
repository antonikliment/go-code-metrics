# goclocbudget

`goclocbudget` is a `golangci-lint` module plugin that enforces a repository-wide Go implementation line budget using `gocloc`.

The module also provides reusable Go code metrics:

- `analysis` discovers Go files and measures LOC with `gocloc` and cyclomatic complexity with `gocyclo`.
- `report` renders terminal, JSON, and self-contained HTML output.
- `cmd/sizeanalyzer` is the command-line adapter.
- `plugin.go` is the thin `golangci-lint` budget adapter.

## Size analyzer

Run a terminal summary:

```bash
go run ./cmd/sizeanalyzer
```

Write machine-readable or browser reports:

```bash
go run ./cmd/sizeanalyzer -json size-report.json -html size-report.html
```

Tests and generated files are excluded by default. Use `-include-tests` or
`-include-generated` when those sources should contribute to the report.

## Usage

Add the plugin to `.custom-gcl.yml`:

```yaml
version: v2.11.4
name: custom-golangci-lint
destination: .

plugins:
  - module: github.com/antonikliment/goclocbudget
    version: v0.1.0
```

Enable it in `.golangci.yml`:

```yaml
version: "2"

linters:
  enable:
    - goclocbudget

  settings:
    custom:
      goclocbudget:
        type: "module"
        description: "Enforces the implementation Go LOC budget using gocloc."
        settings:
          max-go-code-lines: 10000
          include-tests: false
          exclude-generated: true
          exclude-dirs:
            - vendor
            - .git
            - node_modules
            - app/dist
```

Build and run:

```bash
golangci-lint custom
./custom-golangci-lint run
```
