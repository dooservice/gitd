#!/usr/bin/env bash
# gitd's endpoint end to end, as a platform with sandboxes elsewhere uses it: a token
# issued for one repository, a sandbox in a container of its own network that clones and
# pushes over HTTP with it, and its branch published as a pull request from the App's bot.
# The sandbox never holds the git host's credential.
#
# It needs what e2e/github.sh needs, and Docker. The branch it pushes is named after RUN;
# whoever runs it deletes it afterwards.
set -euo pipefail

: "${GITD_E2E_APP_ID:?the App's ID}" "${GITD_E2E_APP_KEY:?the App's private key}"
REPOSITORY="${REPOSITORY:-dooservice/gitd-e2e}"
RUN="${RUN:-local-$(date +%s)}"
ROOT="$(mktemp -d)"
GITD="$PWD/bin/gitd"
# Where a container reaches the host: Docker's bridge gateway.
HOST_ADDRESS="$(docker network inspect bridge --format '{{(index .IPAM.Config 0).Gateway}}')"

umask 077
printf '%s\n' "$GITD_E2E_APP_KEY" > "$ROOT/app.pem"
umask 022
mkdir -p "$ROOT/workspaces"
cat > "$ROOT/gitd.toml" <<TOML
socket = "$ROOT/gitd.sock"
state = "$ROOT/state"
roots = ["$ROOT/workspaces"]

[endpoint]
listen = "$HOST_ADDRESS:9420"

[[credentials]]
prefix = "https://github.com/$REPOSITORY"
github_app = { app_id = $GITD_E2E_APP_ID, private_key_file = "$ROOT/app.pem" }
TOML
"$GITD" serve --config "$ROOT/gitd.toml" > "$ROOT/gitd.log" 2>&1 &
SERVING=$!
trap 'kill $SERVING 2>/dev/null; cat "$ROOT/gitd.log" >&2; rm -rf "$ROOT"' EXIT
export GITD_SOCKET="$ROOT/gitd.sock"
for _ in $(seq 50); do [ -S "$GITD_SOCKET" ] && break; sleep 0.1; done

step() { printf '\n== %s\n' "$*"; }
fail() { printf 'FAILED: %s\n' "$*" >&2; exit 1; }

"$GITD" publishing set "https://github.com/$REPOSITORY" --preset recommended --branch-prefix agents/ \
  --pull-requests always --draft --label agent

step "a token for one repository"
"$GITD" --json token issue "https://github.com/$REPOSITORY" --for 15m > "$ROOT/issued"
TOKEN="$(jq -r .token "$ROOT/issued")"
CLONE="$(jq -r .cloneUrl "$ROOT/issued")"
SANDBOX="$(jq -r .info.sandbox "$ROOT/issued")"
echo "clone from $CLONE, sandbox $SANDBOX"

step "a sandbox on its own network clones, commits and pushes with it"
docker run --rm -e TOKEN="$TOKEN" -e CLONE="$CLONE" -e RUN="$RUN" --entrypoint sh alpine/git:2.47.2 -c '
  set -e
  address="$(echo "$CLONE" | sed "s#://#://x:${TOKEN}@#")"
  git clone -q "$address" /work && cd /work
  git config user.name sandbox && git config user.email sandbox@example.com
  echo "run $RUN" > "endpoint-$RUN.txt" && git add -A && git commit -q -m "endpoint: run $RUN"
  git push -q origin "HEAD:refs/heads/agents/endpoint-$RUN"
  echo "pushed; the remote names no credential of the host:"
  git remote get-url origin | sed "s#${TOKEN}#<gitd token>#"
'

step "a token for another repository opens nothing"
OTHER="$(echo "$CLONE" | sed "s#$REPOSITORY#dooservice/gitd#; s#://#://x:${TOKEN}@#")"
if docker run --rm alpine/git:2.47.2 ls-remote "$OTHER" > /dev/null 2>&1; then fail "the token opened another repository"; fi
echo "refused"

step "gitd publishes what the sandbox pushed"
"$GITD" --json publish --sandbox "$SANDBOX" "agents/endpoint-$RUN" --agent "e2e sandbox" | tee "$ROOT/published"
grep -q '"url": "https://github.com/'"$REPOSITORY"'/pull/' "$ROOT/published" || fail "no pull request was opened"

step "the token is revoked, and opens nothing after"
ID="$(jq -r .info.id "$ROOT/issued")"
"$GITD" token revoke "$ID" --remove-sandbox
if docker run --rm alpine/git:2.47.2 ls-remote "$(echo "$CLONE" | sed "s#://#://x:${TOKEN}@#")" > /dev/null 2>&1; then fail "a revoked token opened the repository"; fi
echo "refused"

printf '\nPASSED: %s\n' "$RUN"
