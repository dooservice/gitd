#!/usr/bin/env bash
# Two tenants on one gitd, and a sandbox on its own network that clones and pushes with a
# token of gitd's — no mount, no credential of the git host in it.
#
#   ./demo.sh       run it
#   ./demo.sh down  stop everything and delete ./state and ./keys
set -euo pipefail
cd "$(dirname "$0")"
export HOST_UID="$(id -u)" HOST_GID="$(id -g)"
compose() { docker compose "$@"; }
as() { local tenant="$1"; shift; compose exec -T gitd gitd --key-file "/keys/$tenant" "$@"; }

if [ "${1:-}" = down ]; then
  compose down
  rm -rf state keys
  exit 0
fi

ACME_REPO="${ACME_REPO:-https://github.com/octocat/Hello-World}"
GLOBEX_REPO="${GLOBEX_REPO:-https://github.com/octocat/Spoon-Knife}"
mkdir -p keys state
for tenant in acme globex; do
  [ -s "keys/$tenant" ] || head -c 24 /dev/urandom | base64 > "keys/$tenant"
done
chmod 600 keys/*

compose up -d --build gitd
until compose exec -T gitd gitd --key-file /keys/acme info > /dev/null 2>&1; do sleep 0.5; done

echo "== a call without a tenant's key is refused"
compose exec -T gitd gitd info 2>&1 | tail -1 || true

echo "== acme issues a token for a sandbox"
as acme --json token issue "$ACME_REPO" --for 30m > state/acme-token.json
TOKEN="$(sed -n 's/.*"token": "\([^"]*\)".*/\1/p' state/acme-token.json)"
CLONE="$(sed -n 's/.*"cloneUrl": "\([^"]*\)".*/\1/p' state/acme-token.json)"
SANDBOX="$(sed -n 's/.*"sandbox": "\([^"]*\)".*/\1/p' state/acme-token.json)"
echo "clone from $CLONE"
AUTHENTICATED="$(echo "$CLONE" | sed "s#://#://x:${TOKEN}@#")"

echo "== the sandbox, on its own network with nothing mounted, clones, commits and pushes"
compose run --rm -T sandbox "
  set -e
  git clone -q '$AUTHENTICATED' /work && cd /work
  git -c user.name=agent -c user.email=agent@example.com commit -q --allow-empty -m 'agent was here'
  git push -q origin HEAD:refs/heads/agents/demo
  env | grep -ciE 'github_token|gh_token' || echo 'no credential of the git host in the sandbox'
"

echo "== acme's token opens nothing of globex's"
OTHER="$(echo "$AUTHENTICATED" | sed "s#octocat/hello-world#octocat/spoon-knife#")"
compose run --rm -T sandbox "git ls-remote '$OTHER' > /dev/null 2>&1 && echo OPENED || echo refused"

echo "== globex sees none of acme's tokens"
as globex token list

echo "== what acme's sandbox pushed waits in its sandbox repository, to publish"
echo "   gitd --key-file /keys/acme publish --sandbox $SANDBOX agents/demo"
echo "   (give acme a credential in gitd.toml first)"
as acme audit --limit 2
echo "== ./demo.sh down removes it all"
