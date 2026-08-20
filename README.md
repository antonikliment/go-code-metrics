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

## Continuous integration

The analyzer runs anywhere Go is available. Two ready-made pipelines live in this
repository and double as copy-paste templates for downstream projects.

### GitHub Actions

`.github/workflows/pr-metrics.yml` runs PR analysis on every pull request and
attaches the report to the run as a downloadable artifact:

```yaml
on:
  pull_request:
    branches: [main]

jobs:
  metrics:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0            # full history so merge-base resolves
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: git fetch --no-tags origin "${{ github.base_ref }}"
      - run: go run ./cmd/sizeanalyzer -pr -base "origin/${{ github.base_ref }}" -html pr-metrics.html
      - uses: actions/upload-artifact@v4
        with:
          name: pr-metrics
          path: pr-metrics.html
```

Downstream projects that pin the tool can replace `go run ./cmd/sizeanalyzer`
with `go tool sizeanalyzer`.

## Go LOC budget

`goclocbudget` is one feature in the module. It enforces repository-wide and
path-specific Go implementation line budgets using the shared analysis engine.
Path budgets include the entire directory tree, so one budget can cover several
Go packages. In a monorepo, a parent `go.work` defines the repository root; a
repository root without a `go.mod` or `go.work` is also supported. Only exceeded
budgets produce lint diagnostics.

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
          path-budgets:
            agent:
              max-go-code-lines: 6000
            provider:
              max-go-code-lines: 2500
            capability/memory:
              max-go-code-lines: 5000
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
