#!/usr/bin/env bash
# Two sandboxes with copy-on-write workspaces of one repository, fetched once, and no token
# in either.
#
#   ./demo.sh [REPO]     run it
#   ./demo.sh down       remove the workspaces, stop everything, and delete ./state
set -euo pipefail
cd "$(dirname "$0")"
compose() { docker compose "$@"; }
gitd() { compose exec -T gitd gitd "$@"; }

if [ "${1:-}" = down ]; then
  for agent in agent-1 agent-2; do gitd remove "/var/lib/gitd/workspaces/$agent" 2>/dev/null || true; done
  compose down
  docker run --rm -v "$PWD/state:/state" alpine sh -c 'rm -rf /state/*' && rmdir state
  exit 0
fi

repo="${1:-https://github.com/OCA/web}"
# The pool's path, which the sandboxes mount: <host>/<path>.git, lowercased.
pool="$(echo "$repo" | sed -E 's#^https://##; s#\.git$##' | tr '[:upper:]' '[:lower:]').git"
export POOL="$pool"

compose up -d --build gitd
until gitd info >/dev/null 2>&1; do sleep 0.5; done
gitd info | tail -1

echo "== gitd prepares a workspace per agent: the first fetches, the second takes milliseconds"
time gitd prepare "$repo" /var/lib/gitd/workspaces/agent-1
time gitd prepare "$repo" /var/lib/gitd/workspaces/agent-2

echo "== each agent works in its own copy, with plain git"
compose run --rm -T agent-1 sh -c 'echo "change by agent-1" > AGENT.md && git add AGENT.md && git commit -qm "agent-1 was here" && git log --oneline -1'
compose run --rm -T agent-2 sh -c 'git log --oneline -1 && git status --short | wc -l'

echo "== no credential in a sandbox"
compose run --rm -T agent-1 sh -c 'env | grep -ciE "token|secret|password" || true; ls /run/gitd 2>&1 | head -1'

echo "== the disk: one pool and one template, and what each workspace changed"
compose exec -T gitd sh -c 'du -sh /var/lib/gitd/pools /var/lib/gitd/templates /var/lib/gitd/layers/*/upper'

echo "== to publish agent-1's commit, set [[credentials]] in gitd.toml and GITD_TOKEN, then:"
echo "   docker compose exec gitd gitd publish /var/lib/gitd/workspaces/agent-1 agent-1/change"
echo "== ./demo.sh down removes it all"
