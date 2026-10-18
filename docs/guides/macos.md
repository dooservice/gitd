# macOS

gitd runs on macOS for agents on a laptop. With no overlay there, it clones each template
whole with APFS's `clonefile(2)`: one call however many files, sharing the template's
blocks until written.

```sh
curl -fsSLo /usr/local/bin/gitd https://github.com/dooservice/gitd/releases/latest/download/gitd-darwin-arm64
chmod +x /usr/local/bin/gitd
mkdir -p ~/gitd/workspaces
cat > ~/gitd/gitd.toml <<TOML
socket = "$HOME/gitd/gitd.sock"
state = "$HOME/gitd/state"
roots = ["$HOME/gitd/workspaces"]
TOML
gitd serve --config ~/gitd/gitd.toml &
export GITD_SOCKET=~/gitd/gitd.sock
gitd prepare https://github.com/acme/web ~/gitd/workspaces/agent-1
```

`gitd info` says `workspaces made by reflink` when templates are cloned. Keep the state and
the roots on one APFS volume. The CI's macOS job measures it with
[`benchmarks/macos.sh`](../../benchmarks/macos.sh).
