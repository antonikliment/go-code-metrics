#!/usr/bin/env bash
# Installs the LoC budget workflow on a repository: seeds .loc-budgets.json,
# writes the caller workflow, and creates the labels it uses. With --app it
# also registers the line-warden GitHub App from a manifest and stores its
# credentials as repository secrets. A goclocbudget linter in .golangci.yml
# is migrated: its exclude-dirs seed the ledger and the linter is removed,
# since a hardcoded cap would fail every approved raise. With --pr the changes
# go to a loc-budget branch and a pull request. --budgets-repo owner/name keeps
# the ledger in a separate repository of the same owner, as <owner>/<repo>.json
# on its default branch; --migrate-budgets moves an existing in-repo ledger there
# and removes the in-repo copy, leaving the code repository change for you to PR.
# Usage: scripts/setup-budget.sh [--app] [--pr] [--budgets-repo owner/name |
#   --migrate-budgets owner/name] [repo-dir] [-- extra `budget init` flags]
# Env: GCM_VERSION (tool version), GCM_REF (workflow ref), GCM_BIN (local binary),
# APP_NAME, APP_PORT, APP_BYPASS=yes (add ruleset bypasses without asking),
# DASHBOARD_ISSUE=yes (also publish the dashboard to the loc-budgets issue; the
# job summary always gets the table),
# REQUIRE_CHECK=yes (make "pr / budget" required without asking),
# APP_FORCE=1 (register a new app even if one is configured).
set -euo pipefail

app=0 pr=0 migrate=0 budgets_repo=
while :; do
  case "${1:-}" in
    --app) app=1; shift ;;
    --pr) pr=1; shift ;;
    --budgets-repo) budgets_repo=${2:?--budgets-repo needs owner/name}; shift 2 ;;
    --migrate-budgets) budgets_repo=${2:?--migrate-budgets needs owner/name}; migrate=1; shift 2 ;;
    *) break ;;
  esac
done
dir=${1:-.}
shift || true
[ "${1:-}" = "--" ] && shift
ref=${GCM_REF:-main}
version=${GCM_VERSION:-$ref}
# The job summary always holds the dashboard; DASHBOARD_ISSUE=yes adds the issue.
dashboard_issue=false
labels=loc-raise
[ -z "${DASHBOARD_ISSUE:-}" ] || { dashboard_issue=true; labels="loc-raise and loc-budgets"; }
cd "$dir"
[ -f go.mod ] || { echo "no go.mod in $dir" >&2; exit 1; }

# The seed file: .loc-budgets.json, or a temp copy when the ledger is elsewhere.
ledger=.loc-budgets.json
if [ -n "$budgets_repo" ]; then
  name=$(gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null)
  [ -n "$name" ] || { echo "--budgets-repo needs gh to name the code repository" >&2; exit 1; }
  ledger=$(mktemp -d)/ledger.json
fi

# lint_cap excludes|strip: reads or removes the goclocbudget linter in
# .golangci.yml by indentation, so the file's comments survive.
lint_cap() {
  [ -f .golangci.yml ] && grep -q goclocbudget .golangci.yml || return 0
  MODE=$1 python3 -I - <<'EOF'
import os, re
path = ".golangci.yml"
lines = open(path).read().split("\n")
indent = lambda l: len(l) - len(l.lstrip())
start = next((i for i, l in enumerate(lines) if l.strip() == "goclocbudget:"), None)
end = start
if start is not None:
    end = start + 1
    while end < len(lines) and (not lines[end].strip() or indent(lines[end]) > indent(lines[start])):
        end += 1
    while end > start + 1 and not lines[end - 1].strip():
        end -= 1
block = lines[start:end] if start is not None else []
if os.environ["MODE"] == "excludes":
    dirs, inside, level = [], False, 0
    for l in block:
        if l.strip() == "exclude-dirs:":
            inside, level = True, indent(l)
        elif inside and l.strip().startswith("- "):
            dirs.append(l.strip()[2:].split("#")[0].strip())
        elif inside and l.strip() and not l.strip().startswith("#") and indent(l) <= level:
            inside = False
    print(",".join(d for d in dirs if d))
else:
    if start is not None:
        del lines[start:end]
        parent = start - 1
        if lines[parent].strip() == "custom:" and (start >= len(lines) or not lines[start].strip() or indent(lines[start]) <= indent(lines[parent])):
            del lines[parent]
    lines = [l for l in lines if not re.fullmatch(r"\s*- goclocbudget\s*", l)]
    open(path, "w").write("\n".join(lines))
    print("removed: goclocbudget from .golangci.yml")
EOF
}

