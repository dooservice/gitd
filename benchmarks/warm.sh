#!/usr/bin/env bash
# Workspaces whose dependencies are installed already: N workspaces of a JavaScript
# repository standing on a warm layer sealed after one install, against N workspaces each
# installing its own — in time, disk, and whether each holds what was installed.
#
# Runs gitd with Node.js as root on a directory of the host, where it can mount overlays:
#
#   docker build -t gitd:bench . && sudo benchmarks/warm.sh [REPO] [N] [STATE] [INSTALL]
#
# STATE is the host directory for gitd's state (default /mnt/gitd-warm); INSTALL is the
# command that installs the dependencies (default `npm ci --ignore-scripts`).
set -euo pipefail
repo="${1:-https://github.com/socketio/socket.io}"
agents="${2:-10}"
state="${3:-/mnt/gitd-warm}"
install="${4:-npm ci --ignore-scripts --no-audit --no-fund}"
here="$(cd "$(dirname "$0")" && pwd)"

docker build -q -t gitd:warm-bench - >/dev/null <<'DOCKERFILE'
FROM node:24-bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=gitd:bench /usr/local/bin/gitd /usr/local/bin/gitd
DOCKERFILE

mkdir -p "$state"
docker run --rm --privileged --user 0 \
  -v "$state:/var/lib/gitd" -v "$here:/bench:ro" \
  --entrypoint bash gitd:warm-bench /bench/warm-run.sh "$repo" "$agents" "$install"
