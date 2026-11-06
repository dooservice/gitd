#!/usr/bin/env bash
# The Rust client calls a real gitd, on its socket and then on the network over TLS with a
# certificate of its own and the key its network address asks for, on Linux, macOS and
# Windows (Git Bash): the integration tests of sdk/rust, ignored otherwise, run against it.
# gitd, openssl and the tests are given native paths, C:/… on Windows, where Git Bash is also
# told not to rewrite openssl's subject as a path. Needs bin/gitd, openssl and cargo.
set -euo pipefail
ROOT="$(mktemp -d)"
GITD="${GITD:-$PWD/bin/gitd}"
ADDRESS="127.0.0.1:9433"
native() { if command -v cygpath > /dev/null; then cygpath -m "$1"; else printf '%s' "$1"; fi; }
NATIVE_ROOT="$(native "$ROOT")"
MSYS_NO_PATHCONV=1 openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 1 \
  -subj "/CN=gitd" -addext "subjectAltName=IP:127.0.0.1" -addext "basicConstraints=critical,CA:FALSE" \
  -keyout "$NATIVE_ROOT/key.pem" -out "$NATIVE_ROOT/cert.pem" 2> /dev/null
head -c 24 /dev/urandom | base64 > "$ROOT/api.key"
mkdir -p "$ROOT/workspaces"
cat > "$ROOT/gitd.toml" <<TOML
socket = "$NATIVE_ROOT/gitd.sock"
state = "$NATIVE_ROOT/state"
roots = ["$NATIVE_ROOT/workspaces"]
api_key_file = "$NATIVE_ROOT/api.key"

[endpoint]
listen = "$ADDRESS"
tls_cert_file = "$NATIVE_ROOT/cert.pem"
tls_key_file = "$NATIVE_ROOT/key.pem"
TOML
"$GITD" serve --config "$NATIVE_ROOT/gitd.toml" > "$ROOT/gitd.log" 2>&1 &
SERVING=$!
trap 'kill $SERVING 2>/dev/null; cat "$ROOT/gitd.log" >&2; rm -rf "$ROOT"' EXIT
for _ in $(seq 100); do "$GITD" info --socket "$NATIVE_ROOT/gitd.sock" > /dev/null 2>&1 && break; sleep 0.1; done

export GITD_TEST_ROOT="$NATIVE_ROOT/workspaces"
cd sdk/rust

echo "== the Rust client, on the socket"
GITD_SOCKET="$NATIVE_ROOT/gitd.sock" cargo test --locked --test gitd -- --ignored --nocapture

echo "== the Rust client, on the network"
GITD_URL="https://$ADDRESS" GITD_KEY_FILE="$NATIVE_ROOT/api.key" GITD_CA_FILE="$NATIVE_ROOT/cert.pem" \
  cargo test --locked --test gitd -- --ignored --nocapture
echo "PASSED"
