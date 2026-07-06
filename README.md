# go-code-metrics

`go-code-metrics` provides reusable Go source analysis, reporting, and code-quality tooling.

The module also provides reusable Go code metrics:

- `analysis` discovers Go files and measures LOC with `gocloc` and cyclomatic complexity with `gocyclo`.
- `report` renders terminal, JSON, and self-contained HTML output.
- `cmd/sizeanalyzer` is the command-line adapter.
- `goclocbudget` is the thin `golangci-lint` budget feature.

## Install lint configuration

From a Go project, create the default lint files:

```bash
go run github.com/antonikliment/go-code-metrics/cmd/go-code-metrics@v0.0.2 install
```

This creates `.custom-gcl.yml` and `.golangci.yml` only when they do not exist.
Existing configuration is never changed. Review the generated LOC budget before
building the custom linter.

## Size analyzer

Pin the analyzer as a project tool:

```bash
go get -tool github.com/antonikliment/go-code-metrics/cmd/sizeanalyzer@v0.0.2
go tool sizeanalyzer
```

This records the command in the downstream project's `go.mod`, so local and CI
runs use the same version. To upgrade or remove it:

```bash
go get -tool github.com/antonikliment/go-code-metrics/cmd/sizeanalyzer@latest
go get -tool github.com/antonikliment/go-code-metrics/cmd/sizeanalyzer@none
```

Terminal output is the default. JSON and self-contained HTML reports are
explicit outputs suitable for CI artifacts:

```bash
go tool sizeanalyzer -json size-report.json -html size-report.html
```

Tests and generated files are excluded by default. Use `-include-tests` or
`-include-generated` to include them. Project-relative directories can be
excluded with repeatable flags:

```bash
go tool sizeanalyzer -exclude-dir app/dist -exclude-dir build
```

Unparseable files retain their LOC and produce warnings by default; use
`-strict` to fail immediately. Use `-hotspots N` to control the number of
complexity hotspots retained per file.

### Pull request analysis

Compare the current working tree with its merge base on `main`:

```bash
go tool sizeanalyzer -pr
```

PR mode includes committed, staged, unstaged, and untracked files. It reports
Git line changes, gocloc code deltas, and function-level complexity added,
removed, and net. Use another target branch with `-base` and write CI artifacts
with the existing output flags:

```bash
go tool sizeanalyzer -pr -base origin/main \
  -json pr-metrics.json -html pr-metrics.html
```

The base ref and its merge-base history must exist locally. For GitHub Actions,
check out full history before running the tool:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0
- run: go tool sizeanalyzer -pr -base origin/main
```

To run without adding a tool dependency:

```bash
go run github.com/antonikliment/go-code-metrics/cmd/sizeanalyzer@v0.0.2
```

## Go LOC budget

`goclocbudget` is one feature in the module. It enforces a repository-wide Go
implementation line budget using the shared analysis engine.

Add the plugin to `.custom-gcl.yml`:

```yaml
version: v2.11.4
name: custom-golangci-lint
destination: .

plugins:
  - module: github.com/antonikliment/go-code-metrics
    import: github.com/antonikliment/go-code-metrics/goclocbudget
    version: v0.0.2
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
