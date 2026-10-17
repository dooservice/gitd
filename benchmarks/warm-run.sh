#!/usr/bin/env bash
# The benchmark itself, inside a Node.js image with gitd (see warm.sh).
set -euo pipefail
repo="$1" agents="$2" install="$3"
readonly socket=/tmp/gitd.sock workspaces=/var/lib/gitd/workspaces

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
agent() { echo "$workspaces/$1"; }
gitd_() { gitd --socket "$socket" "$@"; }
installs_in() { (cd "$1" && sh -c "$install" >/dev/null 2>&1); }
# installed DIR: whether the workspace holds what was installed, as node resolves it.
installed() { [ -d "$1/node_modules" ] && [ -n "$(ls -A "$1/node_modules")" ]; }

gitd serve --config /dev/null --socket "$socket" >/tmp/gitd.log 2>&1 &
until gitd_ info >/dev/null 2>&1; do sleep 0.2; done
made=$(gitd_ info | tail -1)
[[ "$made" == "workspaces made by overlay"* ]] || { echo "overlay is not available here: $made" >&2; exit 1; }

# One install, sealed.
gitd_ prepare "$repo" "$(agent install)" >/dev/null
start=$(now); installs_in "$(agent install)"; first_install=$(seconds_since "$start")
modules=$(megabytes "$(agent install)/node_modules")
# tree DIR: the dependency tree npm reads in a workspace, without its first line, which
# names the workspace's path.
tree() { (cd "$1" && { npm ls --all 2>/dev/null || true; } | tail -n +2); }
installed_tree=$(tree "$(agent install)")
start=$(now); sealed=$(gitd_ seal "$(agent install)" deps); seal_time=$(seconds_since "$start")

# N workspaces on the warm layer.
start=$(now)
for i in $(seq "$agents"); do gitd_ prepare "$repo" "$(agent "warm-$i")" --warm deps >/dev/null; done
warm_total=$(seconds_since "$start")
holding=0 clean=0
for i in $(seq "$agents"); do
  installed "$(agent "warm-$i")" && holding=$((holding + 1))
  [ -z "$(git -C "$(agent "warm-$i")" status --porcelain)" ] && clean=$((clean + 1))
done
# npm reads the same dependency tree as in the workspace the layer was sealed from.
[ "$(tree "$(agent warm-1)")" = "$installed_tree" ] && same_tree=yes || same_tree=NO
warm_disk=$(megabytes /var/lib/gitd/layers)
warm_layer=$(megabytes /var/lib/gitd/templates/.warm)
for i in $(seq "$agents"); do gitd_ remove "$(agent "warm-$i")" >/dev/null; done

# N workspaces, each installing its own, with npm's cache already warm from the first.
start=$(now)
for i in $(seq "$agents"); do
  gitd_ prepare "$repo" "$(agent "cold-$i")" >/dev/null
  installs_in "$(agent "cold-$i")"
done
cold_total=$(seconds_since "$start")
cold_holding=0
for i in $(seq "$agents"); do installed "$(agent "cold-$i")" && cold_holding=$((cold_holding + 1)); done
cold_disk=$(megabytes /var/lib/gitd/layers)
for i in $(seq "$agents"); do gitd_ remove "$(agent "cold-$i")" >/dev/null; done

echo "## $agents workspaces of $repo, dependencies installed"
echo
echo "$(git --version), node $(node --version), $(uname -r), $(nproc) CPUs. Install: \`$install\`,"
echo "${first_install}s the first time, ${modules} MB of node_modules; sealed in ${seal_time}s ($sealed)."
echo
echo "| How | Time | Disk of the workspaces | Holding node_modules |"
echo "|---|---|---|---|"
echo "| $agents × \`gitd prepare --warm\` | ${warm_total}s, $(divide "$warm_total" "$agents")s each | ${warm_disk} MB, plus the ${warm_layer} MB layer once | ${holding} of ${agents} |"
echo "| $agents × \`gitd prepare\` + install | ${cold_total}s, $(divide "$cold_total" "$agents")s each | ${cold_disk} MB | ${cold_holding} of ${agents} |"
echo
echo "- Clean checkouts as git sees them on the warm layer: ${clean} of ${agents}."
echo "- \`npm ls --all\` reads, on the warm layer, the tree of the install it was sealed from: ${same_tree} ($(echo "$installed_tree" | wc -l) lines)."
