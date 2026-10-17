#!/usr/bin/env bash
# gitd against GitHub itself, as a platform runs it: a GitHub App kept in the configuration,
# publishing settings set through the API, an agent's change published as a pull request
# from the App's bot, and a change to the CI refused before anything is pushed.
#
# It needs the App's ID and private key (GITD_E2E_APP_ID, GITD_E2E_APP_KEY), the App
# installed on REPOSITORY only, and gitd built at ./bin/gitd. The branches it pushes are
# named after RUN; whoever runs it deletes them afterwards.
set -euo pipefail

: "${GITD_E2E_APP_ID:?the App's ID}" "${GITD_E2E_APP_KEY:?the App's private key}"
REPOSITORY="${REPOSITORY:-dooservice/gitd-e2e}"
RUN="${RUN:-local-$(date +%s)}"
REVIEWER="${REVIEWER:-}"
ROOT="$(mktemp -d)"
GITD="$PWD/bin/gitd"

umask 077
printf '%s\n' "$GITD_E2E_APP_KEY" > "$ROOT/app.pem"
cat > "$ROOT/gitd.toml" <<TOML
socket = "$ROOT/gitd.sock"
state = "$ROOT/state"
roots = ["$ROOT/workspaces"]

[[credentials]]
prefix = "https://github.com/$REPOSITORY"
github_app = { app_id = $GITD_E2E_APP_ID, private_key_file = "$ROOT/app.pem" }
TOML
mkdir -p "$ROOT/workspaces"
"$GITD" serve --config "$ROOT/gitd.toml" > "$ROOT/gitd.log" 2>&1 &
SERVING=$!
trap 'kill $SERVING 2>/dev/null; cat "$ROOT/gitd.log" >&2; rm -rf "$ROOT"' EXIT
export GITD_SOCKET="$ROOT/gitd.sock"
for _ in $(seq 50); do [ -S "$GITD_SOCKET" ] && break; sleep 0.1; done

step() { printf '\n== %s\n' "$*"; }
fail() { printf 'FAILED: %s\n' "$*" >&2; exit 1; }

step "the App is listed, never its key"
"$GITD" connections list | tee "$ROOT/listed"
grep -q "github_app  https://github.com/$REPOSITORY" "$ROOT/listed" || fail "the App is not listed"
"$GITD" --json connections list | grep -q "PRIVATE KEY" && fail "the key is shown"

step "a private repository reads with the App's token"
"$GITD" probe "https://github.com/$REPOSITORY" | tee "$ROOT/probe"
grep -qi "reachable" "$ROOT/probe" || fail "the repository is not reachable with the App"

step "publishing settings, through the API"
"$GITD" publishing set "https://github.com/$REPOSITORY" --preset recommended --branch-prefix agents/ \
  --pull-requests always --draft --label agent ${REVIEWER:+--review-requester}


step "an agent's change is published as a pull request from the App's bot"
"$GITD" prepare "https://github.com/$REPOSITORY" "$ROOT/workspaces/fix" > /dev/null
echo "run $RUN" > "$ROOT/workspaces/fix/e2e-$RUN.txt"
git -C "$ROOT/workspaces/fix" add -A
git -C "$ROOT/workspaces/fix" -c user.name="gitd e2e" -c user.email="e2e@gitd.dev" commit -q -m "e2e: run $RUN"
"$GITD" --json publish "$ROOT/workspaces/fix" "agents/e2e-$RUN" ${REVIEWER:+--requested-by "$REVIEWER"} \
  --body "An end to end run of gitd: $RUN." | tee "$ROOT/published"
grep -q '"url": "https://github.com/'"$REPOSITORY"'/pull/' "$ROOT/published" || fail "no pull request was opened"

step "a change to the CI is refused before anything is pushed"
"$GITD" prepare "https://github.com/$REPOSITORY" "$ROOT/workspaces/ci" > /dev/null
mkdir -p "$ROOT/workspaces/ci/.github/workflows"
echo "name: injected" > "$ROOT/workspaces/ci/.github/workflows/injected.yml"
git -C "$ROOT/workspaces/ci" add -A
git -C "$ROOT/workspaces/ci" -c user.name="gitd e2e" -c user.email="e2e@gitd.dev" commit -q -m "e2e: change the CI"
if "$GITD" publish "$ROOT/workspaces/ci" "agents/e2e-ci-$RUN" 2> "$ROOT/refused"; then
  fail "a change to the CI was published"
fi
cat "$ROOT/refused"
grep -q "POLICY_REFUSED" "$ROOT/refused" || fail "the refusal is not POLICY_REFUSED"

step "both are on the record"
"$GITD" audit --limit 5

step "no token reached a workspace"
grep -rq "ghs_" "$ROOT/workspaces" && fail "a token is in a workspace"
echo "none"

printf '\nPASSED: %s\n' "$RUN"
