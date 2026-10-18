#!/usr/bin/env bash
# Both clients reach a gitd on the network, as a platform on another machine does: over
# TLS with a certificate of its own, with the key its network address asks for, and are
# refused without it. Needs bin/gitd, openssl, python3 with the Python client installed, and
# node with sdk/typescript built.
set -euo pipefail
ROOT="$(mktemp -d)"
GITD="$PWD/bin/gitd"
ADDRESS="127.0.0.1:9432"
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 1 \
  -subj "/CN=gitd" -addext "subjectAltName=IP:127.0.0.1" \
  -keyout "$ROOT/key.pem" -out "$ROOT/cert.pem" 2> /dev/null
head -c 24 /dev/urandom | base64 > "$ROOT/api.key"
mkdir -p "$ROOT/workspaces"
cat > "$ROOT/gitd.toml" <<TOML
socket = "$ROOT/gitd.sock"
state = "$ROOT/state"
roots = ["$ROOT/workspaces"]
api_key_file = "$ROOT/api.key"

[endpoint]
listen = "$ADDRESS"
tls_cert_file = "$ROOT/cert.pem"
tls_key_file = "$ROOT/key.pem"
TOML
"$GITD" serve --config "$ROOT/gitd.toml" > "$ROOT/gitd.log" 2>&1 &
SERVING=$!
trap 'kill $SERVING 2>/dev/null; cat "$ROOT/gitd.log" >&2; rm -rf "$ROOT"' EXIT
for _ in $(seq 50); do [ -S "$ROOT/gitd.sock" ] && break; sleep 0.1; done

export GITD_URL="https://$ADDRESS" GITD_KEY_FILE="$ROOT/api.key" GITD_CA_FILE="$ROOT/cert.pem"

echo "== the command line"
"$GITD" info | head -1

echo "== the Python client"
python3 - <<'PY'
from gitd import Gitd, GitdError
with Gitd() as gitd:
    print("reached", gitd.info().version, "with", len(gitd.endpoint.tokens()), "tokens")
try:
    Gitd(key="wrong").info()
    raise SystemExit("a wrong key was answered")
except GitdError as refused:
    print("a wrong key is refused:", refused.code)
PY

echo "== the TypeScript client"
(cd sdk/typescript && node --input-type=module -e '
import { Gitd } from "./dist/index.js";
const info = await new Gitd().info();
console.log("reached", info.version);
try {
  await new Gitd({ key: "wrong" }).info();
  process.exit(1);
} catch {
  console.log("a wrong key is refused");
}
')
echo "PASSED"
