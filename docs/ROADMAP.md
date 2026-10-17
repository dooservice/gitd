# gitd — roadmap and work plan

**The aim:** any AI agent, in any sandbox, gets its own checkout of any repository in
milliseconds, takes almost no disk, and never sees a credential — on a server that may host
the agents of many customers at once.

This document says where gitd is, what each version brings, how and why it is built that
way, what could go wrong and how it is handled, the order the work is done in, and how
gitd reaches the people it is for. It is kept up to date as versions ship and as
measurements correct it.

---

## Principles every version keeps

1. **Secretless above all.** No credential ever reaches a workspace, a sandbox, an agent's
   context, an MCP tool's input or a command line. gitd reads and publishes with the
   credentials it keeps; pushing happens from the pool, never from a workspace.
2. **The contract only grows.** Fields and methods are added, never renamed or reused. A
   caller written for an older gitd — such as a platform that sends its own credentials
   and starts gitd with `-config` — keeps working unchanged.
3. **Automatic, not configured.** When there are several ways to do something (how to make a
   workspace, who mounts a layer), gitd picks the best the machine allows and falls back on
   its own. The default always works; configuration only narrows the choice.
4. **Only measured claims.** Every number in the README comes from a benchmark anyone can
   run again (`benchmarks/`, and the workflows that run them on GitHub's runners). When a
   measurement contradicts a claim or a plan, the claim or the plan changes — as "half a
   second whatever the repository" did when kubernetes measured 2.8 s, and as the strategy
   order of 0.7 did when the spike measured reflink and fuse-overlayfs.
5. **Run the official git.** gitd never reimplements git; what git does not do, gitd does
   not do.
6. **Small, tested, one change at a time.** One responsibility per file and package, tests
   with every change, one signed-off commit per change.
7. **Finish before opening.** A version is closed — benchmarked, documented, released —
   before the next one starts. Three half-done fronts are worth less than one done.

---

## Where gitd is: 0.6 (released)

| Capability | State |
|---|---|
| Pools fetched once, workspaces borrowing their objects | Done |
| Any git host (GitHub, GitLab with nested groups, Bitbucket, Gitea, Forgejo, Codeberg, self-hosted), HTTPS or SSH, any port | Done |
| Credentials kept by gitd (`[[credentials]]`: token file, environment variable, SSH key) | Done |
| Command line (`gitd prepare`, `publish`, …), on spf13/cobra | Done |
| MCP tools (`gitd mcp`), none of which takes a credential, enforced by a test | Done |
| Binaries and Docker image for amd64 and arm64; an example of sandboxes with no token | Done |
| Benchmark workflow | Done |
| Apache License 2.0 | Done |

**Measured** (kubernetes, ten agents, on a GitHub runner):

| History | 10 × git clone | 10 × gitd | |
|---|---|---|---|
| Tip only | 119.5 s, 4,190 MB, 10 fetches | 37.7 s, 3,786 MB, 1 fetch | 3.2× faster |
| Whole | 1,093 s, 17,103 MB, 10 fetches | 128.6 s, 5,132 MB, 1 fetch | 8.5× faster, a third of the disk |

**What that shows:** the network is solved — one fetch however many agents. But every
workspace still writes all its files: 2.8 s and some 370 MB per agent on kubernetes. That
is the cost the next version removes.

---

## 0.7 — copy-on-write workspaces, and what platforms need first

### Goal

A workspace of kubernetes in **milliseconds** and **about a megabyte** per agent, instead
of 2.8 s and 370 MB, with git behaving exactly as in a normal checkout; plus the first
things an agent platform asks for — clients in its own language, a record of what agents
published, and a rule on what they may name their branches.

**Done when:** the benchmark shows 100 agents on kubernetes in about a second in total, the
conformance suite passes on every strategy gitd picks automatically, the Python and
TypeScript clients are published, the security review of 0.7's new surface is done, and
callers written for 0.6 notice nothing.

### The idea, simply

Today each agent gets a full copy of the repository's files. Instead, gitd keeps **one
read-only checkout per commit** — the **template** — and gives each agent a **layer of its
own on top** that holds only what that agent changes. Think of transparent sheets on a
table: the table is the template, shared by everyone and never touched; each agent writes
on its own sheet. The agent sees both together as an ordinary directory.

### What the spike measured (task 0, done)

kubernetes (31,441 files, 369 MB checked out), 100 workspaces from one template, on a
GitHub runner (4 CPUs, git 2.55):

| Strategy | New workspace | Disk per workspace | First `git status` (default / tuned) | git behaves |
|---|---|---|---|---|
| copy (today) | 2.31 s | 374 MB | – | yes |
| **overlay (kernel)** | **0.004 s** (100 in 0.43 s) | **1.12 MB** | 0.62 s / 0.59 s | **yes** |
| fuse-overlayfs, unprivileged | 0.005 s | 1.00 MB | 7.27 s | **no**: writes inside `.git` refused |
| reflink (XFS) | 3.32 s | 16.33 MB | 3.96 s / 0.74 s | yes |

**What it decided:**

- **Kernel overlay is the strategy.** A thousand times faster and over three hundred times
  less disk than today, with status, diff, stash, commit and checkout behaving and the
  template untouched.
- **reflink is not a speed strategy.** Cloning file by file is slower than writing the files
  (3.3 s against 2.3 s); it only saves disk. It stays as an option for machines short of
  disk and without overlay, never chosen for speed.
- **fuse-overlayfs stays out of gitd:** run unprivileged outside a user namespace, it refuses
  writes inside `.git`, and git breaks. Inside a user namespace it may work, but a mount made
  there is seen only in that namespace, so gitd could never hand it to a sandbox: whatever
  the result, it could not join gitd's choice. It is a runtime's tool — rootless Podman can
  use it on the layers gitd hands over in 0.9 — and gitd does not test it.
- **The `git status` tuning (D4) matters for reflink** (3.96 s to 0.74 s) and is a small gain
  on overlay; it is applied to both.

### How it is built

**Templates.** For each repository and commit asked for, gitd builds once a full checkout,
with its `.git` borrowing from the pool and its index, exactly as a workspace is built
today. A template never changes. gitd records which workspaces use it and retires it, like
a pool, once none does.

**How a workspace is made, in the order gitd tries:**

| Strategy | When | New workspace (kubernetes) | Disk per agent |
|---|---|---|---|
| **overlay** | Someone may mount it (below) and the layers are on a real disk | milliseconds | what the agent changes |
| **copy** | Otherwise; always works | 2.3 s | the whole checkout |
| **reflink** | Only when configured (`strategy = "reflink"`) to save disk, on Btrfs, XFS, bcachefs or ZFS ≥ 2.2 | 3.3 s | almost none |

**Who mounts an overlay.** Linux lets only privileged processes mount one, and gitd runs
unprivileged on purpose. Nobody chooses this: gitd uses the first that exists.

1. **The sandbox's own runtime, when it is already allowed to.** Docker (a volume of
   `type=overlay`), Kubernetes (the CSI driver of 0.9), rootless Podman or bubblewrap inside
   a user namespace. gitd hands over the layers' paths; the runtime mounts them where the
   sandbox sees them.
