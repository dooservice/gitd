#!/usr/bin/env bash
# Two sandboxes, one repository fetched once, and no token in either of them.
set -euo pipefail
cd "$(dirname "$0")"
repo="${1:-https://github.com/OCA/web}"
compose() { docker compose "$@"; }

compose up -d --build gitd
until compose exec -T gitd gitd info >/dev/null 2>&1; do sleep 0.5; done

echo "== gitd prepares a workspace per agent"
time compose exec -T gitd gitd prepare "$repo" /var/lib/gitd/workspaces/agent-1
time compose exec -T gitd gitd prepare "$repo" /var/lib/gitd/workspaces/agent-2

echo "== each agent works in its own copy, with plain git"
compose run --rm -T agent-1 sh -c 'echo "change by agent-1" > AGENT.md && git add AGENT.md && git commit -qm "agent-1 was here" && git log --oneline -1'
compose run --rm -T agent-2 sh -c 'git log --oneline -1 && git status --short | wc -l'

echo "== no credential in a sandbox"
compose run --rm -T agent-1 sh -c 'env | grep -ciE "token|secret|password" || true; git config --list --show-origin | grep -ci credential || true'

echo "== the disk: one pool, two workspaces that borrow from it"
compose exec -T gitd sh -c 'du -sh /var/lib/gitd/pools /var/lib/gitd/workspaces/agent-1/.git /var/lib/gitd/workspaces/agent-2/.git'

echo "== to publish agent-1's commit, set [[credentials]] in gitd.toml and GITD_TOKEN, then:"
echo "   docker compose exec gitd gitd publish /var/lib/gitd/workspaces/agent-1 agent-1/change"
