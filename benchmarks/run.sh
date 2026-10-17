#!/usr/bin/env bash
# The benchmark itself, inside gitd's image (see agents.sh).
set -euo pipefail
repo="$1" agents="$2"
work=/var/lib/gitd/bench
mkdir -p "$work"

seconds() { local start=$1; echo "$(( ($(date +%s%N) - start) / 1000000 ))" | awk '{printf "%.1f", $1/1000}'; }
disk() { du -sm "$@" | awk '{total += $1} END {print total}'; }

# run LABEL CLONE_FLAGS GITD_FLAGS, each flag list as one word split on spaces.
run() {
  local label=$1 clone_flags gitd_flags
  read -ra clone_flags <<< "$2"
  read -ra gitd_flags <<< "$3"
  rm -rf "${work:?}"/* /var/lib/gitd/pools

  local start; start=$(date +%s%N)
  for i in $(seq "$agents"); do git clone -q "${clone_flags[@]}" "$repo" "$work/clone-$i"; done
  local clone_time clone_disk; clone_time=$(seconds "$start"); clone_disk=$(disk "$work"/clone-*)

  gitd --config /dev/null --socket /tmp/gitd.sock >/tmp/gitd.log 2>&1 &
  local server=$!
  until gitd info --socket /tmp/gitd.sock >/dev/null 2>&1; do sleep 0.2; done
  start=$(date +%s%N)
  local first=""
  for i in $(seq "$agents"); do
    gitd prepare --socket /tmp/gitd.sock "${gitd_flags[@]}" "$repo" "/var/lib/gitd/workspaces/agent-$i" >/dev/null
    [ -z "$first" ] && first=$(seconds "$start")
  done
  local gitd_time gitd_disk; gitd_time=$(seconds "$start")
  gitd_disk=$(disk /var/lib/gitd/pools /var/lib/gitd/workspaces/agent-*)
  local each; each=$(awk -v total="$gitd_time" -v first="$first" -v n="$agents" 'BEGIN {printf "%.2f", (total - first) / (n - 1)}')
  kill "$server"; wait "$server" 2>/dev/null || true
  rm -rf /var/lib/gitd/workspaces/agent-*

  printf '| %s | %s × git clone | %ss | %s MB | %s fetches |\n' "$label" "$agents" "$clone_time" "$clone_disk" "$agents"
  printf '| %s | %s × gitd prepare | %ss (first %ss, then %ss each) | %s MB | 1 fetch |\n' "$label" "$agents" "$gitd_time" "$first" "$each" "$gitd_disk"
}

echo "Repository: $repo, $agents agents, $(git --version)"
echo
echo "| History | How | Time | Disk | Network |"
echo "|---|---|---|---|---|"
run "tip only" "--depth 1" ""
run "whole" "" "--full-history"
