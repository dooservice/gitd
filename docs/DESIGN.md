# gitd — design

`gitd` manages the **local copies of git repositories** on one server: a cache of each
repository, the workspaces checked out from it, keeping them current, their submodules,
and the credentials they are read with. Repositories live on GitHub; `gitd` hosts none.

It runs on each customer's server, called only by the agent on that server, over a local
Unix socket.

---

## 1. Principles

1. **Run the official `git`.** Nothing of git is reimplemented: what the git binary does
   not do, `gitd` does not do. As in Gitaly.
2. **Nothing secret on disk.** Credentials arrive with each request, live in memory for as
   long as it lasts and reach git through its environment — never on a command line, never
   in `.git/config`.
3. **Nothing half-built is ever visible.** Whatever is created is built aside and moved into
   place with a `rename` once complete. A power cut leaves, at most, a temporary directory
   that the next start removes.
4. **Nothing is ever deleted from the shared cache.** Some workspace, or a rollback to an
   old commit, may still need an object after a force push on GitHub dropped it.
5. **Refuse rather than sink.** Every repository and every kind of operation has a
   concurrency limit with a queue; what does not fit waits, or is refused.

---

## 2. Concepts

### Repository
Identified by its **canonical HTTPS address** (`https://github.com/owner/name`), lowercased
and without `.git`, however it was written (`git@github.com:Owner/Name.git` is the same
one). The same rule as `dooservice_git.Repo`: an address that does not name a host, an
owner and a name is not a repository, which is also where `file://` and `ext::` stop.

### Pool (cache)
One **bare** repository per remote repository and per server, holding its objects:

```
<state>/pools/<host>/<owner>/<name>.git
```

- Created with `init --bare` and filled with `fetch --no-prune`, bringing **only what is
  wanted**: the tip of a branch or one pinned commit with `--depth 1`, or the whole
  history when a request asks for it. A submodule is its pinned commit and an added
  addon is the tip of its branch, as a server always fetched them; a project's own
  repository keeps its whole history, so `git log` and `git blame` in a console go back
  as far as they did. A rollback to a commit the pool lacks brings that one commit.
  git's own gc is switched off in it.
- After each fetch its `HEAD` follows the remote's, so a repository whose main branch
  changed hands the new one to new workspaces.
- It is **the only copy that talks to the network**: all of a repository's network traffic
  goes through its pool, once, however many environments use it. A pool synced in the last
  few seconds is taken as current.
- Objects nothing points at any more are kept in *cruft packs* and never pruned (§6).

### Workspace
A working tree for one environment, at the path its creator
asks for, always inside one of the roots allowed by the configuration:

```
/var/lib/dooservice/projects/<project>/<env>/addons            (the project's repository)
/var/lib/dooservice/projects/<project>/<env>/extra-addons/<x>  (the addons added to it)
```

- A **non-bare** repository whose `objects/info/alternates` points at the pool, with the
  pool's `shallow` file copied in: it holds no objects of its own and is created in
  seconds. It is built with `init` rather than `clone --shared`, because git ignores
  `--shared` for a shallow pool and copies every object.
- Its `origin` is the repository's real address, so `git status` or a console see what
  they always saw. But it **never fetches from the network**: `gitd` syncs the pool and
  then fetches the workspace from the pool, on this disk.
- Submodules are treated the same way: each has its own pool, and its git directory under
  `.git/modules/<name>` borrows from it, with its `.git` file and `core.worktree` written
  as relative paths so the workspace can be renamed into place. They are mounted by hand
  because `submodule update --reference` refuses a shallow pool. `submodule.<name>.url`
  points at the pool in `.git/config` only; the customer's `.gitmodules` is never touched.
- `gitd.url`, `gitd.pool` and `gitd.branch` in its `.git/config` say it is gitd's, of
  which repository, and which branch it follows while a rollback has it detached.

### Credential
What opens a repository. It arrives with each request, bound to a **URL prefix**
(`https://github.com/apiservicesac/odoo-pos`, or empty for "any repository"). Which one a
repository gets is decided by whoever sends the request, from what GitHub says about it:

| Repository | Credential | Transport |
|---|---|---|
| Public | The project's key (empty prefix) | SSH |
| Private, administered by the account | Its own deploy key | SSH |
| Private, the account is only a collaborator | The account's token | HTTPS |

- **SSH key**: the path of a key already on the server (the deploy keys the agent
  generates). Used through `GIT_SSH_COMMAND` with `IdentitiesOnly=yes`, and the host
  checked against GitHub's published keys, which are built into the binary.
- **HTTPS token** (the OAuth token of the connected account): handed to git as an
  `Authorization` header through `GIT_CONFIG_*`, scoped to that one repository. A
  repository it opens is read over HTTPS even when written as `git@`, so a `.gitmodules`
  line with `git@` is read without editing anything.

A repository is read with **the most specific credential that opens it, and with that one
only** — or with nothing when none does. Nothing is tried in turn: a refusal is the
answer, and it says which credential was refused.

---

## 3. Architecture

