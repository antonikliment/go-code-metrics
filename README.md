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

### Woodpecker CI

`.woodpecker/pr-metrics.yaml` is the equivalent pipeline. Woodpecker clones
shallow by default, so override the clone depth for `merge-base`, and fetch the
target branch before analysis:

```yaml
when:
  - event: pull_request

clone:
  git:
    image: woodpeckerci/plugin-git
    settings:
      depth: 0

steps:
  metrics:
    image: golang:1.26
    commands:
      - git fetch --no-tags origin "$CI_COMMIT_TARGET_BRANCH"
      - go run ./cmd/sizeanalyzer -pr -base "origin/$CI_COMMIT_TARGET_BRANCH" -html pr-metrics.html
```

Woodpecker has no built-in per-run artifact store. To publish the HTML report,
add a storage step (for example `woodpeckerci/plugin-s3`) and provide its
credentials as repo secrets. `.woodpecker/ci.yaml` additionally mirrors the
GitHub Actions `test` + `lint` jobs.

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

## Budget ledger

`.loc-budgets.json` holds the global ceiling and per-area ceilings (longest
path prefix wins). Pull requests never edit it; only the main-branch workflow
does, so concurrent PRs do not conflict on budget numbers. The usage table
is the dashboard: `budget-main` appends it to the job summary of the push
run on the default branch; an issue labelled `loc-budgets` is opt-in.

```json
{"global": 40000, "areas": {"cmd": 2500, "cmd/ui": 6800}, "exclude-dirs": ["vendor"]}
```

```bash
go-code-metrics budget report [-root .] [-base-root ../main]  # table, optional delta column
go-code-metrics budget check  ...                             # exit 3 when a ceiling is exceeded
go-code-metrics budget sync   ...                             # set ceilings to measured usage
```

Drop `goclocbudget` from lint in a repo that uses the ledger: a hardcoded cap
would fail every approved raise, and `budget-pr` is the gate.

Install on a repository (seeds the ledger from current usage, writes
`.github/workflows/loc-budget.yml`, creates the labels):

```bash
scripts/setup-budget.sh path/to/repo -- -areas cmd,internal   # areas default to top-level dirs
scripts/setup-budget.sh --app path/to/repo                    # also register the line-warden app
DASHBOARD_ISSUE=yes scripts/setup-budget.sh path/to/repo      # dashboard also as a loc-budgets issue
```

`--app` registers a private GitHub App (`line-warden`, override with
`APP_NAME`) from a manifest: a localhost page posts the manifest to GitHub,
the callback code is exchanged for the app id and key, which are stored as
`LINE_WARDEN_APP_ID` / `LINE_WARDEN_APP_KEY`, and the script offers to add the
app to each branch ruleset's bypass list. Installing the app on the repo and
uploading an icon stay manual. Once the workflow is on the default branch, a
re-run offers to make `pr / budget` a required check (`REQUIRE_CHECK=yes`
answers without asking). Re-runs reuse a configured app and an open
`loc-budget` pull request.

Reusable workflows:

- `budget-pr.yml` measures the PR merged onto the latest base and posts a
  sticky table. A raise passes once someone with at least `approver-role`
  (default `maintain`) approves the head commit or adds the `loc-raise` label;
  a label from anyone below that role is removed, and a push removes it too.
- `budget-main.yml` runs on push to the default branch, syncs the ledger to
  what main measures and commits it, then appends the report table to the
  job summary - that is where the dashboard lives. The `dashboard-issue`
  input (`boolean`, default `false`) also updates the issue labelled
  `loc-budgets`. It mints a token from the line-warden app (`app-id`,
  `app-key`) or takes a `token`; either needs a ruleset bypass to push. The
  ledger is committed through the contents API, so GitHub signs the commit.
  Create the `loc-raise` label first, plus `loc-budgets` when the issue
  dashboard is on.

With `budgets-repo: owner/name` set on both workflows, the ledger lives in that
repository (same owner as the code repository) as `<owner>/<repo>.json` on its
default branch, so no code commit ever touches budget numbers; `scripts/setup-budget.sh`
bootstraps it, and `--migrate-budgets` moves an existing in-repo ledger there. A private budgets repo is read with the `app-id`/`app-key` secrets (the line-warden app), which both budget workflows accept.

```bash
scripts/setup-budget.sh path/to/repo --budgets-repo owner/name
scripts/setup-budget.sh --migrate-budgets owner/name path/to/repo
```

```yaml
# caller: .github/workflows/loc-budget.yml
on:
  pull_request:
    types: [opened, synchronize, reopened, labeled, unlabeled]
  pull_request_review:
    types: [submitted, dismissed]
jobs:
  budget:
    uses: antonikliment/go-code-metrics/.github/workflows/budget-pr.yml@main
```
