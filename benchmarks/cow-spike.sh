#!/usr/bin/env bash
# Roadmap 0.7.0: can a workspace be made from a shared template in milliseconds, with almost
# no disk, and does git notice nothing? Measures, for N agents on one repository, the
# workspace as gitd makes it today (copy) against kernel overlay, reflink on XFS and
# fuse-overlayfs, and checks git on each. Needs root (overlay, a loop XFS); the CI runner
# has it:  sudo benchmarks/cow-spike.sh [REPO] [N]
set -euo pipefail
trap 'echo "spike stopped at line $LINENO: $BASH_COMMAND" >&2' ERR
repo="${1:-https://github.com/kubernetes/kubernetes}"
agents="${2:-100}"
branch="${3:-master}"
export branch
base=/mnt/spike
user="${SUDO_USER:-runner}"

rm -rf "$base" && mkdir -p "$base"
cd "$base"
now() { date +%s%N; }
since() { awk -v start="$1" -v end="$(now)" 'BEGIN {printf "%.2f", (end - start) / 1e9}'; }
mb() { du -sm "$@" 2>/dev/null | awk '{total += $1} END {print total + 0}'; }
row() { printf '| %s | %s | %s | %s | %s |\n' "$@"; }

# git as gitd runs it: no global configuration, no hooks.
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
# The spike's repositories change owner between root and the unprivileged user (for
# fuse-overlayfs); command-line configuration is where git takes safe.directory from.
export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=safe.directory GIT_CONFIG_VALUE_0='*'
export GIT_AUTHOR_NAME=agent GIT_AUTHOR_EMAIL=agent@example.com
export GIT_COMMITTER_NAME=agent GIT_COMMITTER_EMAIL=agent@example.com

echo "## Copy-on-write spike: $repo, $agents agents"
echo
echo "$(git --version), $(uname -r), $(nproc) CPUs"
echo

# The pool and the template, as gitd builds a workspace today.
git init -q --bare pool.git
start=$(now)
git -C pool.git fetch -q --depth 1 "$repo" "+refs/heads/$branch:refs/heads/$branch"
fetch_time=$(since "$start")

make_workspace() {
  local dir=$1
  git init -q "$dir"
  echo "$base/pool.git/objects" > "$dir/.git/objects/info/alternates"
  cp pool.git/shallow "$dir/.git/shallow"
  git -C "$dir" fetch -q "$base/pool.git" "+refs/heads/$branch:refs/remotes/origin/$branch"
  git -C "$dir" checkout -q -B "$branch" "origin/$branch"
}
start=$(now)
make_workspace template
template_time=$(since "$start")
files=$(git -C template ls-files | wc -l)
template_mb=$(mb template --exclude=.git)
start=$(now)
make_workspace copy-2
copy_time=$(since "$start")
copy_mb=$(mb copy-2)
rm -rf copy-2

echo "Fetch into the pool: ${fetch_time}s. Template: ${files} files, ${template_mb} MB, built in ${template_time}s."
echo

# check DIR: what an agent does, and whether git sees the tree as it is.
check() {
  local dir=$1
  [ -z "$(git -C "$dir" status --porcelain)" ] || { echo "dirty before any change"; return 1; }
  echo "agent" >> "$dir/README.md"
  [ "$(git -C "$dir" status --porcelain)" = " M README.md" ] || { echo "status wrong after a change"; return 1; }
  git -C "$dir" diff --quiet && { echo "diff saw nothing"; return 1; }
  if ! git -C "$dir" stash -q || [ -n "$(git -C "$dir" status --porcelain)" ]; then echo "stash failed"; return 1; fi
  git -C "$dir" stash pop -q
  git -C "$dir" commit -qam "agent change" || { echo "commit failed"; return 1; }
  git -C "$dir" checkout -q -b agent/work || { echo "checkout failed"; return 1; }
  git -C "$dir" checkout -q "$branch" || { echo "checkout back failed"; return 1; }
  echo ok
}

configure() { git -C "$1" config core.checkStat minimal && git -C "$1" config core.trustctime false; }

first_status() {
  local dir=$1 start
  start=$(now)
  git -C "$dir" status --porcelain >/dev/null
  echo "$(since "$start")s"
}

echo "| Strategy | New workspace | Disk per workspace | First git status (default / tuned) | git |"
echo "|---|---|---|---|---|"
row "copy (today)" "${copy_time}s" "${copy_mb} MB" "–" "ok"

# --- kernel overlay ---------------------------------------------------------------------
mkdir -p overlay
start=$(now)
for i in $(seq "$agents"); do
  mkdir -p "overlay/$i/upper" "overlay/$i/work" "overlay/$i/merged"
  mount -t overlay overlay -o "lowerdir=$base/template,upperdir=$base/overlay/$i/upper,workdir=$base/overlay/$i/work" "overlay/$i/merged"
