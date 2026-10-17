# gitd — roadmap and work plan

**The aim:** any AI agent, in any sandbox, gets its own checkout of any repository in
milliseconds, takes almost no disk, and never sees a credential.

This document says where gitd is, what each version brings, how and why it is built that
way, what could go wrong and how it is handled, and the order the work is done in. It is
kept up to date as versions ship.

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
   measurement contradicts a claim, the claim changes — as "half a second whatever the
   repository" did once kubernetes measured 2.8 s.
5. **Run the official git.** gitd never reimplements git; what git does not do, gitd does
   not do.
6. **Small, tested, one change at a time.** One responsibility per file and package, tests
   with every change, one signed-off commit per change.

---

## Where gitd is: 0.6

| Capability | State |
|---|---|
| Pools fetched once, workspaces borrowing their objects | Done |
| Any git host (GitHub, GitLab with nested groups, Bitbucket, Gitea, Forgejo, Codeberg, self-hosted), HTTPS or SSH, any port | Done |
| Credentials kept by gitd (`[[credentials]]`: token file, environment variable, SSH key) | Done |
| Command line (`gitd prepare`, `publish`, …), on spf13/cobra | Done |
| MCP tools (`gitd mcp`), none of which takes a credential, enforced by a test | Done |
| Docker image for amd64 and arm64, and an example of sandboxes with no token | Done |
| Benchmark workflow | Done |
| Apache License 2.0 | Done |

**Measured** (kubernetes, ten agents, tip only, on a GitHub runner): ten clones take 119.5 s
and ten fetches; ten gitd workspaces take 37.7 s and one fetch — the first 12.3 s, each
after it 2.8 s. Disk: 4,190 MB against 3,786 MB.

**What that shows:** the network is solved (one fetch), and gitd is 3.2× faster. But every
workspace still writes all its files — 2.8 s and some 370 MB per agent on kubernetes — and
the disk saving with tip-only pools is small. Removing that cost is the next version.

---

## 0.7 — copy-on-write workspaces

### Goal

A workspace of kubernetes in **milliseconds** and **a few megabytes** per agent, instead of
2.8 s and 370 MB, on any Linux machine, with git behaving exactly as in a normal checkout.

**Done when:** the benchmark shows 100 agents on kubernetes in about a second in total with
kernel overlay, the conformance suite passes on every strategy gitd picks automatically,
and dooservice-style callers notice nothing.

### The idea, simply

Today each agent gets a full copy of the repository's files. Instead, gitd keeps **one
read-only checkout per commit** — the **template** — and gives each agent a **layer of its
own on top** that holds only what that agent changes. Think of transparent sheets on a
table: the table is the template, shared by everyone and never touched; each agent writes
on its own sheet. The agent sees both together as an ordinary directory.

### How it is built

**Templates.** For each repository and commit asked for, gitd builds once a full checkout,
with its `.git` borrowing from the pool and its index, exactly as a workspace is built
today. A template never changes. gitd records which workspaces use it and retires it, like
a pool, once none does.

**Strategies.** From a template, a workspace is made by the best of these the machine
allows. gitd picks; nobody has to:

| Strategy | How | New workspace | Disk per agent | Needs |
|---|---|---|---|---|
| **overlay** | Kernel overlayfs: template below, read only; an empty layer per agent above | milliseconds | only what the agent changes | someone allowed to mount (below) |
| **reflink** | The template copied with block cloning: files share disk blocks until written | about 1 s on kubernetes | almost none | Btrfs, XFS, bcachefs or ZFS ≥ 2.2 |
| **fuse-overlayfs** | The same layering as overlay, done without privilege | milliseconds | only what the agent changes | FUSE; reads are slower than the kernel's |
| **copy** | The files written, as today | 2.8 s on kubernetes | the whole checkout | nothing: always works |

The order gitd tries them in — overlay, reflink, fuse-overlayfs, copy — is the expected one;
the spike (0.7.0) decides it with measurements, especially between reflink and
fuse-overlayfs, which trade creation time against read speed.

**Who mounts an overlay.** Linux lets only privileged processes mount one, and gitd runs
unprivileged on purpose. This is not a choice anyone makes: gitd uses the first that
exists.

1. **The sandbox's own runtime, when it is already allowed to.** Docker (a volume of
   `type=overlay`), Kubernetes (the CSI driver of 0.9), rootless Podman or bubblewrap inside
   a user namespace (kernel ≥ 5.11). gitd only hands over the layers' paths; the runtime
   mounts them where the sandbox sees them.
2. **`gitd-mount`**, a tiny helper that holds the one permission to mount and unmount, and
   only layers inside gitd's roots — like `fusermount` for FUSE. gitd itself stays
   unprivileged.
