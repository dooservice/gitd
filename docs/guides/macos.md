# macOS

gitd runs on macOS for agents on a laptop. With no overlay there, it clones each template
whole with APFS's `clonefile(2)`: one call however many files, sharing the template's
blocks until written.

```sh
curl -fsSLo /usr/local/bin/gitd https://github.com/dooservice/gitd/releases/latest/download/gitd-darwin-arm64
chmod +x /usr/local/bin/gitd
gitd up
gitd prepare https://github.com/acme/web ~/gitd/workspaces/agent-1
```

`gitd up` runs gitd for you as a launchd agent, `~/Library/LaunchAgents/dev.gitd.gitd.plist`:
started when you log in, restarted when it fails, logging to `~/Library/Logs/gitd.log`. It
writes your configuration unless you have one, and the command line finds gitd with no
`GITD_SOCKET`:

| | |
|---|---|
| Configuration | `~/Library/Application Support/gitd/gitd.toml` |
| State | `~/Library/Application Support/gitd/state` |
| Socket | `~/Library/Application Support/gitd/gitd.sock` |
| Workspaces | `~/gitd/workspaces` |

With the GitHub CLI logged in (`gh auth login`), gitd reads its token each time it starts
and never writes it down; otherwise `gitd connect github` signs you in. `gitd down` stops
gitd and keeps it from starting at login; the configuration, pools and workspaces stay.

Over SSH, with nobody logged in at the screen, launchd has no session to start gitd in, and
`gitd up` says how to run it in a terminal instead.

`gitd info` says `workspaces made by reflink` when templates are cloned. Keep the state and
the roots on one APFS volume. The CI's macOS job measures it with
[`benchmarks/macos.sh`](../../benchmarks/macos.sh).
