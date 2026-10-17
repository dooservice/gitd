#!/usr/bin/env bash
# N agents on one repository, with copy-on-write workspaces: how long gitd takes to give each
# its own workspace, one after another and all at once, the disk they take, and whether git
# sees every one as a clean checkout; against N `git clone --depth 1`.
#
# Runs gitd's image as root on a directory of the host, where it can mount overlays:
#
#   docker build -t gitd:bench . && sudo benchmarks/hundred.sh [REPO] [N] [STATE] [CLONES]
#
# STATE is the host directory for gitd's state (default /mnt/gitd-bench); CLONES is how many
# clones to measure for the comparison (default N, 0 to skip).
set -euo pipefail
repo="${1:-https://github.com/kubernetes/kubernetes}"
agents="${2:-100}"
state="${3:-/mnt/gitd-bench}"
clones="${4:-$agents}"
here="$(cd "$(dirname "$0")" && pwd)"

mkdir -p "$state"
docker run --rm --privileged --user 0 \
  -v "$state:/var/lib/gitd" -v "$here:/bench:ro" \
  --entrypoint bash gitd:bench /bench/hundred-run.sh "$repo" "$agents" "$clones"
