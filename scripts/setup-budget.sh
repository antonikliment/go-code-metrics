#!/usr/bin/env bash
# Installs the LoC budget workflow on a repository: seeds .loc-budgets.json,
# writes the caller workflow, and creates the labels it uses.
# Usage: scripts/setup-budget.sh [repo-dir] [-- extra `budget init` flags]
set -euo pipefail

dir=${1:-.}
shift || true
[ "${1:-}" = "--" ] && shift
version=${GCM_VERSION:-latest}
cd "$dir"
[ -f go.mod ] || { echo "no go.mod in $dir" >&2; exit 1; }

if [ -f .loc-budgets.json ]; then
  echo "keep: .loc-budgets.json"
else
  if [ -n "${GCM_BIN:-}" ]; then "$GCM_BIN" budget init "$@"
  else go run "github.com/antonikliment/go-code-metrics/cmd/go-code-metrics@$version" budget init "$@"; fi
fi

wf=.github/workflows/loc-budget.yml
if [ -f "$wf" ]; then
  echo "keep: $wf"
else
  mkdir -p .github/workflows
  branch=$(git symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null | sed 's#^origin/##' || echo main)
  cat > "$wf" <<EOF
name: loc-budget

on:
  pull_request:
    types: [opened, synchronize, reopened, labeled, unlabeled]
  pull_request_review:
    types: [submitted, dismissed]
  push:
    branches: [${branch}]

concurrency:
  group: loc-budget-\${{ github.event.pull_request.number || github.ref }}

jobs:
  pr:
    if: github.event_name != 'push'
    permissions:
      contents: read
      pull-requests: write
    uses: antonikliment/go-code-metrics/.github/workflows/budget-pr.yml@main
    with:
      version: ${version}
  main:
    if: github.event_name == 'push'
    uses: antonikliment/go-code-metrics/.github/workflows/budget-main.yml@main
    with:
      version: ${version}
    secrets:
      token: \${{ secrets.LOC_BUDGET_TOKEN }}
EOF
  echo "created: $wf"
fi

if command -v gh >/dev/null && gh auth status >/dev/null 2>&1; then
  gh label create loc-raise --color d93f0b --description "Approves a LoC budget raise" --force || echo "label loc-raise not created" >&2
  gh label create loc-budgets --color 0e8a16 --description "LoC budget dashboard" --force || echo "label loc-budgets not created" >&2
else
  echo "gh not authenticated: create labels loc-raise and loc-budgets by hand" >&2
fi

cat <<'EOF'

Remaining by hand:
  1. Secret LOC_BUDGET_TOKEN: a token (GitHub App or fine-grained PAT) with
     contents:write and issues:write, allowed to bypass the default-branch ruleset.
  2. Require the "loc-budget / pr" check in the ruleset; turn on
     "dismiss stale approvals" so a push re-asks for a raise approval.
  3. Commit .loc-budgets.json and the workflow.
EOF