3. **fuse-overlayfs**, which needs no permission at all.
4. **copy**, when none of the above exists.

**Where layers live.** On a real disk — a host directory or a volume — never inside a
container's own filesystem, because an overlay cannot be stacked on another overlay, and a
container's filesystem is one. gitd checks at start, says so plainly, and falls back.

**git notices nothing.** With overlay or reflink the files are identical, but their inode
numbers and change times differ from what the template's index recorded, so git would
re-read the whole tree on its first `status` (seconds on kubernetes). Every copy-on-write
workspace is configured with `core.checkStat=minimal` and `core.trustctime=false`, which
compare size and modification time only, and both are kept. The spike measures the first
`status` with and without this.

**What does not change.** The agent works with plain git; changes and commits land in its
own layer. `publish` fetches from the workspace's `.git` into the pool and pushes from
there, as today. `update` and `checkout` run plain git inside the workspace, which is
always correct (the layer grows by the difference); moving a clean workspace onto another
template is a later optimisation. `remove` unmounts and deletes the layer.

### What the spike measured (0.7.0)

kubernetes (31,441 files, 369 MB checked out), 100 workspaces from one template, on a
GitHub runner (4 CPUs, git 2.55):

| Strategy | New workspace | Disk per workspace | First `git status` (default / tuned) | git behaves |
|---|---|---|---|---|
| copy (today) | 2.24 s | 374 MB | – | yes |
| overlay (kernel) | **0.002 s** (100 in 0.23 s) | **1.12 MB** | 0.77 s / 0.62 s | yes |
| reflink (XFS) | being measured | | | |
| fuse-overlayfs | being measured | | | |

Kernel overlay meets the goal: a thousand times faster and over three hundred times less
disk than today, with git's status, diff, stash, commit and checkout behaving and the
template untouched. The first `git status` costs well under a second even untuned, so D4 is
a small gain on overlay rather than a necessity; it stays for reflink, where inode numbers
change for every file.

### Decisions

| # | Decision | Why |
|---|---|---|
| D1 | A template per repository, commit and submodule choice, immutable | A layer's lower directory must never change under it; immutability makes sharing safe |
| D2 | Strategies chosen automatically, falling back down to copy | Principle 3: it always works, at the best speed the machine allows |
| D3 | The runtime mounts when it can; `gitd-mount` otherwise; never gitd itself | gitd stays unprivileged; the privilege is confined to one tiny, audited program |
| D4 | `core.checkStat=minimal` and `core.trustctime=false` in copy-on-write workspaces | Without them, the first `git status` re-reads the whole tree |
| D5 | A strategy is chosen automatically only if the conformance suite passes on it | git correctness is not negotiable |
| D6 | `update` and `checkout` stay plain git inside the workspace | Correct in every case; faster paths come later and only when safe |
| D7 | New contract fields only: `strategy` on `Prepare`, `strategy` and `layers` in the answer, `[workspaces] strategy` in the configuration | Principle 2 |

### Risks and how they are handled

