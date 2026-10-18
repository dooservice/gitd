# Claude Code and Codex CLI

Both run an agent on your machine and, on Linux, isolate the commands it runs with
bubblewrap (Claude Code's sandbox) or Landlock (Codex's `workspace-write`): commands write
only in the working directory. Give each agent a workspace of gitd as that directory, and
gitd's MCP tools to publish:

```sh
gitd prepare https://github.com/acme/web ~/agents/fix-login
cd ~/agents/fix-login
claude          # or: codex --sandbox workspace-write
```

With gitd's MCP server the agent prepares, reviews and publishes workspaces itself, and
never handles a credential:

```json
{ "mcpServers": { "gitd": { "command": "gitd", "args": ["mcp"] } } }
```

in Claude Code's `.mcp.json`, or for Codex in `~/.codex/config.toml`:

```toml
[mcp_servers.gitd]
command = "gitd"
args = ["mcp"]
```

The tools are `prepare_workspace`, `list_changes`, `show_diff`, `publish_branch` and the
others in the [README](../../README.md#for-ai-agents-mcp).

On Ubuntu 24.04 and later, unprivileged user namespaces, which bubblewrap needs, are kept
behind AppArmor: allow them with `sysctl kernel.apparmor_restrict_unprivileged_userns=0`, or
an AppArmor profile for `bwrap`.

[`examples/bubblewrap`](../../examples/bubblewrap) shows the isolation part on its own: a
command in bubblewrap commits in its workspace, cannot write outside it, and does not see
gitd's socket. CI runs it on every push. What it cannot show is the agents themselves, which
need their accounts.