2. **`gitd-mount`**, a tiny root service on a socket only gitd's user can reach, that does
   two things — mount an overlay workspace, unmount one — and only within directories its
   root-owned configuration fixes. gitd itself stays unprivileged.
3. **copy**, when neither exists.

**Where layers live.** On a real disk — a host directory or a volume — never inside a
container's own filesystem: an overlay cannot be stacked on another overlay, and a
container's filesystem is one. gitd checks at start, says so plainly, and falls back.

**git notices nothing.** With overlay or reflink the files are identical but their inode
numbers and change times differ from what the template's index recorded. Every such
workspace is configured with `core.checkStat=minimal` and `core.trustctime=false`, which
compare size and modification time only, and both are kept.

**What does not change.** The agent works with plain git; changes and commits land in its
own layer. `publish` fetches from the workspace's `.git` into the pool and pushes from
there, as today. `update` and `checkout` run plain git inside the workspace, which is
always correct; moving a clean workspace onto another template is a later optimisation.
`remove` unmounts and deletes the layer.

**Clients for platforms.** Python and TypeScript clients built from the contract with
buf, published to PyPI and npm, so an agent platform — most are written in one of the two —
calls gitd in a few lines instead of writing a client.

**Publishing under rules, on record.**

- **A branch prefix** (`[publish] branch_prefix = "agents/"`): an agent may publish only to
  branches under it. With publishing already limited to new branches and never forced,
  that keeps agents away from every branch people use.