| Risk | Handling |
|---|---|
| Overlay needs permission to mount | The runtime mounts (Docker, Kubernetes, rootless Podman, bubblewrap); else `gitd-mount`; else fuse-overlayfs; else copy |
| `gitd-mount` becomes an attack surface | It mounts and unmounts only overlays whose every path resolves inside gitd's roots (no symlinks followed), only for gitd's user, and nothing else; its own tests and a fuzzer on its input |
| Overlay on overlay is refused by the kernel | Layers and templates live on a real disk; gitd checks the filesystem at start (overlayfs's magic number) and warns and falls back |
| The filesystem has no reflink (ext4) | gitd detects it and uses overlay, fuse-overlayfs or copy; the docs recommend XFS or Btrfs for `/var/lib/gitd` for the best results |
| git is confused by inodes or change times | D4, and the conformance suite (D5) on every strategy in CI |
| A template retired while a workspace still uses it | Workspaces are registered on their template; it is retired only when none is, after a grace period |
| Mounts do not survive a reboot | gitd remounts, at start, every overlay workspace its registry lists |
| A crash between creating a layer and mounting it | Built aside and moved into place, as everything gitd builds; the start-up sweep removes what is half built |
| fuse-overlayfs is slow to read | Chosen only after the faster strategies; its cost is measured and documented |

### Tasks

| Task | What | Done when |
|---|---|---|
| **0.7.0 Spike** | `benchmarks/cow-spike.sh` and its workflow: 100 workspaces of kubernetes by kernel overlay, fuse-overlayfs, reflink on XFS, and copy | The table gives creation time, disk per agent and first `git status` for each, and git status, diff, stash, commit and checkout behave on each with the template untouched |
| **0.7.1 Templates** | `internal/template`: build, register users, retire; housekeeping retires the unused | Unit tests: built once and shared, never changed, retired only when unused |
| **0.7.2 Strategies** | One interface; copy (today's code moved behind it), reflink (`FICLONE`, falling back per file), overlay (external, helper, fuse-overlayfs); detection at start | Each strategy has its tests; detection is right on ext4, XFS and inside a container |
| **0.7.3 `gitd-mount`** | The helper: a separate small binary, setcap or setuid, with a systemd example | Refuses any path outside the roots or through a symlink; tests and a fuzz target |
| **0.7.4 Contract** | `[workspaces] strategy`; `strategy` on `Prepare`; `strategy` and `layers` in `WorkspaceResult` and `Describe`; CLI flags and MCP output | buf lint and breaking-change check pass; an old client still works |
| **0.7.5 Lifecycle** | Unmount on remove; remount at start; templates and layers in the start-up sweep and in `Info` | A restarted gitd serves every overlay workspace it had |
| **0.7.6 Conformance** | A suite of what agents do — status, diff, add, commit, stash, rebase, checkout, submodules, publish — run on every strategy in CI | It gates D5 |
| **0.7.7 Proof and docs** | Benchmark "100 agents on kubernetes"; the sandboxes example with overlay volumes; README and DESIGN | The README's numbers are the benchmark's |

---

## 0.8 — any sandbox with a network: a local git endpoint

### Goal

MicroVMs (Firecracker: E2B, Fly and others), managed remote sandboxes (Modal, Daytona,
Runloop, Codespaces) and CI runners on other machines cannot mount gitd's directories, but
they can reach it over the network. They get the same promise: a fast checkout, and no
credential of the git host.

**Done when:** a sandbox on another machine clones from gitd at local-network speed with a
token of gitd's that only opens one repository, pushes to its own branch, and gitd
publishes that branch to the git host — and the token is useless once expired.

### How it is built

- **gitd serves its pools over git's smart HTTP**, by running the official `git
  http-backend` (principle 5), read from the pool, on a listening address of the
  configuration.
- **Tokens of gitd's own.** A short-lived token (default an hour, at most a day) is issued
  per sandbox through the API, the command line and an MCP tool. It opens one repository
  for reading and one **sandbox repository** for writing; it is never the git host's
  credential. Kept hashed, in memory; revoked on expiry or when its workspace goes.
- **Writes go to a sandbox repository, not the pool.** Each token has a bare repository of
  its own, borrowing the pool's objects, where the sandbox may push any branch. Publishing
  takes a branch from there to the git host with gitd's credential, through the same code
  as `publish` from a workspace. The pool is never written by a sandbox.
- **Fast clones of large repositories.** The endpoint allows partial clones
  (`--filter=blob:none`): files arrive as they are read.

### Decisions

| # | Decision | Why |
|---|---|---|
| D8 | The official `git http-backend`, not a reimplementation | Principle 5; correctness of the protocol for free |
| D9 | A sandbox repository per token for writes | No ref rules to get wrong on a shared pool; publishing reuses `publish` |
| D10 | Tokens short-lived, scoped to one repository, hashed, revocable | A leaked token opens little, for little time |
| D11 | This way before Kubernetes' CSI driver | It covers more sandboxes (every microVM and remote one, CI) with less code |

### Risks and how they are handled

| Risk | Handling |
|---|---|
| A network service is exposed | Off unless configured; TLS with a given certificate, or a private network only; every request needs a token |
| A token leaks | Short life, one repository, read plus its own sandbox repository only; revocable; never the git host's credential |
| Abuse and load | The same queues and limits as the rest of gitd; a token can be rate limited |
| Pools are shallow by default | A sandbox gets the history its pool has; full-history pools serve full clones |

### Tasks

| Task | What | Done when |
|---|---|---|
| **0.8.0 Endpoint** | Smart HTTP over the pools with `git http-backend`; upload only | `git clone http://gitd/…` works with a token, fails without one |
| **0.8.1 Tokens** | Issue, check and revoke; an RPC, a CLI command and an MCP tool | Expired and revoked tokens refused; a token opens only its repository |
| **0.8.2 Sandbox repositories** | Receive into a repository per token; publish from it | A push from a sandbox becomes a branch on the git host through gitd |
| **0.8.3 Network** | Listen address, TLS, limits | Tests with TLS and without; refused when unconfigured |
| **0.8.4 Examples** | E2B and Modal (or any microVM) cloning from gitd | Each example runs end to end |

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
      volumeAttributes: { repository: https://github.com/acme/web, branch: main }
```

**Done when:** on a kind cluster in CI, a pod gets its workspace, an agent Job commits and
publishes, and the pod never holds a credential.

### How it is built

- **gitd runs on every node** (a DaemonSet), its pools and templates on the node's disk,
  its credentials from a Kubernetes Secret mounted into gitd only.
- **A CSI node plugin** (a DaemonSet too) implements ephemeral inline volumes: when a pod
  starts, it asks the gitd on its node for a workspace in external mode and mounts the
  overlay layers into the pod — CSI drivers are privileged by design, which is where the
  privilege to mount belongs. When the pod goes, it unmounts and gitd removes the layer.
- **A Helm chart** installs both.

### Decisions

| # | Decision | Why |
|---|---|---|
| D12 | Ephemeral inline CSI volumes, node plugin only | A workspace lives as long as its pod; no controller or volume claims to run |
| D13 | Credentials only in gitd's Secret | Pods, and the agents in them, never hold one |

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
| **0.9.0 Node plugin** | CSI NodePublish and NodeUnpublish for ephemeral volumes | A pod on kind gets a workspace |
| **0.9.1 Helm chart** | gitd and the plugin as DaemonSets, the Secret, the configuration | `helm install` on kind and a real cluster |
| **0.9.2 Examples** | A pod and an agent Job that commits and publishes | Run end to end in CI |

---

## 1.0 — everywhere developers are

| Task | What | Done when |
|---|---|---|
| **1.0.0 SDKs** | Python and TypeScript clients generated from the contract with buf, published to PyPI and npm | An agent platform calls gitd in a few lines |
| **1.0.1 macOS** | A darwin binary with a `clonefile` strategy (APFS's reflink), for laptops | Workspaces on a Mac in milliseconds with almost no disk |
| **1.0.2 Guides** | One per sandbox: Docker, Kubernetes, E2B, Modal, Claude Code and Codex with bubblewrap | Each guide runs as written |

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
| macOS laptops | `clonefile` strategy | 1.0 |

---

## Work plan

In order; each step is one or more commits, tested, and each version ends with its
benchmark, its docs and a release.

| Step | Version | Work | Depends on |
|---|---|---|---|
| 1 | 0.6.0 | Release what is done: binaries for amd64 and arm64, the image | — |
| 2 | 0.7.0 | Run the spike; record its table here; fix the strategy order | 1 |
| 3 | 0.7.1 | Templates | 2 |
| 4 | 0.7.2 | Strategies and detection (copy moved first, then reflink, overlay, fuse-overlayfs) | 3 |
| 5 | 0.7.3 | `gitd-mount` | 4 |
| 6 | 0.7.4 | Contract, CLI and MCP | 4 |
| 7 | 0.7.5 | Lifecycle | 5, 6 |
| 8 | 0.7.6 | Conformance suite gating the strategies | 4 |
| 9 | 0.7.7 | Benchmark "100 agents", example, docs; release 0.7.0 | 7, 8 |
| 10 | 0.8.0–0.8.4 | Endpoint, tokens, sandbox repositories, network, examples; release 0.8.0 | 9 |
| 11 | 0.9.0–0.9.2 | CSI node plugin, Helm chart, examples; release 0.9.0 | 9 |
| 12 | 1.0.0–1.0.2 | SDKs, macOS, guides; release 1.0.0 | 10, 11 |

**How the work is done.** Heavy runs (benchmarks, the spike, kind clusters) go on GitHub's
runners: gitd's repository is public, so they cost nothing, and anyone can run them again.
Every change has its tests, every commit is signed off, and README numbers come only from
those runs.

---

## Decisions already taken

| Decision | Why |
|---|---|
| gitd is general and carries nothing of any one platform | Anyone can use it; a platform's paths and policies are its configuration |
| Focus: instant, secretless checkouts for agent sandboxes | Where gitd is different from `git worktree`, clones and agent tools |
| Credentials kept by gitd, by URL prefix, naming where the secret is | Callers, agents and sandboxes never hold one |
| MCP tools never take a credential, enforced by a test | The agent deals in paths, commits and branches only |
| The command line on spf13/cobra, one file per command | Maintained for over a decade; single responsibility |
| Apache License 2.0 instead of the AGPL-3.0 | Platforms can build on gitd; patents granted |
| The name stays `gitd` | Renaming moves the repository and buys nothing now |
| The README states only measured numbers | Credibility; "half a second whatever the repository" was corrected by measurement |
| The local git endpoint (0.8) before the CSI driver (0.9) | It reaches more sandboxes with less code |
