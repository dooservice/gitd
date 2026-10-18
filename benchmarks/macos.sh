#!/usr/bin/env bash
# N workspaces of one repository on macOS, where templates are cloned by APFS: how long gitd
# takes for each and the disk they take, as the volume's free space says — du counts a clone
# whole, though it shares its template's blocks — against N `git clone --depth 1`.
#
#   make build && benchmarks/macos.sh [REPO] [N]
set -euo pipefail
repo="${1:-https://github.com/kubernetes/kubernetes}"
agents="${2:-10}"
GITD="$PWD/bin/gitd"
ROOT="$(mktemp -d /tmp/gitd-macos.XXXXXX)"
socket="$ROOT/gitd.sock"
cat > "$ROOT/gitd.toml" <<TOML
socket = "$socket"
state = "$ROOT/state"
roots = ["$ROOT/workspaces"]
TOML
mkdir -p "$ROOT/workspaces"
"$GITD" serve --config "$ROOT/gitd.toml" > "$ROOT/gitd.log" 2>&1 &
serving=$!
trap 'kill $serving 2>/dev/null; rm -rf "$ROOT"' EXIT
until "$GITD" info --socket "$socket" > /dev/null 2>&1; do sleep 0.2; done

now() { python3 -c 'import time; print(time.time())'; }
since() { python3 -c "import time; print(f'{time.time() - $1:.2f}')"; }
free_kb() { df -k "$ROOT" | awk 'NR == 2 {print $4}'; }
megabytes() { python3 -c "print(f'{$1 / 1024:.1f}')"; }

made=$("$GITD" info --socket "$socket" | tail -1)
start=$(now); "$GITD" prepare --socket "$socket" "$repo" "$ROOT/workspaces/agent-1" > /dev/null; first=$(since "$start")
before=$(free_kb)
start=$(now)
for i in $(seq 2 "$agents"); do "$GITD" prepare --socket "$socket" "$repo" "$ROOT/workspaces/agent-$i" > /dev/null; done
rest=$(since "$start")
after=$(free_kb)
each=$(python3 -c "print(f'{$rest / ($agents - 1):.3f}')")
per_workspace=$(python3 -c "print(f'{($before - $after) / 1024 / ($agents - 1):.1f}')")
clean=0
for i in $(seq "$agents"); do
  [ -z "$(git -C "$ROOT/workspaces/agent-$i" status --porcelain)" ] && clean=$((clean + 1))
done
start=$(now); git clone -q --depth 1 "$repo" "$ROOT/clone"; clone=$(since "$start")
clone_disk=$(du -sk "$ROOT/clone" | awk '{print $1}')

echo "## $agents workspaces of $repo on macOS"
echo
echo "$(sw_vers -productName) $(sw_vers -productVersion), $(uname -m), $(git --version). $made"
echo
echo "| How | Time | Disk |"
echo "|---|---|---|"
echo "| \`gitd prepare\` | the first ${first}s, then ${each}s each | ${per_workspace} MB each, by the volume's free space |"
echo "| \`git clone --depth 1\` | ${clone}s each | $(megabytes "$clone_disk") MB each |"
echo
echo "- Clean checkouts as git sees them: ${clean} of ${agents}."