- **An audit log**: every publish recorded — when, which workspace, which repository,
  branch and commit, the new commits' subjects, and, from 0.8, which tenant and token — in
  an append-only file and readable through the API, the command line and MCP. "Which agent
  published what, when, from where" is governance a company asks for.

### Decisions

| # | Decision | Why |
|---|---|---|
| D1 | A template per repository, commit and submodule choice, immutable | A layer's lower directory must never change under it |
| D2 | overlay, then copy, automatically; reflink only when configured to save disk | Principle 3, and the spike: reflink is slower than copy, fuse-overlayfs broke git |
| D3 | The runtime mounts when it can; `gitd-mount` otherwise; never gitd itself | gitd stays unprivileged; the privilege is confined to one tiny program |
| D3b | `gitd-mount` is a socket-activated service, not a setcap or setuid binary | Built that way after finding that gitd's unit keeps `NoNewPrivileges`, which voids file capabilities; a service also knows its caller by the kernel's credentials (`SO_PEERCRED`), not by anything it is told |
| D4 | `core.checkStat=minimal` and `core.trustctime=false` in copy-on-write workspaces | The first `git status` stays fast (3.96 s to 0.74 s with reflink) |
| D5 | A strategy is chosen automatically only if the conformance suite passes on it | git correctness is not negotiable — it is what kept fuse-overlayfs out of the spike's choice |
| D5b | fuse-overlayfs is not tested in a user namespace | A mount made in one is seen only there, so gitd could not hand it to a sandbox: the result could not change a decision. It is left to runtimes, on the layers of 0.9 |
| D6 | `update` and `checkout` stay plain git inside the workspace | Correct in every case |
| D7 | New contract fields only | Principle 2 |
| D7b | Handing a runtime the layers to mount itself moves to 0.9 | Docker needs none — gitd-mount mounts the workspace on the host, and the container binds it like any directory; the CSI driver is the first that needs layers, so it shapes that part of the contract |
| D8 | SDKs in 0.7, not 1.0 | Platforms are the users; a client in their language is the first thing they need, and generating it is cheap |
| D9 | Branch prefix and audit log in 0.7 | Small, and what makes a company trust agents to publish |

### Risks and how they are handled

