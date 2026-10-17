#!/usr/bin/env bash
# The benchmark itself, inside gitd's image (see hundred.sh).
set -euo pipefail
repo="$1" agents="$2" clones="$3"
readonly socket=/tmp/gitd.sock workspaces=/var/lib/gitd/workspaces at_once=8

# What gitd leaves on the host's disk goes when the benchmark ends, its mounts first; it
# starts from nothing too.
clear_state() {
  awk '$2 ~ "^/var/lib/gitd/" {print $2}' /proc/mounts | sort -r | xargs -r -n1 umount -l
  rm -rf /var/lib/gitd/*
}
clear_state
trap clear_state EXIT

now() { date +%s%N; }
seconds_since() { awk -v start="$1" -v end="$(now)" 'BEGIN {printf "%.2f", (end - start) / 1e9}'; }
divide() { awk -v total="$1" -v n="$2" 'BEGIN {printf "%.3f", total / n}'; }
megabytes() { du -sk "$@" 2>/dev/null | awk '{total += $1} END {printf "%.1f", total / 1024}'; }
agent() { echo "$workspaces/agent-$1"; }
prepare() { gitd prepare --socket "$socket" "$repo" "$(agent "$1")" >/dev/null; }
remove() { gitd remove --socket "$socket" "$(agent "$1")" >/dev/null; }
export -f prepare agent
export socket workspaces repo

start_gitd() {
  gitd serve --config /dev/null --socket "$socket" >/tmp/gitd.log 2>&1 &
  until gitd info --socket "$socket" >/dev/null 2>&1; do sleep 0.2; done
  local made; made=$(gitd info --socket "$socket" | tail -1)
  echo "$made" >&2
  [[ "$made" == "workspaces made by overlay"* ]] || { echo "overlay is not available here" >&2; exit 1; }
}

# clean COUNT: how many of the first COUNT workspaces git sees as clean checkouts.
clean() {
  local count=0
  for i in $(seq "$1"); do
    [ -z "$(git -C "$(agent "$i")" status --porcelain)" ] && count=$((count + 1))
  done
  echo "$count"
}

# isolated: an agent's commit stays in its workspace; the next one and the template are as
# they were.
isolated() {
  local first second
  first=$(agent 1) second=$(agent 2)
  echo "agent" > "$first/AGENT.md"
  git -C "$first" add AGENT.md
  git -C "$first" -c user.name=agent -c user.email=agent@example.com commit -qm "agent change"
  [ -z "$(git -C "$second" status --porcelain)" ] &&
    [ "$(git -C "$second" rev-parse HEAD)" != "$(git -C "$first" rev-parse HEAD)" ] && echo yes || echo NO
}

measure_gitd() {
  start_gitd
  local start first sequential each statuses
  start=$(now); prepare 1; first=$(seconds_since "$start")
  start=$(now)
  for i in $(seq 2 "$agents"); do prepare "$i"; done
  sequential=$(seconds_since "$start"); each=$(divide "$sequential" $((agents - 1)))

  start=$(now); local clean_count; clean_count=$(clean "$agents"); statuses=$(divide "$(seconds_since "$start")" "$agents")
  local disk_pool disk_template disk_layers per_workspace
  disk_pool=$(megabytes /var/lib/gitd/pools)
  disk_template=$(megabytes /var/lib/gitd/templates)
  disk_layers=$(megabytes /var/lib/gitd/layers)
  per_workspace=$(divide "$disk_layers" "$agents")
  local kept_apart; kept_apart=$(isolated)

  for i in $(seq 2 "$agents"); do remove "$i"; done
  start=$(now)
  seq 2 "$agents" | xargs -P "$at_once" -I{} bash -c 'prepare {}'
  local all_at_once; all_at_once=$(seconds_since "$start")

  start=$(now); for i in $(seq "$agents"); do remove "$i"; done
  local removed; removed=$(seconds_since "$start")

  echo "| $agents × \`gitd prepare\` (overlay) | ${first}s, then ${each}s each: $(awk -v f="$first" -v s="$sequential" 'BEGIN {printf "%.1f", f + s}')s | $((agents - 1)) in ${all_at_once}s, $at_once at a time | ${disk_pool} MB pool + ${disk_template} MB template + ${per_workspace} MB per workspace | 1 fetch |"
  echo
  echo "- Clean checkouts as git sees them: ${clean_count} of ${agents} (a first \`git status\` takes ${statuses}s on average)."
  echo "- An agent's commit stays in its workspace, the others untouched: ${kept_apart}."
  echo "- Removing all ${agents} workspaces: ${removed}s."
}

measure_clones() {
  [ "$clones" -gt 0 ] || return 0
  local dir=/var/lib/gitd/clone start total=0 disk=0 elapsed size
  for i in $(seq "$clones"); do
    start=$(now)
    git clone -q --depth 1 "$repo" "$dir"
    elapsed=$(seconds_since "$start"); size=$(megabytes "$dir")
    total=$(awk -v t="$total" -v e="$elapsed" 'BEGIN {printf "%.2f", t + e}')
    disk=$(awk -v d="$disk" -v s="$size" 'BEGIN {printf "%.1f", d + s}')
    rm -rf "$dir"
  done
  echo "| $clones × \`git clone --depth 1\` | ${total}s, $(divide "$total" "$clones")s each | — | ${disk} MB, measured clone by clone | $clones fetches |"
}

echo "## $agents agents on $repo"
echo
echo "$(git --version), $(uname -r), $(nproc) CPUs"
echo
echo "| How | Time | All at once | Disk | Network |"
echo "|---|---|---|---|---|"
measure_clones
measure_gitd
