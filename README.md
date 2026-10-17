# gitd

**Instant, secretless git checkouts for agent sandboxes.** Your agents work in their own
copy and publish a branch; they never see your token.

**[gitd.dev](https://gitd.dev)** · [Get started](#quick-start) · [Benchmark](#benchmark) ·
[Threat model](docs/THREAT_MODEL.md) · [Roadmap](docs/ROADMAP.md)

![Without gitd, three sandboxes each clone the repository and hold a token; with gitd, one pool and one template are shared, each sandbox's workspace is a thin copy-on-write layer, and gitd keeps the token and publishes the branch](docs/gitd.svg)

Give a hundred AI agents, CI jobs or preview environments the same repository and you
usually get a hundred clones, a hundred fetches over the network, and a token inside every
sandbox. gitd fetches each repository once into a **pool**, checks each commit out once
into a **template**, and makes every **workspace** a copy-on-write layer on it: a new
workspace needs no download, writes no files until its agent changes them, and holds no
credential. When an agent is done it asks gitd to **publish** its commits as a new branch,
and gitd pushes from the pool with the token it keeps.

- **Secretless.** Credentials live in gitd, never in a workspace, a sandbox, an agent's
  context or a command line. Pushing happens from the pool, so a hostile `insteadOf` or
  `http.proxy` left in a workspace never sees where the token goes.
- **Fast and small.** For a hundred agents on kubernetes, each workspace after the first
  took **23 ms and 4.3 MB**, against 9.8 s and 418 MB for a shallow clone: all hundred in
  14.5 s and 850 MB instead of 16 minutes and 41.9 GB, with one fetch instead of a hundred
  (see the [benchmark](#benchmark)).
- **Made for sandboxes.** A workspace mounts into a container with its repository's pool,
  read only: each sandbox sees its own checkout and nothing else.
- **Any git host.** GitHub, GitLab (nested groups included), Bitbucket, Gitea, Forgejo,
  Codeberg or your own, over HTTPS or SSH, on any port, submodules included.
- **For agents.** `gitd mcp` offers it as Model Context Protocol tools, none of which takes a
  credential.

## Quick start

With Docker on Linux, two sandboxes with copy-on-write workspaces of one repository, gitd
unprivileged beside its mount helper, and no token in either sandbox:

```sh
cd examples/sandboxes
./demo.sh                                   # or the steps below; ./demo.sh down removes it all

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
gitd publish PATH BRANCH [--requested-by LOGIN] [--pull-request] [--title T] [--draft]
gitd connect github | github-app --name N [--org O]
gitd connections list | remove ID | add-token PREFIX --token-env VAR | add-github-app
gitd publishing show | set PREFIX [--preset recommended] … | remove PREFIX
gitd mcp
```

Every command takes `--socket` (default `$GITD_SOCKET`, then `/run/gitd/gitd.sock`) and
`--json`. Progress shows on a terminal only.

## Credentials

gitd keeps them, so callers never have to, and a platform manages them from its own pages
through the API (`ConnectionService` in both clients) or the command line, with no file to
edit and no restart. None of this is offered to agents through MCP.

**GitHub, with no token made by hand.** The organisation creates its own GitHub App in one
click, and gitd keeps its key; pull requests then come from the App's bot, and each token
gitd makes from it opens one repository for an hour, for what the operation needs:

```sh
$ gitd connect github-app --org acme --name acme-agents
Open http://127.0.0.1:41703/ and press "Create GitHub App"
app-3f9a1c2b7d01  github_app  https://github.com/acme  as acme-agents[bot]
Install it on the repositories it may reach: https://github.com/apps/acme-agents/installations/new
```

A platform does the same through `BeginGitHubApp` and `CompleteGitHubApp`, with GitHub
sending the person back to its own page. One person can instead sign in with a code
(`gitd connect github`, GitHub's device flow, once gitd's public App is registered), and
pull requests appear as them; a platform that already has an App registers it
(`gitd connections add-github-app`). `gitd connections list` shows them all, never a
secret.

**Any host, or as configuration as code.** A token is added the same way (`gitd connections
add-token PREFIX --token-env VAR`), or declared in the file, naming where its secret is,
never the secret itself:

```toml
[[credentials]]
prefix = "https://github.com/your-org"
token_file = "/run/credentials/gitd.service/github"   # LoadCredential= in gitd.service: in memory

[[credentials]]
prefix = "https://gitlab.com/your-group"
token_env = "GITLAB_TOKEN"

[[credentials]]
prefix = "https://github.com/your-org/tools"
github_app = { app_id = 123456, private_key_file = "/run/credentials/gitd.service/github-app.pem" }

[[credentials]]
prefix = ""                                            # any repository
ssh_key = "/etc/gitd/keys/id_ed25519"
```

A repository is read with the most specific credential that opens it, and only that one. A
token reaches git as an HTTP header in its environment, scoped to that one repository; a
key, through `GIT_SSH_COMMAND`, with hosts checked against known keys (GitHub, GitLab,
Bitbucket and Codeberg are built in). A token's prefix must name its host, or it is
refused: with none it would go to any host a submodule names. The secrets of connections
made through the API are kept under `<state>/connections`, readable by gitd's user only,
and encrypted when `secrets_key_file` names a key. A caller may still send a credential
with a request, which wins over a kept one of the same prefix.

## Publishing rules

Agents publish only new branches, never forced. A platform sets, per repository prefix,
what a publish may contain, through the API (`PublishingService`) or the command line, and
gitd checks it on the commits to push — commit by commit, in the pool — before anything is
pushed:

```sh
$ gitd publishing set https://github.com/acme --preset recommended --branch-prefix agents/ \
    --pull-requests always --draft --label agent --review-requester
```

The `recommended` preset protects `.github/workflows/**`, `.gitlab-ci.yml`, `CODEOWNERS`
and `.gitmodules`, keeps tests from being deleted or renamed away, and allows at most 200
files changed. A merge counts only what it brings itself, so merging what people changed on
`main` is not the agent's change. Every setting whose prefix opens a repository applies,
and a narrower one never loosens a broader one. The same settings can be declared in the
file (`[[publish.settings]]`, and `[publish] branch_prefix` for every repository).

When the settings say so, or a publish asks, gitd opens the pull request on GitHub with
the credential that pushed: from the App's bot, the person who asked for the work
(`--requested-by`) asked to review it, and its body saying what gitd checked — the commits,
the files and lines they change, and the rules they passed. If the pull request cannot be
opened, the branch stays published and the answer says why.

Every publish, done or refused, and every change to connections and settings, is on the
record:

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

A hundred agents on [kubernetes](https://github.com/kubernetes/kubernetes), each getting
its own copy, on a GitHub runner (4 CPUs):

| How | Time | Disk | Network |
|---|---|---|---|
| 100 × `git clone --depth 1` | 980 s, 9.8 s each | 41,870 MB | 100 fetches |
| 100 × `gitd prepare` | **14.5 s**: the first 12.2 s, then **23 ms** each | **850 MB**: 45 MB pool, 373 MB template, **4.3 MB** per workspace | **1 fetch** |

gitd is 67× faster and takes 49× less disk. Made 8 at a time, 99 workspaces took 2.0 s.
git saw all hundred as clean checkouts, a first `git status` taking 0.24 s; an agent's
commit stayed in its workspace; removing all hundred took 1.1 s. A workspace's 4.3 MB is
mostly git's index, which each one keeps of its own.

gitd ran as root in the benchmark, mounting the overlays itself; deployed unprivileged, it
asks `gitd-mount` over a socket, which measured 34–36 ms a workspace. With the whole history
(`--full-history`) a workspace is written rather than layered: ten of them took 128.6 s and
5,132 MB, against 1,093 s and 17,103 MB for ten full clones.

Run it yourself: `docker build -t gitd:bench . && sudo benchmarks/hundred.sh REPO N`, or the
*hundred agents* workflow of this repository.

## How it works

A pool is a bare repository fetched with `--no-prune`, never garbage collected, so even a
commit lost to a force push is still there for a rollback. A workspace is a plain
repository whose `objects/info/alternates` points at the pool: it has its own refs, index
and config, any branch, and can be moved or mounted elsewhere. gitd runs the official `git`
with a closed environment, no hooks and only HTTPS and SSH, and builds everything aside
before moving it into place. The design is in [`docs/DESIGN.md`](docs/DESIGN.md), and where
gitd is going — a GitHub App, publishing policies, changes and diffs for platforms, any
sandbox, Kubernetes — in [`docs/ROADMAP.md`](docs/ROADMAP.md).

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
| `[[credentials]]` | none | Credentials gitd keeps, by URL prefix: a token, an SSH key or a GitHub App |
| `secrets_key_file` | empty | The key that encrypts the secrets of connections made through the API |
| `[github]` | gitd's public App | The App people sign in through with `gitd connect github` |
| `[publish]` | `branch_prefix = ""` (any new branch) | What every published branch must start with |
| `[[publish.settings]]` | none | Rules and pull requests per repository prefix, as the API sets them |
| `[audit]` | rotate at 16 MiB, keep a year | How the audit log of every publish and change is kept |
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
  mount is done by `gitd-mount`, a tiny root service holding only the capabilities mounting
  an overlay takes, on a socket only gitd's user reaches:
  install `gitd-mount-linux-*` as `/usr/local/bin/gitd-mount`, put
  [`packaging/gitd-mount.conf`](packaging/gitd-mount.conf) in `/etc/gitd/` (root's,
  matching gitd's state and roots), and enable
  [`gitd-mount.socket`](packaging/gitd-mount.socket). gitd says at start which way it makes
  workspaces, and why. Keep gitd's state and roots on a `nosuid,nodev` filesystem, and read
  [the threat model](docs/THREAT_MODEL.md) before giving workspaces to sandboxes.
- **From source:** `make build` (Go, the version in `go.mod`); `make test`, `make lint`;
  `make generate` regenerates `gen/` from the `.proto` with `buf`.

Pushing a tag such as `0.7.0` releases the binaries, the image and both clients.

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md). What gitd protects, from whom, and what it leaves to
you is in [`docs/THREAT_MODEL.md`](docs/THREAT_MODEL.md). To report a vulnerability, do not
open an issue: follow [SECURITY.md](SECURITY.md).

## License

[Apache License 2.0](LICENSE).
