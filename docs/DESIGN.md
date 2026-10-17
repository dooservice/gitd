# gitd — design

`gitd` gives **instant, secretless git checkouts to agent sandboxes**: AI agents, CI jobs
or preview environments get a workspace of a repository in the seconds it takes to write
its files, with no download, work in it with plain git, and have their commits published
as a branch, without ever holding the credential. To do so it keeps, on one server, a
cache of each repository (its pool), the workspaces that borrow from it, their submodules,
and the credentials they are read and published with. Repositories live on any git host —
GitHub, GitLab (with its nested groups), Bitbucket, Gitea, Forgejo, Codeberg or a
self-hosted one; `gitd` hosts none.

It runs on one server and answers on a Unix socket: to a program through its API, to a
person through its command line, and to an AI agent through MCP. Whatever is particular to
whoever calls it — paths, user, which credential opens what — is configuration and
requests, never code.

---

## 1. Principles

1. **Run the official `git`.** Nothing of git is reimplemented: what the git binary does
   not do, `gitd` does not do. As in Gitaly.
2. **Nothing secret where the work is done.** Credentials are kept by gitd (read at start
   from a file, best a systemd credential in memory, or the environment) or arrive with a
   request, live in its memory and reach git through its environment — never on a command
   line, never in `.git/config`, never in a workspace. Whoever works in a workspace, an
   agent or a sandbox, never sees one: gitd reads and pushes for it.
3. **Nothing half-built is ever visible.** Whatever is created is built aside and moved into
   place with a `rename` once complete. A power cut leaves, at most, a temporary directory
   that the next start removes.
4. **Nothing is ever deleted from the shared cache.** Some workspace, or a rollback to an
   old commit, may still need an object after a force push on the host dropped it.
5. **Refuse rather than sink.** Every repository and every kind of operation has a
   concurrency limit with a queue; what does not fit waits, or is refused.

---

## 2. Concepts

### Repository
Identified by its **canonical HTTPS address** (`https://<host>/<owner>/<name>`), lowercased,
without `.git` and without a port, however it was written: `https://…`, `user@host:…` or
`ssh://…` (`git@github.com:Owner/Name.git` is the same one). Between the owner and the name may sit the groups of a host that nests them
(`https://gitlab.com/acme/tools/web`), up to twenty. An address that does not name a host,
an owner and a name is not a repository, which is also where `file://` and `ext::` stop.

Ports and SSH users are the host's, not the repository's:
`https://git.example.com:3000/acme/web`, `ssh://git@git.example.com:2222/acme/web.git` and
`git@git.example.com:acme/web` are all `https://git.example.com/acme/web`, one pool and one
set of credentials, however a project and its submodules write it. How a host is reached when not over HTTPS on 443 and SSH as
`git` on 22 is configuration:

```toml
[hosts."git.example.com"]
https_port = 3000
ssh_port = 2222
ssh_user = "git"
```

### Pool (cache)
One **bare** repository per remote repository and per server, holding its objects:

```
<state>/pools/<host>/<owner>/[<group>/…]<name>.git
```

- Created with `init --bare` and filled with `fetch --no-prune`, bringing **only what is
  wanted**: the tip of a branch or one pinned commit with `--depth 1`, or the whole
  history when a request asks for it (`full_history`), so that `git log` and `git blame`
  in the workspace go back as far as in a clone. A submodule is always its pinned commit
  only. A rollback to a commit the pool lacks brings that one commit.
  git's own gc is switched off in it.
- After each fetch its `HEAD` follows the remote's, so a repository whose main branch
  changed hands the new one to new workspaces.
- It is **the only copy that talks to the network**: all of a repository's network traffic
  goes through its pool, once, however many workspaces use it. A branch synced in the last
  ten seconds is taken as current.
- Objects nothing points at any more are kept in *cruft packs* and never pruned (§6).

### Workspace
A working tree, at the path its creator asks for, always inside one of the roots allowed
by the configuration (`roots`), for instance:

```
/var/lib/gitd/workspaces/<project>/<env>/app
```

- A **non-bare** repository whose `objects/info/alternates` points at the pool, with the
  pool's `shallow` file copied in: it holds no objects of its own and is created in
  seconds. It is built with `init` rather than `clone --shared`, because git ignores
  `--shared` for a shallow pool and copies every object.
