#!/usr/bin/env bash
# A command in a bubblewrap sandbox — as Claude Code and Codex CLI isolate the commands they
# run on Linux — working in a workspace gitd made: it reads the whole system, writes only
# its workspace, and sees no socket or credential of gitd's.
#
#   make build && examples/bubblewrap/demo.sh [REPO]
#
# Needs bwrap (apt install bubblewrap) and gitd at ./bin/gitd.
set -euo pipefail
repo="${1:-https://github.com/octocat/Hello-World}"
GITD="$PWD/bin/gitd"
ROOT="$(mktemp -d)"
cat > "$ROOT/gitd.toml" <<TOML
socket = "$ROOT/run/gitd.sock"
state = "$ROOT/state"
roots = ["$ROOT/workspaces"]
TOML
mkdir -p "$ROOT/workspaces" "$ROOT/run"
"$GITD" serve --config "$ROOT/gitd.toml" > "$ROOT/gitd.log" 2>&1 &
serving=$!
trap 'kill $serving 2>/dev/null; rm -rf "$ROOT"' EXIT
export GITD_SOCKET="$ROOT/run/gitd.sock"
until "$GITD" info > /dev/null 2>&1; do sleep 0.2; done

workspace="$ROOT/workspaces/agent-1"
"$GITD" prepare "$repo" "$workspace"

echo "== the agent's command, in bubblewrap: the system read-only, its workspace writable"
bwrap --ro-bind / / --dev /dev --proc /proc \
  --tmpfs "$ROOT/run" --tmpfs "$ROOT/state/connections" \
  --bind "$workspace" "$workspace" --chdir "$workspace" \
  --unshare-pid --die-with-parent \
  sh -c '
    echo "a change by the agent" > AGENT.md
    git add AGENT.md
    git -c user.name=agent -c user.email=agent@example.com commit -q -m "agent was here"
    git log --oneline -1
    echo nope > ../outside.txt 2>/dev/null && echo "WROTE OUTSIDE" || echo "cannot write outside the workspace"
    ls "'"$ROOT"'/run" | grep -q . && echo "SOCKET VISIBLE" || echo "gitd'"'"'s socket is not there"
  '

echo "== gitd reads what the agent changed"
"$GITD" changes "$workspace"
echo "== to publish: gitd publish $workspace agents/fix (with a credential gitd keeps)"
