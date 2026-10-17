#!/usr/bin/env bash
# The OpenHands example end to end, as CI runs it: gitd serving with a GitHub App it keeps,
# publishing settings set through the API, OpenHands at work in its sandbox, and the pull
# request opened as the App's bot.
#
# It needs the App's ID and private key (GITD_E2E_APP_ID, GITD_E2E_APP_KEY), the App
# installed on REPOSITORY, gitd built at ./bin/gitd and Docker. The branch it pushes is named
# after RUN; whoever runs it deletes it afterwards.
set -euo pipefail

: "${GITD_E2E_APP_ID:?the App's ID}" "${GITD_E2E_APP_KEY:?the App's private key}"
REPOSITORY="${REPOSITORY:-dooservice/gitd-e2e}"
export RUN="${RUN:-local-$(date +%s)}"
here="$(cd "$(dirname "$0")" && pwd)"
GITD="$PWD/bin/gitd"
STATE="$(mktemp -d)"

umask 077
printf '%s\n' "$GITD_E2E_APP_KEY" > "$STATE/app.pem"
umask 022
mkdir -p "$STATE/workspaces"
cat > "$STATE/gitd.toml" <<TOML
socket = "$STATE/gitd.sock"
state = "$STATE/state"
roots = ["$STATE/workspaces"]

[[credentials]]
prefix = "https://github.com/$REPOSITORY"
github_app = { app_id = $GITD_E2E_APP_ID, private_key_file = "$STATE/app.pem" }
TOML
"$GITD" serve --config "$STATE/gitd.toml" > "$STATE/gitd.log" 2>&1 &
serving=$!
trap 'kill $serving 2>/dev/null; cat "$STATE/gitd.log" >&2; rm -rf "$STATE"' EXIT
export GITD_SOCKET="$STATE/gitd.sock"
for _ in $(seq 50); do [ -S "$GITD_SOCKET" ] && break; sleep 0.1; done

"$GITD" publishing set "https://github.com/$REPOSITORY" --preset recommended --branch-prefix agents/ \
  --pull-requests always --draft --label agent ${REVIEWER:+--review-requester}
docker build -q -t gitd-openhands-sandbox "$here/sandbox" > /dev/null

REPOSITORY="https://github.com/$REPOSITORY" ROOT="$STATE/workspaces" python "$here/openhands_task.py"