- Its `origin` is the repository's real address (on its host's `https_port`), so whoever
  runs git in it sees what a clone shows. But it **never fetches from the network**: `gitd` syncs the pool and
  then fetches the workspace from the pool, on this disk.
- Submodules are treated the same way: each has its own pool, and its git directory under
  `.git/modules/<name>` borrows from it, with its `.git` file and `core.worktree` written
  as relative paths so the workspace can be renamed into place. They are mounted by hand
  because `submodule update --reference` refuses a shallow pool. `submodule.<name>.url`
  points at the pool in `.git/config` only; the repository's `.gitmodules` is never touched.
  A submodule whose name or path in `.gitmodules` would leave the workspace (`..`, an
  absolute path, a `.git` component) is never checked out: it comes back as pending, with
  `PATH_REFUSED`.
- `gitd.url`, `gitd.pool`, `gitd.branch` and `gitd.history` in its `.git/config` say it is
  gitd's, of which repository, which branch it follows while a rollback has it detached,
  and whether it keeps the whole history.

### Credential
What opens a repository, bound to a **URL prefix** (`https://github.com/acme/web`, or empty
for "any repository"). Two kinds:

- **Kept by gitd**, in `[[credentials]]` of its configuration, which names where each
  secret is — `token_file`, `token_env` or `ssh_key` — never the secret. They are read once
  at start, and one that cannot be read stops it. This is what lets an agent, a sandbox or
  the command line ask for a workspace or a branch holding nothing.
- **Sent with a request**, when its caller decides which opens what from what the host
  says about it (a deploy key for one repository, an account's token for the rest of its
  owner's, a key with an empty prefix for anything public).

Both kinds are candidates, and a request's wins over a kept one of the same prefix.

- **SSH key**: the path of a key already on the server. Used through `GIT_SSH_COMMAND`
  with `IdentitiesOnly=yes`, and the host checked against the keys GitHub, GitLab,
  Bitbucket and Codeberg publish, which are built into the binary, plus those of the `known_hosts` files the configuration names, for a
  self-hosted GitLab, Gitea or Forgejo. Nothing is trusted on first use. The address is
  `git@<host>:<path>.git`, or `ssh://<user>@<host>:<port>/<path>.git` for a host whose
  `[hosts."<host>"]` gives another `ssh_port` or `ssh_user`; its key is then looked up as
  `[<host>]:<port>`.
- **HTTPS token** (an OAuth or access token): handed to git as an `Authorization: Basic`
  header through `GIT_CONFIG_*`, scoped to that one repository's address (on its host's
  `https_port`). A repository it opens is read over HTTPS even when written as `git@` or
  `ssh://`, so a `.gitmodules` line written so is read without editing anything. The user
  name sent with it is the credential's (`https_user`, `token_user`), or when empty the one
  the host expects: `oauth2` on gitlab.com, `x-token-auth` on
  bitbucket.org, `x-access-token` anywhere else (GitHub; Gitea and Forgejo take any).

A repository is read with **the most specific credential that opens it, and with that one
only** — or with nothing when none does. A prefix is compared without its port and in any
of the address forms. Nothing is tried in turn: a refusal is the answer, and it says which
credential was refused.

---

## 3. Architecture

```
cmd/gitd/                 start-up and signals
internal/cli/             the command line (spf13/cobra), one file per command
internal/daemon/          serving: the configuration read and every part put together
internal/client/          a client of the socket, for the command line and MCP
internal/mcpserver/       MCP tools over stdio, none of which takes a credential
internal/config/          configuration (TOML): socket, roots, limits, hosts, known hosts, credentials
internal/server/          ConnectRPC services on the socket, deadlines, error codes
internal/repo/            which repository an address names, and how its host is reached
internal/git/             the runner of the official git
internal/pool/            pools: create, sync, seed, probe, registry
internal/workspace/       workspaces: prepare, update, checkout, submodules, describe, adopt
internal/lock/            per-repository locks (memory + flock), concurrency queues
internal/housekeeping/    heuristic maintenance, retirement, start-up sweep
internal/credentials/     kept and requested credentials → the address and git's environment
internal/progress/        git's `--progress` → stream messages
internal/metrics/         Prometheus metrics
proto/gitd/v1/            the contract
gen/go/                   generated code
```

The layout is Go's usual `cmd/` + `internal/`. Logs are structured (`log/slog`) on
standard output, which systemd's journal collects. Whoever deploys it installs and updates
the binary (§10); `gitd` never updates itself.