done
overlay_total=$(since "$start")
overlay_each=$(awk -v total="$overlay_total" -v n="$agents" 'BEGIN {printf "%.3f", total / n}')
default_status=$(first_status overlay/1/merged)
configure overlay/2/merged
tuned_status=$(first_status overlay/2/merged)
overlay_git=$(check overlay/3/merged 2>&1 | tail -1)
overlay_mb=$(mb overlay/*/upper | awk -v n="$agents" '{printf "%.2f", $1 / n}')
row "overlay (kernel)" "${overlay_each}s (${agents} in ${overlay_total}s)" "${overlay_mb} MB" "${default_status} / ${tuned_status}" "$overlay_git"
for i in $(seq "$agents"); do umount "overlay/$i/merged"; done

# --- fuse-overlayfs, unprivileged ---------------------------------------------------------
if command -v fuse-overlayfs >/dev/null; then
  mkdir -p fuse && chown -R "$user" fuse template pool.git
  start=$(now)
  sudo -u "$user" bash -c '
    for i in $(seq "$1"); do
      mkdir -p "fuse/$i/upper" "fuse/$i/work" "fuse/$i/merged"
      fuse-overlayfs -o "lowerdir=$2/template,upperdir=$2/fuse/$i/upper,workdir=$2/fuse/$i/work" "fuse/$i/merged"
    done' _ "$agents" "$base"
  fuse_total=$(since "$start")
  fuse_each=$(awk -v total="$fuse_total" -v n="$agents" 'BEGIN {printf "%.3f", total / n}')
  # A strategy that fails is reported, not fatal: the next one is still measured.
  fuse_status=$(sudo -u "$user" -E bash -c "$(declare -f now since first_status configure); configure fuse/2/merged; first_status fuse/2/merged" 2>"$base/fuse.err") \
    || fuse_status="failed: $(tail -1 "$base/fuse.err")"
  fuse_git=$(sudo -u "$user" -E bash -c "$(declare -f check); check fuse/3/merged" 2>&1 | tail -1) || true
  # What fuse-overlayfs refuses, and where, for the record.
  fuse_diagnosis=$(sudo -u "$user" bash -c 'touch fuse/4/merged/probe-root 2>&1 && echo "root: writable"; touch fuse/4/merged/.git/probe-git 2>&1 && echo ".git: writable"; mkdir fuse/4/merged/.git/probe-dir 2>&1 && echo ".git: mkdir ok"' 2>&1 | tr '\n' ';')
  fuse_mb=$(mb fuse/*/upper | awk -v n="$agents" '{printf "%.2f", $1 / n}')
  row "fuse-overlayfs (no privilege)" "${fuse_each}s (${agents} in ${fuse_total}s)" "${fuse_mb} MB" "– / ${fuse_status}" "$fuse_git"
  for i in $(seq "$agents"); do sudo -u "$user" fusermount3 -u "fuse/$i/merged" 2>/dev/null || fusermount -u "fuse/$i/merged"; done
fi

# --- reflink on XFS -----------------------------------------------------------------------
if command -v mkfs.xfs >/dev/null; then
  truncate -s 40G xfs.img
  mkfs.xfs -q -m reflink=1 xfs.img
  mkdir -p xfs && mount -o loop xfs.img xfs
  cp -a template xfs/template
  used_before=$(df -m --output=used xfs | tail -1)
  start=$(now)
  for i in $(seq "$agents"); do cp -a --reflink=always xfs/template "xfs/$i"; done
  reflink_total=$(since "$start")
  reflink_each=$(awk -v total="$reflink_total" -v n="$agents" 'BEGIN {printf "%.3f", total / n}')
  used_after=$(df -m --output=used xfs | tail -1)
  reflink_mb=$(awk -v a="$used_after" -v b="$used_before" -v n="$agents" 'BEGIN {printf "%.2f", (a - b) / n}')
  default_status=$(first_status xfs/1) || default_status="failed"
  { configure xfs/2 && tuned_status=$(first_status xfs/2); } || tuned_status="failed"
  reflink_git=$(check xfs/3 2>&1 | tail -1) || true
  row "reflink (XFS)" "${reflink_each}s (${agents} in ${reflink_total}s)" "${reflink_mb} MB" "${default_status} / ${tuned_status}" "$reflink_git"
  umount xfs
fi

echo
[ -n "${fuse_diagnosis:-}" ] && echo "fuse-overlayfs probe: ${fuse_diagnosis}"
echo "The template is untouched: $( [ -z "$(git -C template status --porcelain)" ] && echo yes || echo NO )."
echo "Copy (today) per workspace: ${copy_time}s and ${copy_mb} MB, for ${files} files."