| Risk | Handling |
|---|---|
| Overlay needs permission to mount | The runtime mounts (Docker, Kubernetes, rootless Podman, bubblewrap); else `gitd-mount`; else copy |
| `gitd-mount` becomes an attack surface | It answers only the configured user (by `SO_PEERCRED`), on a socket only that user can reach; mounts only an overlay of a template on a layer onto an empty target in a root, all that user's; pins every directory with `openat2(RESOLVE_NO_SYMLINKS)` and mounts and unmounts through the descriptor, so no swap in between redirects it; unmounts only overlays. Tests, a fuzz target, and the security review (task 10) |
| Overlay on overlay is refused by the kernel | Layers and templates on a real disk; checked at start (overlayfs's magic number), with a warning and a fallback |
| git is confused by inodes or change times | D4, and the conformance suite gating every strategy (D5) |
| A template retired while a workspace uses it | Workspaces are registered on their template; it is retired only when none is, after a grace period |
| Mounts do not survive a reboot | gitd remounts at start every overlay workspace its registry lists |
| A crash between making a layer and mounting it | Built aside and moved into place, as everything gitd builds; the start-up sweep removes what is half built |
| The audit log grows without end | Rotated by size and age, configurable |

### Tasks

| Task | What | Done when |
|---|---|---|
| **0.7 · 0 Spike** ✓ | Overlay, fuse-overlayfs, reflink and copy on kubernetes, 100 agents | Done: the table above |
| **0.7 · 1 Templates** ✓ | `internal/template`: build, register users, retire; housekeeping retires the unused | Done: built once even when asked for at once, shared, never rebuilt, retired only when unused and rested, swept after a crash; built by the workspace package, detached at its commit and tuned for copies |
| **0.7 · 2 Strategies** ✓ | One interface; copy (today's code moved behind it), overlay (external and helper), reflink (`FICLONE`); detection at start | Done: overlay and reflink makers, detection (a state on an overlay told apart), Prepare and Remove through them; end to end with the kernel's overlay, workspaces after the first in 26–32 ms. Whole-history and submodule workspaces are still written, until templates hold them |
| **0.7 · 3 `gitd-mount`** ✓ | The helper: a small root service with `CAP_SYS_ADMIN` only, socket-activated, with systemd units | Done: refuses any path outside its directories or through a symlink, any caller but gitd's user, any unmount of what is not an overlay; tests (root ones in CI) and a fuzz target; end to end, an unprivileged gitd made workspaces in 34–36 ms through it |
| **0.7 · 4 Contract** ✓ | `[workspaces] strategy`; `strategy` on `Prepare`; `strategy` and `layers` in `WorkspaceResult` and `Describe`; strategy, its reason and templates in `Info`; CLI flags and MCP output | Done: buf breaking finds nothing against 0.6.0, and the 0.6.0 command line works against it |
| **0.7 · 5 Lifecycle** ✓ | Unmount on remove; remount at start; templates and layers in the start-up sweep and in `Info` | Done: remounted at start and every round, never over files written while unmounted; half-built and orphaned layers swept, their templates let go; an agent's commit survives a reboot, end to end |
| **0.7 · 6 Conformance** ✓ | What agents do — status, diff, add, commit, stash, rebase, checkout, publish — on every strategy in CI | Done: copy, overlay and reflink pass, the other workspace sees nothing and the template never changes; CI runs it as root with an XFS disk. fuse-overlayfs is not tested: it could not join gitd's choice whatever the result (above) |
| **0.7 · 7 SDKs** ✓ | Python and TypeScript clients from the contract with buf; published; one example each | Done: `gitd-client` (TypeScript, generated by buf) and `gitd` (Python, which reads the contract at run time from JSON built by buf: no generated code); each with an example, tested end to end against gitd; the release publishes both by trusted publishing, with no token kept |
| **0.7 · 8 Publishing rules** | `[publish] branch_prefix`; the audit log with its API, CLI and MCP reads, and rotation | A branch outside the prefix is refused; every publish is in the log |
| **0.7 · 9 Threat model** | `docs/THREAT_MODEL.md`: what gitd protects, from whom, and how — credentials, roots, `gitd-mount`, MCP | Reviewed against the code |
| **0.7 · 10 Security review** | An outside review, even informal, of the threat model, `gitd-mount` and credential handling, before 0.8 opens a network service | Findings fixed or recorded |
| **0.7 · 11 Proof and docs** | Benchmark "100 agents on kubernetes"; the sandboxes example with overlay volumes; README and DESIGN | The README's numbers are the benchmark's |

---

## 0.8 — any sandbox with a network, and many tenants on one gitd

### Goal

Two things that grow from the same root, gitd's own tokens:

- **Any sandbox with a network.** MicroVMs (Firecracker: E2B, Fly and others), managed
  remote sandboxes (Modal, Daytona, Runloop, Codespaces) and CI runners elsewhere cannot
  mount gitd's directories, but can reach it over the network. They get the same promise:
  a fast checkout, and no credential of the git host.
- **Many tenants on one gitd.** A platform that hosts the agents of many customers runs one
  gitd for all of them, each customer isolated from the others — not one gitd per customer.

**Done when:** a sandbox on another machine clones from gitd with a token that opens one
repository of one tenant, pushes to its own branch, and gitd publishes that branch with
that tenant's credential; two tenants on one gitd cannot see each other's repositories,
workspaces, logs or tokens; and a gitd with no tenants configured behaves exactly as 0.7.

### How it is built: the endpoint

- **gitd serves its pools over git's smart HTTP**, by running the official `git
  http-backend` (principle 5), on a listening address of the configuration.
- **Tokens of gitd's own.** A short-lived token (default an hour, at most a day) is issued
  per sandbox through the API, the command line and an MCP tool. It opens one repository
  for reading and one **sandbox repository** for writing; it is never the git host's
  credential. Kept hashed, in memory; revoked on expiry or when its workspace goes.
- **Writes go to a sandbox repository, not the pool.** Each token has a bare repository of
  its own, borrowing the pool's objects, where the sandbox may push. Publishing takes a
  branch from there to the git host with gitd's credential, through the same code — and
  the same branch prefix and audit log — as `publish` from a workspace.
- **Fast clones of large repositories.** The endpoint allows partial clones
  (`--filter=blob:none`): files arrive as they are read.

### How it is built: tenants

A **tenant** is one customer of the platform. In gitd's configuration each has:

```toml
[[tenants]]
name = "acme"
api_key_file = "/run/credentials/gitd.service/acme"   # the key the platform calls with for acme
roots = ["/var/lib/gitd/tenants/acme"]                 # where acme's workspaces may be made
limits = { workspaces = 200, disk = "50GB", fetches = 2 }

  [[tenants.credentials]]
  prefix = "https://github.com/acme"
  token_file = "/run/credentials/gitd.service/acme-github"
```

- **Every call names its tenant** with that tenant's API key — a header on the socket, a
  flag or variable on the command line and MCP, or the endpoint token, which belongs to one
  tenant. A call without a valid key is refused; a key opens only its own tenant.
- **Nothing is shared between tenants.** Pools, templates, workspaces, credentials, tokens
  and audit entries are kept per tenant, under its own directory. A repository fetched with
  acme's credential is never readable by another tenant — not even public repositories share
  a pool at first: separate is simple and safe, and copy-on-write already keeps the disk
  small.
- **Limits per tenant**: workspaces, disk and concurrent fetches, so one customer cannot
  starve the others.
- **No tenants, no change.** Without `[[tenants]]`, gitd is one implicit tenant and behaves
  exactly as before — the configuration and the calls of 0.7 stay valid.

### Decisions

| # | Decision | Why |
|---|---|---|
| D10 | The official `git http-backend`, not a reimplementation | Principle 5 |
| D11 | A sandbox repository per token for writes | No ref rules to get wrong on a shared pool; publishing reuses `publish` |
| D12 | Tokens short-lived, scoped to one repository of one tenant, hashed, revocable | A leaked token opens little, for little time |
| D13 | Tenants in 0.8, on the same tokens | A multi-customer platform asks first; the tokens are half of it |
| D14 | Nothing shared between tenants, public pools included, at first | Isolation that cannot be got wrong; sharing public pools can come later, measured |
| D15 | No tenants configured means one implicit tenant | Principle 2: nothing changes for anyone who does not need tenants |
| D16 | The endpoint (0.8) before Kubernetes' CSI driver (0.9) | It covers more sandboxes — every microVM and remote one, CI — with less code |

### Risks and how they are handled

| Risk | Handling |
|---|---|
| A network service is exposed | Off unless configured; TLS with a given certificate, or a private network only; every request needs a token; reviewed (task 0.7 · 10) before it ships |
| A token or an API key leaks | Tokens: short life, one repository, revocable. Keys: per tenant, read from a credential file, hashed in memory, replaced without restart |
| One tenant starves the others | Limits per tenant on workspaces, disk and fetches; the queues already in gitd |
| A tenant reads another's objects | Pools, templates and workspaces per tenant; a conformance test tries every cross-tenant read and expects a refusal |
| Disk duplicated across tenants | Accepted at first; copy-on-write keeps workspaces small; shared public pools only if measured worth it |
| Pools are shallow by default | A sandbox gets the history its pool has; full-history pools serve full clones |

### Tasks

| Task | What | Done when |
|---|---|---|
| **0.8 · 0 Endpoint** | Smart HTTP over the pools with `git http-backend`; upload only | `git clone http://gitd/…` works with a token, fails without one |
| **0.8 · 1 Tokens** | Issue, check and revoke; an RPC, a CLI command and an MCP tool | Expired and revoked tokens refused; a token opens only its repository |
| **0.8 · 2 Sandbox repositories** | Receive into a repository per token; publish from it, with the prefix and the audit log | A push from a sandbox becomes a branch on the git host through gitd |
| **0.8 · 3 Tenants** | `[[tenants]]`: keys, roots, credentials, limits; every call scoped; per-tenant pools, templates, workspaces, tokens and audit | Isolation tests pass; no tenants configured behaves as 0.7 |
| **0.8 · 4 Network** | Listen address, TLS, limits | Tests with TLS and without; refused when unconfigured |
| **0.8 · 5 Examples** | E2B and Modal (or any microVM) cloning from gitd; a two-tenant setup | Each example runs end to end |

---

## 0.9 — Kubernetes: a CSI driver

### Goal

A pod asks for a workspace like any volume, and gets a copy-on-write checkout in
milliseconds, with no credential in the pod:

```yaml
volumes:
  - name: repo
    csi:
      driver: gitd.dev
      volumeAttributes: { repository: https://github.com/acme/web, branch: main, tenant: acme }
```

**Done when:** on a kind cluster in CI, a pod gets its workspace, an agent Job commits and
publishes, and the pod never holds a credential.

### How it is built

- **gitd runs on every node** (a DaemonSet), its pools and templates on the node's disk,
  its credentials and tenant keys from Kubernetes Secrets mounted into gitd only.
- **A CSI node plugin** (a DaemonSet too) implements ephemeral inline volumes: when a pod
  starts, it asks the gitd on its node for a workspace in external mode and mounts the
  overlay layers into the pod — CSI drivers are privileged by design, which is where the
  privilege to mount belongs. When the pod goes, it unmounts and gitd removes the layer.
- **A Helm chart** installs both.

### Decisions

| # | Decision | Why |
|---|---|---|
| D17 | Ephemeral inline CSI volumes, node plugin only | A workspace lives as long as its pod; no controller or claims to run |
| D18 | Credentials and tenant keys only in gitd's Secrets | Pods, and the agents in them, never hold one |

### Risks and how they are handled

| Risk | Handling |
|---|---|
| Each node fetches its own pools | Accepted: once per node, not per pod |
| A pod is rescheduled to another node | Its ephemeral volume is made again there, in milliseconds |
| Sandboxed runtimes (gVisor, Kata) | Tested in CI on both; Kata shares the volume through virtio-fs |
| The plugin's privilege | Confined to mounting gitd's layers, as `gitd-mount` is |

### Tasks

| Task | What | Done when |
|---|---|---|
| **0.9 · 0 Node plugin** | CSI NodePublish and NodeUnpublish for ephemeral volumes; the contract's way of handing the plugin a workspace's layers to mount itself (from 0.7, D7b) | A pod on kind gets a workspace |
| **0.9 · 1 Helm chart** | gitd and the plugin as DaemonSets, Secrets, configuration | `helm install` on kind and a real cluster |
| **0.9 · 2 Examples** | A pod and an agent Job that commits and publishes | Run end to end in CI |

---

## 1.0 — everywhere developers are

| Task | What | Done when |
|---|---|---|
| **1.0 · 0 macOS** | A darwin binary with a `clonefile` strategy (APFS's copy-on-write), for laptops | Workspaces on a Mac with almost no disk, measured |
| **1.0 · 1 Guides** | One per sandbox: Docker, Kubernetes, E2B, Modal, Claude Code and Codex with bubblewrap | Each guide runs as written |
| **1.0 · 2 Stability** | The contract declared stable; a deprecation policy | Documented |

---

## Sandboxes and how each gets a workspace

| Sandbox | Way | Version |
|---|---|---|
| Docker, Podman, containerd, gVisor | Local mount of a copy-on-write workspace | 0.7 |
| bubblewrap, nsjail, Landlock — how Claude Code and Codex CLI isolate commands on Linux | Local mount | 0.7 |
| Firecracker microVMs (E2B, Fly…) | Local git endpoint | 0.8 |
| Managed remote sandboxes (Modal, Daytona, Runloop, Codespaces) | Local git endpoint | 0.8 |
| CI runners | Local mount on the same host; the endpoint elsewhere | 0.7 / 0.8 |
| Kubernetes, GKE Sandbox (gVisor), Kata Containers | CSI driver | 0.9 |
| macOS laptops | `clonefile` | 1.0 |

---

## Launch: reaching the people gitd is for

The engineering is ahead of the audience: the repository has no stars yet. gitd launches
**when 0.7 ships**, with its headline number — *100 agents on kubernetes in about a
second, a megabyte each, and no token in any sandbox* — measured by a workflow anyone can
run.

| Where | What |
|---|---|
| Hacker News | A "Show HN": the problem, the benchmark, the demo |
| Claude Code, Codex and Cursor communities | `gitd mcp` for parallel agents, with a short demo |
| MCP directories | The official MCP registry, the awesome-mcp-servers lists, Smithery and Glama |
| A post and a demo | The why and the numbers; a terminal recording of ten agents and a published branch |
| Agent and sandbox platforms | Talk to two or three (E2B, Daytona, Modal, Runloop or similar): would they integrate it, what is missing — the answers shape 0.8 |
| GitHub | Discussions open; good first issues labelled; the roadmap linked from the README |

---

## Work plan

Versions are released (0.6.0, then 0.7.0 …); tasks are not. "0.7 · 3" is task 3 of version
0.7: it lands on `main` with its commits and tests, and the version is released once all
its tasks are done.

In order; each step is one or more commits, tested, and each version ends with its
benchmark, its docs and a release (principle 7).

| Step | Version | Work | Depends on |
|---|---|---|---|
| 1 | 0.6.0 ✓ (released) | Released: binaries for amd64 and arm64, the image | — |
| 2 | 0.7 · 0 ✓ | Spike run, table recorded, strategy order decided | 1 |
| 3 | 0.7 · 1 ✓ | Templates | 2 |
| 4 | 0.7 · 2 ✓ | Strategies and detection: copy moved behind the interface first, then overlay, then reflink | 3 |
| 5 | 0.7 · 3 ✓ | `gitd-mount` | 4 |
| 6 | 0.7 · 4 ✓ | Contract, CLI and MCP | 4 |
| 7 | 0.7 · 5 ✓ | Lifecycle | 5, 6 |
| 8 | 0.7 · 6 ✓ | Conformance suite gating the strategies | 4 |
| 9 | 0.7 · 7 ✓ | Python and TypeScript SDKs | 6 |
| 10 | 0.7 · 8 | Branch prefix and audit log | 6 |
| 11 | 0.7 · 9–10 | Threat model and security review | 5, 10 |
| 12 | 0.7 · 11 | Benchmark "100 agents", example, docs; **release 0.7.0 and launch** | 7–11 |
| 13 | 0.8 · 0–5 | Endpoint, tokens, sandbox repositories, tenants, network, examples; release 0.8.0 | 12 |
| 14 | 0.9 · 0–2 | CSI node plugin, Helm chart, examples; release 0.9.0 | 13 |
| 15 | 1.0 · 0–2 | macOS, guides, stability; release 1.0.0 | 13, 14 |

**How the work is done.** Heavy runs (benchmarks, spikes, kind clusters) go on GitHub's
runners: gitd's repository is public, so they cost nothing, and anyone can run them again.
Every change has its tests, every commit is signed off, and README numbers come only from
those runs.

---

## Decisions already taken

| Decision | Why |
|---|---|
| gitd is general and carries nothing of any one platform | Anyone can use it; a platform's paths and policies are its configuration |
| Focus: instant, secretless checkouts for agent sandboxes | Where gitd differs from `git worktree`, clones and agent tools |
| Credentials kept by gitd, by URL prefix, naming where the secret is | Callers, agents and sandboxes never hold one |
| MCP tools never take a credential, enforced by a test | The agent deals in paths, commits and branches only |
| The command line on spf13/cobra, one file per command | Maintained for over a decade; single responsibility |
| Apache License 2.0 instead of the AGPL-3.0 | Platforms can build on gitd; patents granted |
| The name stays `gitd` | Renaming moves the repository and buys nothing now |
| The README states only measured numbers | Credibility; corrected twice already by measurement |
| Kernel overlay is the copy-on-write strategy; reflink only to save disk; fuse-overlayfs out until proven | The spike: 2 ms and 1.1 MB, git behaving; reflink slower than copy; fuse-overlayfs broke git |
| SDKs, branch prefix and audit log move into 0.7 | What platforms need first, and cheap |
| Multi-tenancy in 0.8, isolated per tenant, nothing shared at first | A multi-customer platform asks first; isolation must not be gettable wrong |
| A security review before 0.8's network service | The more surface gitd exposes, the more "secretless" must be proved |
| Launch with 0.7, not before | The headline number is what gets attention |
| The local git endpoint (0.8) before the CSI driver (0.9) | It reaches more sandboxes with less code |