### Transport
**ConnectRPC** on a **Unix socket** (`/run/gitd/gitd.sock` unless `socket` in the
configuration or `--socket` says otherwise; reachable only by the user that runs gitd). It
speaks gRPC, gRPC-Web and Connect (HTTP/JSON) from the same `.proto`, so it can be tried
with `buf curl` or `curl`, and any language with a Connect or gRPC client can call it.

Long operations (preparing a 2 GB workspace) are **server-streaming**: they send progress
messages and end with their result. Cancelling the call cancels the git in flight and
answers `CANCELED`. Every call carries a **deadline**; one that brings none gets the
configured one (`limits.operation`).

`ServerService.Info` gives the version of `gitd` and of git, so a client knows whether it
may use it, and how many pools there are and the disk they take. `ServerService.Retire`
retires now the pools nobody used for a given time (at least a day).

A client needs no gRPC stack: in the Connect protocol a unary call is one POST of JSON,
and a server stream is a POST whose answer is a sequence of length-prefixed JSON messages.

### Command line and MCP
The same binary is its own client: `gitd` alone (or `gitd serve`) serves, and `gitd
prepare`, `update`, `checkout`, `submodules`, `describe`, `publish`, `remove`, `adopt`,
`probe`, `info` and `retire` call the gitd that serves, with progress on a terminal and
`--json` for scripts. A command brings a credential only from an environment variable or a
key path, never on its own command line. The single-dash flags of units written before
the command line (`-config`, `-socket`, `-version`) still read.

`gitd mcp` answers the Model Context Protocol over stdio with seven tools —
`prepare_workspace`, `update_workspace`, `checkout_commit`, `describe_workspace`,
`publish_branch`, `remove_workspace`, `probe_repository` — each saying whether it only
reads or destroys. **No tool takes a credential**, and a test keeps it so: an agent deals
in paths, commits and branches, and gitd in secrets.

---

## 4. The git runner

The only thing that starts `git`. Everything else goes through it.

- **Closed environment**, never the process's own: only `HOME`, `PATH`,
  `LD_LIBRARY_PATH` and `TZ` are inherited, plus:
  - `GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_SYSTEM=/dev/null`, `GIT_CONFIG_NOSYSTEM=1`,
    `XDG_CONFIG_HOME=/dev/null`
  - `GIT_TERMINAL_PROMPT=0`, empty `GIT_ASKPASS` and `SSH_ASKPASS`, `LANG` and `LC_ALL` at
    `en_US.UTF-8`
  - the request's configuration in `GIT_CONFIG_COUNT/KEY/VALUE` (credentials)
- **Fixed options** the repository cannot change (`-c`): no hooks
  (`core.hooksPath=/dev/null`), no fsmonitor, an empty `credential.helper` and
  `core.askPass`, and only the `https` and `ssh` protocols (`protocol.allow=never`, `protocol.file.allow=never`,
  `protocol.ext.allow=never`; `file` only for the local pool, see §5).
- **Subcommand allowlist**, as Gitaly's `commandDescriptions`: `clone`, `fetch`,
  `ls-remote`, `rev-parse`, `checkout`, `submodule`, `config`, `symbolic-ref`, `ls-tree`,
  `status`, `log`, `count-objects`, `repack`, `pack-refs`, `commit-graph`, `fsck`,
  `version`, `init`, `push`, `update-ref`, `check-ref-format`.
  Nothing else can run. Arguments are always built by `gitd`.
- **`--end-of-options`** before any argument that comes from outside (branch, URL,
  commit), so none of them can pass for an option.
- **Time limit** per operation and cancellation through the context; cancelling kills the
  whole process group (git starts children of its own).
- **CPU and memory limits**, when wanted, are set in its systemd service.
- **Errors carry what git said**: the last useful line of its `stderr`
  (`remote: Repository not found.`), classified into a reason (§8) from what each host
  says (GitHub, GitLab, Bitbucket, Gitea). A credential written in an address is left out
  of every message.

---

## 5. Operations

