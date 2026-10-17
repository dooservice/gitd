#!/usr/bin/env bash
# N agents, one repository: N `git clone`s against N `gitd prepare`s, in time and disk.
# Runs in gitd's image, so the numbers do not depend on the machine's git:
#
#   docker build -t gitd:bench . && benchmarks/agents.sh [REPO] [N]
set -euo pipefail
repo="${1:-https://github.com/kubernetes/kubernetes}"
agents="${2:-10}"
here="$(cd "$(dirname "$0")" && pwd)"
docker run --rm -v "$here:/bench:ro" --entrypoint bash gitd:bench /bench/run.sh "$repo" "$agents"