if [ ! -f "$ledger" ] && [[ " $* " != *" -exclude "* ]]; then
  excludes=$(lint_cap excludes)
  [ -n "$excludes" ] && set -- "$@" -exclude "$excludes"
fi

if [ $migrate = 1 ] && [ -f .loc-budgets.json ]; then
  cp .loc-budgets.json "$ledger"
  echo "migrate: .loc-budgets.json copied to $ledger"
elif [ -n "${GCM_BIN:-}" ]; then
  "$GCM_BIN" budget init -ledger "$ledger" "$@"
else
  go run "github.com/antonikliment/go-code-metrics/cmd/go-code-metrics@$version" budget init -ledger "$ledger" "$@"
fi

lint_cap strip

wf=.github/workflows/loc-budget.yml
# The workflow is generated, so a re-run rewrites it. Reusable workflows are
# pinned to the commit the ref names, as workflow audits (zizmor) require.
sha=$ref
if ! [[ $ref =~ ^[0-9a-f]{40}$ ]]; then
  sha=$(git ls-remote https://github.com/antonikliment/go-code-metrics.git "refs/heads/$ref" "refs/tags/$ref" | head -1 | cut -f1)
  [ -n "$sha" ] || { echo "cannot resolve go-code-metrics ref $ref" >&2; exit 1; }
fi
# With the app, writes go through its token; github.token only reads.
main_perms="contents: read"
[ $app = 1 ] || [ -n "$budgets_repo" ] || main_perms="contents: write"
if [ $app != 1 ] && [ $dashboard_issue = true ]; then
  main_perms="$main_perms
      issues: write"
fi
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

permissions: {}

concurrency:
  group: loc-budget-\${{ github.event.pull_request.number || github.ref }}

jobs:
  pr:
    if: github.event_name != 'push'
    permissions:
      contents: read
      pull-requests: write
    uses: antonikliment/go-code-metrics/.github/workflows/budget-pr.yml@${sha} # ${ref}
    with:
      version: ${version}
      budgets-repo: ${budgets_repo}
    secrets:
      app-id: \${{ secrets.LINE_WARDEN_APP_ID }}
      app-key: \${{ secrets.LINE_WARDEN_APP_KEY }}
  main:
    if: github.event_name == 'push'
    permissions:
      ${main_perms}
    uses: antonikliment/go-code-metrics/.github/workflows/budget-main.yml@${sha} # ${ref}
    with:
      version: ${version}
      dashboard-issue: ${dashboard_issue}
      budgets-repo: ${budgets_repo}
    secrets:
      app-id: \${{ secrets.LINE_WARDEN_APP_ID }}
      app-key: \${{ secrets.LINE_WARDEN_APP_KEY }}
      token: \${{ secrets.LOC_BUDGET_TOKEN }}
EOF
echo "wrote: $wf (go-code-metrics ${ref} = ${sha:0:7})"

have_gh=0
command -v gh >/dev/null && gh auth status >/dev/null 2>&1 && have_gh=1
if [ $have_gh = 1 ]; then
  gh label create loc-raise --color d93f0b --description "Approves a LoC budget raise" --force || echo "label loc-raise not created" >&2
  if [ $dashboard_issue = true ]; then
    gh label create loc-budgets --color 0e8a16 --description "LoC budget dashboard" --force || echo "label loc-budgets not created" >&2
  fi
else
  echo "gh not authenticated: create labels $labels by hand" >&2
fi

# The manifest flow: the browser POSTs the manifest to GitHub, GitHub
# redirects to our localhost callback with a one-hour code, and the code is
# exchanged for the app id and private key.
register_app() {
  [ $have_gh = 1 ] || { echo "--app needs an authenticated gh" >&2; exit 1; }
  command -v python3 >/dev/null || { echo "--app needs python3" >&2; exit 1; }
  local repo owner name port state target code json slug
  repo=$(gh repo view --json nameWithOwner --jq .nameWithOwner)
  owner=${repo%%/*}
  name=${APP_NAME:-line-warden}
  port=${APP_PORT:-8765}
  # Re-runs reuse a registered, configured app; APP_FORCE=1 registers anew.
  if [ -z "${APP_FORCE:-}" ] && gh secret list --json name --jq '.[].name' | grep -qx LINE_WARDEN_APP_ID &&
    app_id=$(gh api "apps/$name" --jq .id 2>/dev/null); then
    echo "keep: app $name (id $app_id) and its secrets; APP_FORCE=1 registers a new one"
    add_bypass "$repo" "$app_id"
    return
  fi
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
  # A ruleset accepts the app as a bypass actor only once it is installed.
  local install="https://github.com/apps/$slug/installations/new"
  echo "Install it on $repo (only that repository): $install"
  command -v xdg-open >/dev/null && xdg-open "$install" >/dev/null 2>&1 || true
  read -r -p "Press Enter once the app is installed... " _
  add_bypass "$repo" "$app_id"
}

add_bypass() {
  local repo=$1 app_id=$2 id installed
  # GitHub rejects a whole bypass list that names a deleted or uninstalled
  # app, so stale Integration entries are dropped on write.
  installed=$(gh api "orgs/${repo%%/*}/installations" --paginate --jq '[.installations[].app_id]' 2>/dev/null | jq -s 'add // []' || echo null)
  for id in $(gh api "repos/$repo/rulesets" --jq '.[] | select(.target == "branch") | .id'); do
    local rs
    rs=$(gh api "repos/$repo/rulesets/$id")
    if jq -e --argjson a "$app_id" '.bypass_actors // [] | any(.actor_type == "Integration" and .actor_id == $a)' <<<"$rs" >/dev/null; then
      continue
    fi
    yn=${APP_BYPASS:-}
    [ -n "$yn" ] || read -r -p "Add $(jq -r .name <<<"$rs") ruleset bypass for the app? [y/N] " yn
    [ "$yn" = y ] || [ "$yn" = yes ] || continue
    jq --argjson a "$app_id" --argjson live "$installed" '{bypass_actors: (
        [(.bypass_actors // [])[] | select(.actor_type != "Integration" or $live == null or (.actor_id as $i | $live | index($i)))]
        + [{actor_id: $a, actor_type: "Integration", bypass_mode: "always"}])}' <<<"$rs" |
      gh api -X PUT "repos/$repo/rulesets/$id" --input - --jq '"bypass added to ruleset \(.id)"' ||
      echo "bypass not added to ruleset $id (see the error above; is the app installed?)" >&2
  done
}

# The budgets repository shares this owner, tracks its default branch, and is
# written only by the app, with repo admins able to override.
budgets_repo_ready() {
  [ $have_gh = 1 ] || { echo "--budgets-repo needs an authenticated gh" >&2; exit 1; }
  app_id=$(gh api "apps/${APP_NAME:-line-warden}" --jq .id 2>/dev/null) ||
    { echo "--budgets-repo needs the ${APP_NAME:-line-warden} app: run with --app" >&2; exit 1; }
  gh api "repos/$budgets_repo" --jq '"keep: budgets repo \(.full_name)"' 2>/dev/null ||
    gh repo create "$budgets_repo" --public --add-readme
  gh api -X PUT "repos/$budgets_repo/contents/$name.json" -f message="LoC budgets: seed $name.json" \
    -f content="$(base64 -w0 "$ledger")" >/dev/null || echo "keep: $name.json in $budgets_repo"
  [ $migrate = 1 ] && git rm -q --ignore-unmatch .loc-budgets.json
  # An Integration bypass needs the app installed on the budgets repository first.
  if ! gh api "repos/$budgets_repo/rulesets" --jq '.[].name' 2>/dev/null | grep -qx budgets-app-only; then
    gh api -X POST "repos/$budgets_repo/rulesets" --input - <<EOF || echo "install the ${APP_NAME:-line-warden} app on $budgets_repo (org settings > GitHub Apps > Configure), then re-run" >&2
{"name":"budgets-app-only","target":"branch","enforcement":"active","conditions":{"ref_name":{"include":["~DEFAULT_BRANCH"],"exclude":[]}},
 "rules":[{"type":"update"},{"type":"deletion"},{"type":"non_fast_forward"},{"type":"creation"}],
 "bypass_actors":[{"actor_type":"Integration","actor_id":$app_id,"bypass_mode":"always"},
                  {"actor_type":"RepositoryRole","actor_id":5,"bypass_mode":"always"}]}
EOF
  fi
}

[ $app = 1 ] && register_app
[ -n "$budgets_repo" ] && budgets_repo_ready

# The PR check is "<caller job> / <reusable job>": pr / budget. Requiring it
# before the workflow is on the default branch would block every other PR.
require_check() {
  local repo id rs
  repo=$(gh repo view --json nameWithOwner --jq .nameWithOwner)
  if ! gh api "repos/$repo/contents/$wf" --silent 2>/dev/null; then
    echo "after the workflow is merged, re-run to require the pr / budget check"
    return
  fi
  for id in $(gh api "repos/$repo/rulesets" --jq '.[] | select(.target == "branch") | .id' 2>/dev/null); do
    rs=$(gh api "repos/$repo/rulesets/$id")
    if jq -e '[.rules[] | select(.type == "required_status_checks") | .parameters.required_status_checks[].context] | index("pr / budget")' <<<"$rs" >/dev/null; then
      continue
    fi
    yn=${REQUIRE_CHECK:-}
    [ -n "$yn" ] || read -r -p "Require the pr / budget check in the $(jq -r .name <<<"$rs") ruleset? [y/N] " yn
    [ "$yn" = y ] || [ "$yn" = yes ] || continue
    jq '{rules: (if any(.rules[]; .type == "required_status_checks")
          then .rules | map(if .type == "required_status_checks"
            then .parameters.required_status_checks += [{context: "pr / budget"}] else . end)
          else .rules + [{type: "required_status_checks", parameters: {
            required_status_checks: [{context: "pr / budget"}], strict_required_status_checks_policy: false}}] end)}' <<<"$rs" |
      gh api -X PUT "repos/$repo/rulesets/$id" --input - --jq '"pr / budget required in ruleset \(.id)"' ||
      echo "check not required in ruleset $id (see the error above)" >&2
  done
}
[ $have_gh = 1 ] && require_check

open_pr() {
  [ $have_gh = 1 ] || { echo "--pr needs an authenticated gh" >&2; exit 1; }
  [ "$(git branch --show-current)" = loc-budget ] || git checkout -q -B loc-budget
  git add "$wf"
  [ -f .golangci.yml ] && git add .golangci.yml
  [ -f .loc-budgets.json ] && git add .loc-budgets.json
  local title body
  if [ $migrate = 1 ]; then
    title="LoC budgets: move the ledger to $budgets_repo"
    body="The ledger now lives in \`$budgets_repo\`, which the \`loc-budget\` workflow reads and writes through its \`budgets-repo\` input; the in-repo copy is removed here."
  elif git cat-file -e "origin/$branch:.loc-budgets.json" 2>/dev/null; then
    title="Pin loc-budget to go-code-metrics $ref"
    body="Re-pins the loc-budget reusable workflows to go-code-metrics $ref (${sha:0:7})."
  else
    title="LoC budgets: ledger and loc-budget workflow"
    body=$(cat <<'EOF'
Adds the LoC budget ledger (`.loc-budgets.json`, seeded from current usage) and the `loc-budget` workflow from go-code-metrics. Ceilings are synced to main by the workflow after each merge, so PRs never edit budget numbers. A raise in a PR passes once a maintainer approves the head commit or adds `loc-raise`. The goclocbudget linter's hardcoded cap is removed in favour of that check.

After merge, re-run the setup script to require the `pr / budget` check, and turn on "dismiss stale approvals".
EOF
)
  fi
  git diff --cached --quiet || git commit -q -m "$title"
  git push -q -u --force-with-lease origin loc-budget
  # gh pr view also finds merged and closed PRs for the branch.
  if gh pr view loc-budget --json state,url --jq 'select(.state == "OPEN") | .url' 2>/dev/null | grep .; then
    echo "keep: open pull request above"
    return
  fi
  gh pr create --title "$title" --body "$body"
}
[ $pr = 1 ] && open_pr

cat <<'EOF'

Remaining by hand:
  1. Without --app: secret LOC_BUDGET_TOKEN (fine-grained PAT, contents:write,
     plus issues:write with DASHBOARD_ISSUE=yes) allowed to bypass the default-branch
     ruleset. With --app: install the app (link above), check the bypass was added, and
     optionally upload an icon in the app settings (manifests cannot set one).
     A separate budgets repo needs the same write access there, which --budgets-repo
     grants only with --app (or a PAT scoped to that repository too).
  2. Once the workflow is merged, re-run to require the "pr / budget" check;
     turn on "dismiss stale approvals" so a push re-asks for a raise approval.
  3. Without --pr: commit .loc-budgets.json, the workflow and .golangci.yml;
     with --migrate-budgets, commit the removal of .loc-budgets.json.
EOF