```
cmd/gitd/                 start-up and signals
internal/config/          configuration (TOML): roots, limits
internal/server/          ConnectRPC services on the socket, interceptor
internal/repo/            which repository an address names
internal/git/             the runner of the official git
internal/pool/            pools: create, sync, seed, probe, registry
internal/workspace/       workspaces: prepare, update, checkout, submodules, describe, adopt
internal/lock/            per-repository locks (memory + flock), concurrency queues
internal/housekeeping/    heuristic maintenance, retirement, start-up sweep
internal/credentials/     a request's credentials → git's environment
internal/progress/        git's `--progress` → stream messages
internal/metrics/         Prometheus metrics
proto/gitd/v1/            the contract
gen/go/                   generated code
```

It follows `dooprint`'s layout (`cmd/` + `internal/`). Logs are structured (`log/slog`) on
standard output, which systemd's journal collects. The agent installs and updates the
binary (§10), so `gitd` never updates itself.

### Transport
**ConnectRPC** on a **Unix socket** (`/run/dooservice/gitd.sock`, reachable only by the
agent's user). It speaks gRPC, gRPC-Web and Connect (HTTP/JSON) from the same `.proto`, so
it can be tried with `buf curl` or `curl`.

Long operations (preparing a 2 GB workspace) are **server-streaming**: they send progress
messages and end with their result. Cancelling the call cancels the git in flight. Every
call carries a **deadline**; one that brings none gets the configured one.

`ServerService.Info` gives the version of `gitd` and of git, so the agent knows whether it
may use it.

### The agent's client
The agent speaks the Connect protocol in JSON over `httpx`, which it already ships and
which talks to Unix sockets natively: a unary call is one POST, a server stream is a POST
whose answer is a sequence of length-prefixed JSON messages. That keeps the agent's binary
free of a gRPC stack, and the messages are plain `msgspec` structs mirroring the `.proto`.

---

## 4. The git runner

The only thing that starts `git`. Everything else goes through it.

- **Closed environment**, never the process's own: only `HOME`, `PATH`,
  `LD_LIBRARY_PATH` and `TZ` are inherited, plus:
  - `GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_SYSTEM=/dev/null`, `XDG_CONFIG_HOME=/dev/null`
  - `GIT_TERMINAL_PROMPT=0`, empty `GIT_ASKPASS`, `LANG=en_US.UTF-8`
  - the request's configuration in `GIT_CONFIG_COUNT/KEY/VALUE` (credentials)
- **Fixed options** the repository cannot change (`-c`): no hooks
  (`core.hooksPath=/dev/null`), no fsmonitor, an empty `credential.helper`, and only the
  `https` and `ssh` protocols (`protocol.allow=never`, `protocol.file.allow=never`,
  `protocol.ext.allow=never`; `file` only for the local pool, see §5).
- **Subcommand allowlist**, as Gitaly's `commandDescriptions`: `clone`, `fetch`,
  `ls-remote`, `rev-parse`, `checkout`, `submodule`, `config`, `symbolic-ref`, `ls-tree`,
  `status`, `count-objects`, `repack`, `pack-refs`, `commit-graph`, `fsck`, `version`,
  `init`, `push`, `check-ref-format`.
  Nothing else can run. Arguments are always built by `gitd`.
- **`--end-of-options`** before any argument that comes from outside (branch, URL,
  commit), so none of them can pass for an option.
- **Time limit** per operation and cancellation through the context; cancelling kills the
  whole process group (git starts children of its own).
- **CPU and memory limits** come from its systemd service.
- **Errors carry what git said**: the last useful line of its `stderr`
  (`remote: Repository not found.`), classified into a reason (§8).

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
   it, and its checkout cloned from that pool on this disk. What cannot be brought in comes
   back as **pending**, without failing the rest. Nested submodules are not brought in in
   v1.
5. `rename` the temporary directory to the destination. When the destination exists and
   is not a `gitd` workspace, it is refused: nothing that is not gitd's is overwritten.

### Update (`Update`)
Pool (network) → `fetch` of the workspace from the pool → `checkout --force` at the tip of
the branch → submodules. Returns the new commit and the pending submodules. With no branch,
the one the workspace follows (also after a rollback, which leaves it detached).

### Go back to a commit (`Checkout`)
When the commit is in the pool, `checkout --force --detach` and submodules; otherwise the
pool is synced first. This is what a rollback uses: since the pool never prunes, the commit
from before a deployment is still there even after the history was rewritten on GitHub.

### Submodules (`SyncSubmodules`)
All of them, or one. Reads the workspace's `.gitmodules` and brings each in from its pool.

### Describe (`Describe`)
Commit, branch, whether there are local changes, and the state of each submodule, read from
disk without the network.

### Probe a repository (`Probe`)
`ls-remote --symref` with nothing, then with each credential: whether it can be read, its
branches and its default branch. What the agent's `repo.probe` answers with.

### Publish (`Publish`)
What a console committed in a development environment, pushed to a **new branch** with
the account's token so a pull request can be opened from it. A submodule whose commit is
not in its pool was committed to there, so it goes first, to a branch of the same name in
its own repository. The commit is fetched into the pool and pushed **from the pool**: the
token is used where gitd owns the configuration, never in the copy a console could write,
so an `http.proxy` or an `insteadOf` left there cannot see where the token goes. Only a
token publishes — deploy keys are read only — and the push is never forced, and nothing
is pushed when the workspace's own repository brings no new commit. What comes back
carries each repository's new commit subjects. The agent then opens the pull requests,
with titles an AI agent in the console wrote or, when nobody did, made from those
subjects; `dooservice publish` in the console and the panel's button go the same way. The
orchestrator only decides who may, and only for development environments; nothing
reaches an environment until a pull request is merged.

### Remove (`Remove`)
Deletes a workspace. Its pool stays; when no workspace borrows from it for a configurable
time, maintenance retires it.

### Adopt (`Adopt`)
For the clones that already exist on servers: seeds the repository's pool from the clone
when there is none (a local copy with hard links, no network), links the clone to it with
`alternates` and repacks it so it stops holding what the pool holds. Before anything is
removed, `git fsck --connectivity-only`; if it fails, the link is undone and the clone is
left as it was. Its checked-out submodules are taken in the same way, each to its own pool.

---

## 6. Maintenance

Done by `gitd` on its own, in the background, with heuristics (as Gitaly's
`HeuristicalOptimizationStrategy`):

- **Pools:** `repack` with *cruft packs* that never expire (no object is ever lost) and
  bitmaps, only once enough loose objects or packs have piled up; `pack-refs` and
  `commit-graph` on every round.
- **Workspaces:** nothing of their own. git's automatic gc only touches their local
  objects; it does not take the pool's as theirs.
- **Retirement:** a pool no workspace uses (by its registry and by their `alternates`) for
  the configured time is deleted.
- **Never a blind `git gc`.**
- **At start-up:** the `*.gitd-tmp-*` directories are removed.
- **Before every write**, with the repository's lock held: only git's known `.lock` files
  (`index.lock`, `HEAD.lock`, `packed-refs.lock`, `config.lock`, `refs/**/*.lock`…) older
  than a margin. Never any `*.lock`.

---

## 7. Concurrency

- **One lock per repository** (pool) and one per workspace, in memory and backed by an
  `flock` file, so that a second process or a restart does not ignore them. Writes on one
  repository are **serialized** before git takes its own locks.
- **Concurrency queue** per kind of operation, with a maximum size and a maximum wait. What
  does not fit is refused with `RESOURCE_EXHAUSTED` and its reason.
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
| `GIT_FAILED` | Any other git failure | `INTERNAL` |

The agent turns the reason into a code the panel already knows how to say in the user's
language.

---

## 9. Observability

- **Metrics** (Prometheus, on a separate local address, off by default): duration of each
  operation with its code, failures by reason, size of each pool, and git processes alive.
- **Logs** per call, with the repository, the operation and what git said; never
  credentials.

---

## 10. Deployment on customers' servers

- The binary is released from this repository (a GitHub release per tag). The agent's
  release pins a `gitd` version, publishes that binary next to its own and names it, with
  its hash, in the **same signed manifest**. The agent's root updater installs it when its
  hash changes and restarts the service.
- systemd service `dooservice-gitd`, with the same user as the environments' files
  (uid 1500), `ProtectSystem=strict`, `PrivateTmp` and write access only to its state and
  its roots. Its configuration is written by the agent's `agent install`, from the agent's
  own data directory.
- **Transition:** the agent uses `gitd` when it answers and its version is compatible, and
  otherwise does what it does today. An agent setting forces one or the other. Existing
  clones are adopted (`Adopt`) the first time they are touched.
- **Consoles:** an operator console mounts the working copy read-write and runs git in it,
  so it also mounts the pools directory read-only at the same path. When the console is
  closed, the agent's working-copy guard removes the copy's link to its pool and writes it
  again from what the agent knows — the repository it says the copy is of, that
  repository's pool, and the pool's own `shallow` — never from what the console left. A
  submodule's link is kept only when it names a real pool, which the console could only
  read.
- **Health:** every heartbeat says whether gitd is installed, which version answers and
  the disk its pools take, and the server's page in the panel shows it. A gitd that does
  not answer is shown, not alarmed about: the server does its git itself meanwhile.

---

## 11. Out of v1

- **Embedded git** in the binary (as Gitaly), so as not to depend on the server's.
- **`sparse-checkout`** of only the modules each environment loads.
- **Nested submodules**, and SSH host keys of hosts other than GitHub.

---

## 12. Open decisions

1. **Credentials in shared pools:** a private repository's pool is synced with the
   credential of the request in flight. A server serves one customer, so a pool never mixes
   customers' credentials.
2. **Partial clone** (`blob:none`) in the pool was tried and set aside: a workspace linked
   through `alternates` cannot fetch from the network the objects its pool lacks, and its
   checkout fails. Shallow pools that bring only what is checked out give the same saving
   without that problem: `OCA/web` takes 16 MB for the tip of 18.0 against 226 MB whole.
3. **Retiring unused pools:** 30 days without any workspace, by default.
