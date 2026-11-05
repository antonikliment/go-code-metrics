#!/usr/bin/env bash
# Installs the LoC budget workflow on a repository: seeds .loc-budgets.json,
# writes the caller workflow, and creates the labels it uses. With --app it
# also registers the line-warden GitHub App from a manifest and stores its
# credentials as repository secrets.
# Usage: scripts/setup-budget.sh [--app] [repo-dir] [-- extra `budget init` flags]
# Env: GCM_VERSION (tool version), GCM_REF (workflow ref), GCM_BIN (local binary),
# APP_NAME, APP_PORT.
set -euo pipefail

app=0
[ "${1:-}" = "--app" ] && { app=1; shift; }
dir=${1:-.}
shift || true
[ "${1:-}" = "--" ] && shift
version=${GCM_VERSION:-latest}
ref=${GCM_REF:-main}
cd "$dir"
[ -f go.mod ] || { echo "no go.mod in $dir" >&2; exit 1; }

if [ -f .loc-budgets.json ]; then
  echo "keep: .loc-budgets.json"
elif [ -n "${GCM_BIN:-}" ]; then
  "$GCM_BIN" budget init "$@"
else
  go run "github.com/antonikliment/go-code-metrics/cmd/go-code-metrics@$version" budget init "$@"
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
    uses: antonikliment/go-code-metrics/.github/workflows/budget-pr.yml@${ref}
    with:
      version: ${version}
  main:
    if: github.event_name == 'push'
    uses: antonikliment/go-code-metrics/.github/workflows/budget-main.yml@${ref}
    with:
      version: ${version}
    secrets:
      app-id: \${{ secrets.LINE_WARDEN_APP_ID }}
      app-key: \${{ secrets.LINE_WARDEN_APP_KEY }}
      token: \${{ secrets.LOC_BUDGET_TOKEN }}
EOF
  echo "created: $wf"
fi

have_gh=0
command -v gh >/dev/null && gh auth status >/dev/null 2>&1 && have_gh=1
if [ $have_gh = 1 ]; then
  gh label create loc-raise --color d93f0b --description "Approves a LoC budget raise" --force || echo "label loc-raise not created" >&2
  gh label create loc-budgets --color 0e8a16 --description "LoC budget dashboard" --force || echo "label loc-budgets not created" >&2
else
  echo "gh not authenticated: create labels loc-raise and loc-budgets by hand" >&2
fi