### Prepare a workspace (`Prepare`)
1. Lock the workspace.
2. Pool: create it if missing, otherwise sync it (network, with the credential).
3. Build the workspace at `<destination>.gitd-tmp-<id>`: `git init`, `alternates` and
   `shallow` from the pool, a fetch from the pool on this disk (nothing travels: every
   object is already the pool's), `origin` at the real address, and a checkout of the
   requested branch or commit.
4. Submodules (when asked): each from its own pool, synced with the credential that opens
   it, and checked out from that pool on this disk. What cannot be brought in comes back as
   **pending**, without failing the rest. Nested submodules are not brought in in v1.
5. `rename` the temporary directory to the destination. When the destination exists and
   is not a `gitd` workspace, it is refused: nothing that is not gitd's is overwritten.

### Update (`Update`)
Pool (network) → `fetch` of the workspace from the pool → `checkout --force` at the tip of
the branch → submodules. Returns the new commit and the pending submodules. With no branch,
the one the workspace follows (also after a rollback, which leaves it detached). A
workspace that keeps the whole history keeps fetching it.

### Go back to a commit (`Checkout`)
The commit is required. When it is in the pool, `checkout --force --detach` and
submodules; otherwise the pool fetches that one commit first. This is what a rollback uses: since the pool never prunes, the commit
from before a deployment is still there even after the history was rewritten on the host.

### Submodules (`SyncSubmodules`)
All of them, or one. Reads the workspace's `.gitmodules` and brings each in from its pool.

### Describe (`Describe`)
Commit, branch, whether there are local changes, and the state of each submodule, read from
disk without the network.

### Probe a repository (`Probe`)
`ls-remote --symref` with the credential that opens the repository, or with nothing:
whether it can be read (public, reachable with that credential, or denied and why), its
branches with the commit each points at, and its default branch.

### Publish (`Publish`)
What was committed in a workspace, pushed to a **new branch** with a token so a pull (or
merge) request can be opened from it. A submodule whose commit is not in its pool was
committed to there, so it goes first, to a branch of the same name in its own repository.
The commit is fetched into the pool and pushed **from the pool**: the token is used where
gitd owns the configuration, never in the copy somebody else could write, so an
`http.proxy` or an `insteadOf` left there cannot see where the token goes. Only a token
publishes — deploy keys are read only — the push is never forced, and nothing is pushed
when the workspace's own repository brings no new commit (`NOTHING_NEW`). What comes back
carries each repository's new commit subjects, for the client to title the requests it
opens; gitd opens none.

### Remove (`Remove`)
Deletes a workspace. Its pool stays; when no workspace borrows from it for a configurable
time, maintenance retires it.

### Adopt (`Adopt`)
For clones that already exist: seeds the repository's pool from the clone when there is
none (a local copy with hard links, no network), links the clone to it with
`alternates` and repacks it so it stops holding what the pool holds. Before anything is
removed, `git fsck --connectivity-only`; if it fails, the link is undone and the clone is
left as it was. Its checked-out submodules are taken in the same way, each to its own pool;
one that cannot be is left as it was, and logged. A workspace gitd made is adopted already.

---

## 6. Maintenance

Done by `gitd` on its own, in the background, with heuristics (as Gitaly's
`HeuristicalOptimizationStrategy`):

- **Pools:** `repack` with *cruft packs* that never expire (no object is ever lost) and
  bitmaps, only once enough loose objects or packs have piled up; `pack-refs` and
  `commit-graph` on every round.
- **Workspaces:** nothing of their own. git's automatic gc only touches their local
  objects; it does not take the pool's as theirs.
- **Retirement:** a pool no workspace uses (by its registry and by their `alternates`, the
  submodules' included) for `housekeeping.retire_after` is deleted.
- **Never a blind `git gc`.**
- **At start-up:** what a crash left half built (`*.gitd-tmp-*`) is removed, under the
  pools, inside them, and under the roots down to `housekeeping.sweep_depth`.
- **Before every write**, with the repository's lock held: only git's known `.lock` files
  (`index.lock`, `HEAD.lock`, `packed-refs.lock`, `config.lock`, `shallow.lock`,
  `refs/**/*.lock`…) older than `housekeeping.stale_lock`. Never any `*.lock`.

---

## 7. Concurrency

- **One lock per repository** (pool) and one per workspace, in memory and backed by an
  `flock` file, so that a second process or a restart does not ignore them. Writes on one
  repository are **serialized** before git takes its own locks.
- **Concurrency queue** per kind of operation (fetches, writes), with a maximum size and a
  maximum wait. What does not fit is refused with `BUSY` (`RESOURCE_EXHAUSTED`); a caller
  that gives up while waiting gets `CANCELED`, never `BUSY`.
- **Reads** (`Describe`, `Probe`) do not wait for writes on other repositories.

---

## 8. Errors

Connect codes with a typed detail (`GitFailure`) saying what happened and what git said:

| Reason | When | Code |
|---|---|---|
| `AUTH_REQUIRED` | The repository asks for a credential and none was given | `UNAUTHENTICATED` |
| `AUTH_REFUSED` | The credential given does not open it | `PERMISSION_DENIED` |
| `NOT_FOUND` | The repository, branch or commit does not exist | `NOT_FOUND` |
| `BAD_ADDRESS` | The address names no accepted repository | `INVALID_ARGUMENT` |
| `PATH_REFUSED` | The destination is outside the roots or not gitd's | `PERMISSION_DENIED` |
| `BUSY` | The queue is full | `RESOURCE_EXHAUSTED` |
| `TIMEOUT` | git did not finish in time | `DEADLINE_EXCEEDED` |
| `CORRUPT` | `fsck` found missing objects | `DATA_LOSS` |
| `NOTHING_NEW` | `Publish` found no new commit to push | `FAILED_PRECONDITION` |
| `GIT_FAILED` | Any other git failure | `INTERNAL` |

A call cancelled or out of time without git failing answers `CANCELED` or
`DEADLINE_EXCEEDED`. The reason, the repository and what git said also travel as the
`Gitd-Reason`, `Gitd-Repository` and `Gitd-Said` headers, for a client without protobuf.
A client tells its users what happened from the reason, never by parsing the message.

---

## 9. Observability

- **Metrics** (Prometheus, on a separate local address, off by default):
  `gitd_operation_seconds` by procedure and code, `gitd_failures_total` by reason,
  `gitd_pool_bytes` by repository, and `gitd_git_processes`.
- **Logs** per call, with the repository, the operation and what git said; never
  credentials.

---

## 10. Deployment

- The binary is released from this repository: `gitd-linux-amd64` and `gitd-linux-arm64`
  with their checksums, a GitHub release per tag, and the image `ghcr.io/dooservice/gitd`
  for both platforms. The image serves by default and is the command line too.
- `packaging/gitd.service`: user `gitd`, socket `/run/gitd/gitd.sock`, configuration
  `/etc/gitd/gitd.toml`, `ProtectSystem=strict`, `PrivateTmp` and write access only to its
  state. Write access to roots outside `/var/lib/gitd` is added with `ReadWritePaths=` in
  a drop-in, and the user should own the workspaces, as whoever reads them does.
- `packaging/gitd.toml` shows every key with its default. A token gitd keeps is best handed
  to it with `LoadCredential=`, which puts it in memory at
  `/run/credentials/gitd.service/<name>` for `token_file` to name.
- A program that ships gitd with it pins a version, installs the binary, writes its own
  service and configuration, and restarts it on an update.
- A workspace mounted somewhere else where git runs (a container, a sandbox) also needs the
  pools directory mounted, read only, at the same path, since its `alternates` name it.
  `examples/sandboxes` does so with Docker Compose: each sandbox mounts only its own
  workspace and the pools, and holds no credential.
- An AI agent reaches gitd through `gitd mcp` on its side, which calls the socket; only
  that process needs to reach the socket.

---

## 11. Out of v1

- **Embedded git** in the binary (as Gitaly), so as not to depend on the server's.
- **`sparse-checkout`** of only the modules each environment loads.
- **Nested submodules**.

---

## 12. Open decisions

1. **Credentials in shared pools:** a private repository's pool is synced with the
   credential that opens it, kept or sent, and any workspace on the server borrows from it.
   Until 0.8, one gitd is meant for one tenant; tenants who must not read each other's
   repositories get a gitd each. 0.8 brings tenants to one gitd, each with its own pools,
   templates, workspaces, credentials and limits ([ROADMAP](ROADMAP.md)). Likewise, whoever can reach the socket can ask for any repository gitd
   holds a credential for, so the socket is reachable only by gitd's own user and the
   programs acting for that tenant.
2. **Partial clone** (`blob:none`) in the pool was tried and set aside: a workspace linked
   through `alternates` cannot fetch from the network the objects its pool lacks, and its
   checkout fails. Shallow pools that bring only what is checked out give the same saving
   without that problem: `OCA/web` takes 16 MB for the tip of 18.0 against 226 MB whole.
3. **Retiring unused pools:** 30 days without any workspace, by default.
