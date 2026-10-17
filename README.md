# gitd

**Instant, secretless git checkouts for agent sandboxes.** Your agents work in their own
copy and publish a branch; they never see your token.

![Without gitd, three clones fetch and store every object three times; with gitd, one pool fetches once and the workspaces borrow its objects](docs/gitd.svg)

Give ten AI agents, CI jobs or preview environments the same repository and you usually get
ten full clones, ten fetches over the network, and a token inside every sandbox. gitd
fetches each repository once into a **pool** and makes **workspaces** that borrow its
objects: a new workspace needs no download, holds no copy of the history, and holds no
credential. When an agent is done it asks gitd to **publish** its commits as a new
branch, and gitd pushes from the pool with the token it keeps.

- **Secretless.** Credentials live in gitd, never in a workspace, a sandbox, an agent's
  context or a command line. Pushing happens from the pool, so a hostile `insteadOf` or
  `http.proxy` left in a workspace never sees where the token goes.
- **Fast.** The repository is fetched once; every workspace after that only writes its
  files: half a second for a small repository, under three seconds for kubernetes (see the
  [benchmark](#benchmark)), against twelve for a clone.
- **Made for sandboxes.** A workspace mounts into a container next to the pools, read only:
  each sandbox sees its own checkout and nothing else.
- **Any git host.** GitHub, GitLab (nested groups included), Bitbucket, Gitea, Forgejo,
  Codeberg or your own, over HTTPS or SSH, on any port, submodules included.
- **For agents.** `gitd mcp` offers it as Model Context Protocol tools, none of which takes a
  credential.

## Quick start

With Docker, two sandboxes sharing one repository and no token in either:

```sh
cd examples/sandboxes
./demo.sh                                   # or the steps below

docker compose up -d gitd
docker compose exec gitd gitd prepare https://github.com/OCA/web /var/lib/gitd/workspaces/agent-1
docker compose exec gitd gitd prepare https://github.com/OCA/web /var/lib/gitd/workspaces/agent-2
docker compose run --rm agent-1 sh -c 'echo hi > NOTE.md && git add NOTE.md && git commit -m "agent-1 was here"'
docker compose exec gitd gitd publish /var/lib/gitd/workspaces/agent-1 agent-1/note
```

To publish, give gitd a credential in `gitd.toml` (see [Credentials](#credentials)).

On a server, without Docker:

```sh
curl -fsSLo /usr/local/bin/gitd https://github.com/dooservice/gitd/releases/latest/download/gitd-linux-amd64
chmod +x /usr/local/bin/gitd
gitd serve --config /etc/gitd/gitd.toml &   # packaging/gitd.service runs it under systemd
gitd prepare https://github.com/OCA/web /var/lib/gitd/workspaces/web
```

## For AI agents (MCP)

`gitd mcp` answers the Model Context Protocol over stdio. Add it to your agent, for
instance in Claude Code's `.mcp.json` or Cursor's `mcp.json`:

```json
{
  "mcpServers": {
    "gitd": { "command": "gitd", "args": ["mcp"], "env": { "GITD_SOCKET": "/run/gitd/gitd.sock" } }
  }
}
```

| Tool | What it does |
|---|---|
| `prepare_workspace` | A workspace of a repository at a path under gitd's roots, at a branch or a commit |
| `update_workspace` | To the tip of its branch |
| `checkout_commit` | To a commit, as a rollback does |
| `describe_workspace` | Its repository, commit, branch, uncommitted changes and submodules |
| `publish_branch` | Its commits pushed to a new branch, for a pull request |
| `remove_workspace` | Deleted; its pool stays |
| `probe_repository` | Whether it can be read, its default branch and branches |
| `read_audit_log` | The publishes done or refused, newest first |

No tool takes a credential, and each says whether it only reads or destroys.

## Command line

`gitd` alone serves; every other command calls the gitd that serves.

```sh
gitd prepare URL PATH [--branch B | --commit C] [--submodules] [--full-history]
gitd update PATH [--branch B]        gitd checkout PATH COMMIT
gitd describe PATH                   gitd publish PATH BRANCH
gitd submodules PATH                 gitd remove PATH
gitd probe URL                       gitd adopt PATH
gitd info                            gitd retire [--unused 720h]
gitd audit [--since 24h] [--repository URL] [--limit N]
gitd mcp
```

Every command takes `--socket` (default `$GITD_SOCKET`, then `/run/gitd/gitd.sock`) and
`--json`. Progress shows on a terminal only.

## Credentials

gitd keeps them, so callers never have to. Each names where its secret is, never the
secret itself:

```toml
[[credentials]]
prefix = "https://github.com/your-org"
token_file = "/run/credentials/gitd.service/github"   # LoadCredential= in gitd.service: in memory

[[credentials]]
prefix = "https://gitlab.com/your-group"
token_env = "GITLAB_TOKEN"

[[credentials]]
prefix = ""                                            # any repository
ssh_key = "/etc/gitd/keys/id_ed25519"
```

A repository is read with the most specific credential that opens it, and only that one. A
token reaches git as an HTTP header in its environment, scoped to that one repository; a
key, through `GIT_SSH_COMMAND`, with hosts checked against known keys (GitHub, GitLab,
Bitbucket and Codeberg are built in). A token's prefix must name its host, or gitd does not
start: with none it would go to any host a submodule names. A caller may still send a
credential with a request, which wins over a kept one of the same prefix.

## Publishing rules

Agents publish only new branches, never forced. A prefix keeps them off the branches people
use, and every publish, done or refused, is on the record:

```toml
[publish]
branch_prefix = "agents/"      # "main" or "release/1.2" is refused: BRANCH_REFUSED
```

```sh
$ gitd audit --since 24h
2026-10-17 07:38:36  agents/x  https://github.com/octocat/hello-world  /var/lib/gitd/workspaces/agent-1
  refused, AUTH_REQUIRED: publishing takes a token, and none was given
2026-10-17 07:38:36  master  https://github.com/octocat/hello-world  /var/lib/gitd/workspaces/agent-1
  refused, BRANCH_REFUSED: "master" does not start with "agents/": gitd publishes only to branches that do
```

The log is `<state>/audit/publishes.jsonl`, one JSON entry per line, rotated at 16 MiB and
kept for a year (`[audit] rotate_bytes`, `keep`); the API, the clients and MCP read it too.

## Benchmark

Ten agents on [kubernetes](https://github.com/kubernetes/kubernetes), each getting its own
copy, on a GitHub runner:

| History | How | Time | Disk | Network |
|---|---|---|---|---|
| Tip only | 10 × `git clone --depth 1` | 119.5 s | 4,190 MB | 10 fetches |
| Tip only | 10 × `gitd prepare` | **37.7 s** (first 12.3 s, then 2.8 s each) | 3,786 MB | **1 fetch** |
| Whole | 10 × `git clone` | 1,093 s | 17,103 MB | 10 fetches |
| Whole | 10 × `gitd prepare --full-history` | **128.6 s** (first 99.5 s, then 3.2 s each) | **5,132 MB** | **1 fetch** |

With the whole history, gitd is 8.5× faster and takes a third of the disk; with the tip
only, 3.2× faster, and most of the disk is each copy's own files. Those files are what the
next version stops writing: in its first measurements, a kubernetes workspace made by
kernel overlay takes 2 ms and 1.1 MB (see the [roadmap](docs/ROADMAP.md)).

Run it yourself: `docker build -t gitd:bench . && benchmarks/agents.sh REPO N`, or the
*benchmark* workflow of this repository.

## How it works

A pool is a bare repository fetched with `--no-prune`, never garbage collected, so even a
commit lost to a force push is still there for a rollback. A workspace is a plain
repository whose `objects/info/alternates` points at the pool: it has its own refs, index
and config, any branch, and can be moved or mounted elsewhere. gitd runs the official `git`
with a closed environment, no hooks and only HTTPS and SSH, and builds everything aside
before moving it into place. The design is in [`docs/DESIGN.md`](docs/DESIGN.md), and where
gitd is going — copy-on-write workspaces, any sandbox, Kubernetes — in
[`docs/ROADMAP.md`](docs/ROADMAP.md).

Unlike `git worktree`, workspaces share no refs or config with each other, can check out
the same branch twice, and survive being mounted into a container.

## Configure

Every key has a default; [`packaging/gitd.toml`](packaging/gitd.toml) shows them all.

| Key | Default | What it is |
|---|---|---|
| `socket` | `/run/gitd/gitd.sock` | The Unix socket gitd answers on |
| `state` | `/var/lib/gitd` | Pools, lock files and the SSH host keys |
| `roots` | `["/var/lib/gitd/workspaces"]` | The only directories a workspace may be made in |
| `git` | `git` | The git binary |
| `metrics` | empty (off) | Where Prometheus metrics are served, e.g. `127.0.0.1:9419` |
| `known_hosts` | `[]` | Files of SSH host keys trusted besides the built-in ones |
| `[[credentials]]` | none | Credentials gitd keeps, by URL prefix |
| `[publish]` | `branch_prefix = ""` (any new branch) | What every published branch must start with |
| `[audit]` | rotate at 16 MiB, keep a year | How the audit log of every publish is kept |
| `[hosts."<host>"]` | — | `https_port`, `ssh_port`, `ssh_user` of a host not on the usual ones |
| `[limits]` | 4 fetches, 8 writes, 32 queued, 5m wait, 30m per call | How much runs at once and for how long |
| `[housekeeping]` | every 6h, retire after 30 days | Pool maintenance and retirement |

A host on other ports says so, and every form of its addresses is then one repository:

```toml
[hosts."git.example.com"]
https_port = 3000
ssh_port = 2222       # its key goes in known_hosts as [git.example.com]:2222 ssh-ed25519 …
```

## API

gitd speaks ConnectRPC (gRPC, gRPC-Web and Connect over HTTP) on its Unix socket; the
contract is [`proto/gitd/v1/gitd.proto`](proto/gitd/v1/gitd.proto). Any language with a
Connect or gRPC client can call it, and `buf curl` can too. There are clients for
[Python](sdk/python) (`pip install gitd`) and [TypeScript](sdk/typescript)
(`npm install gitd-client`).

## Install

- **Binary:** `gitd-linux-amd64` or `gitd-linux-arm64` from the
  [releases](https://github.com/dooservice/gitd/releases), with its checksum.
- **Docker:** `ghcr.io/dooservice/gitd`, for amd64 and arm64.
- **systemd:** [`packaging/gitd.service`](packaging/gitd.service).
- **Copy-on-write workspaces:** gitd makes a workspace by mounting an overlay on a shared
  template whenever it can, and writes its files otherwise. It runs unprivileged, so the
  mount is done by `gitd-mount`, a tiny root service on a socket only gitd's user reaches:
  install `gitd-mount-linux-*` as `/usr/local/bin/gitd-mount`, put
  [`packaging/gitd-mount.conf`](packaging/gitd-mount.conf) in `/etc/gitd/` (root's,
  matching gitd's state and roots), and enable
  [`gitd-mount.socket`](packaging/gitd-mount.socket). gitd says at start which way it makes
  workspaces, and why.
- **From source:** `make build` (Go, the version in `go.mod`); `make test`, `make lint`;
  `make generate` regenerates `gen/` from the `.proto` with `buf`.

Pushing a tag such as `0.6.0` releases the binaries, the image and both clients.

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md). What gitd protects, from whom, and what it leaves to
you is in [`docs/THREAT_MODEL.md`](docs/THREAT_MODEL.md). To report a vulnerability, do not
open an issue: follow [SECURITY.md](SECURITY.md).

## License

[Apache License 2.0](LICENSE).