# The manifest flow: the browser POSTs the manifest to GitHub, GitHub
# redirects to our localhost callback with a one-hour code, and the code is
# exchanged for the app id and private key.
register_app() {
  [ $have_gh = 1 ] || { echo "--app needs an authenticated gh" >&2; exit 1; }
  command -v python3 >/dev/null || { echo "--app needs python3" >&2; exit 1; }
  local repo owner name port state target code json app_id slug
  repo=$(gh repo view --json nameWithOwner --jq .nameWithOwner)
  owner=${repo%%/*}
  name=${APP_NAME:-line-warden}
  port=${APP_PORT:-8765}
  state=$(python3 -I -c 'import secrets; print(secrets.token_hex(16))')
  if [ "$(gh api "users/$owner" --jq .type)" = Organization ]; then
    target="https://github.com/organizations/$owner/settings/apps/new?state=$state"
  else
    target="https://github.com/settings/apps/new?state=$state"
  fi
  local manifest
  manifest=$(NAME=$name REPO=$repo PORT=$port python3 -I -c '
import json, os
print(json.dumps({
  "name": os.environ["NAME"],
  "url": "https://github.com/" + os.environ["REPO"],
  "description": "Keeps LoC budgets: raises ceilings after merge, updates the dashboard.",
  "hook_attributes": {"url": "https://example.invalid/unused", "active": False},
  "redirect_url": "http://localhost:%s/callback" % os.environ["PORT"],
  "public": False,
  "default_permissions": {"contents": "write", "issues": "write", "pull_requests": "read"},
  "default_events": [],
}))')

  echo "Open http://localhost:$port/ to register the app (a name already taken can be edited on GitHub's form)."
  command -v xdg-open >/dev/null && xdg-open "http://localhost:$port/" >/dev/null 2>&1 || true
  command -v open >/dev/null && open "http://localhost:$port/" >/dev/null 2>&1 || true
  code=$(MANIFEST=$manifest TARGET=$target STATE=$state PORT=$port python3 -I -c '
import html, os, sys, urllib.parse
from http.server import BaseHTTPRequestHandler, HTTPServer
class H(BaseHTTPRequestHandler):
    code = None
    def log_message(self, *a): pass
    def do_GET(self):
        u = urllib.parse.urlparse(self.path)
        q = urllib.parse.parse_qs(u.query)
        if u.path == "/callback":
            ok = q.get("state", [""])[0] == os.environ["STATE"] and "code" in q
            H.code = q["code"][0] if ok else ""
            body = "Done, return to the terminal." if ok else "State mismatch; aborted."
        else:
            body = ("<form id=f method=post action=\"%s\"><input type=hidden name=manifest value=\"%s\">"
                    "</form><script>document.getElementById(\"f\").submit()</script>"
                    % (html.escape(os.environ["TARGET"]), html.escape(os.environ["MANIFEST"])))
        self.send_response(200); self.send_header("Content-Type", "text/html"); self.end_headers()
        self.wfile.write(body.encode())
srv = HTTPServer(("127.0.0.1", int(os.environ["PORT"])), H)
while H.code is None:
    srv.handle_request()
print(H.code)')
  [ -n "$code" ] || { echo "app registration aborted" >&2; exit 1; }

  json=$(gh api -X POST "app-manifests/$code/conversions")
  app_id=$(jq -r .id <<<"$json")
  slug=$(jq -r .slug <<<"$json")
  jq -r .pem <<<"$json" | gh secret set LINE_WARDEN_APP_KEY
  gh secret set LINE_WARDEN_APP_ID --body "$app_id"
  echo "registered app $slug (id $app_id); secrets LINE_WARDEN_APP_ID and LINE_WARDEN_APP_KEY set"
  echo "Install it on $repo: https://github.com/apps/$slug/installations/new"
  add_bypass "$repo" "$app_id"
}

add_bypass() {
  local repo=$1 app_id=$2 id
  for id in $(gh api "repos/$repo/rulesets" --jq '.[] | select(.target == "branch") | .id'); do
    local rs
    rs=$(gh api "repos/$repo/rulesets/$id")
    if jq -e --argjson a "$app_id" '.bypass_actors // [] | any(.actor_type == "Integration" and .actor_id == $a)' <<<"$rs" >/dev/null; then
      continue
    fi
    read -r -p "Add $(jq -r .name <<<"$rs") ruleset bypass for the app? [y/N] " yn
    [ "$yn" = y ] || continue
    jq --argjson a "$app_id" '{bypass_actors: ((.bypass_actors // []) + [{actor_id: $a, actor_type: "Integration", bypass_mode: "always"}])}' <<<"$rs" |
      gh api -X PUT "repos/$repo/rulesets/$id" --input - >/dev/null && echo "bypass added to ruleset $id"
  done
}

[ $app = 1 ] && register_app

cat <<'EOF'

Remaining by hand:
  1. Without --app: secret LOC_BUDGET_TOKEN (fine-grained PAT, contents:write +
     issues:write) allowed to bypass the default-branch ruleset.
     With --app: install the app (link above), check the bypass was added, and
     optionally upload an icon in the app settings (manifests cannot set one).
  2. Require the "loc-budget / pr" check in the ruleset; turn on
     "dismiss stale approvals" so a push re-asks for a raise approval.
  3. Commit .loc-budgets.json and the workflow.
EOF
