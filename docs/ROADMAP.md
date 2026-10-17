# gitd — roadmap

Where gitd is going, in the order it gets there. Each version ships on its own and keeps
the contract backwards compatible: fields and methods are added, never renamed.

The aim: **any AI agent, in any sandbox, gets its own checkout of any repository in
milliseconds, takes almost no disk, and never sees a credential.**

---

## Today (0.6)

Pools fetched once, workspaces that borrow their objects, credentials kept by gitd, a
command line, MCP tools, a Docker image and a benchmark. On kubernetes with ten agents,
gitd is 3.2× faster than ten clones and fetches once instead of ten times; each workspace
after the first takes 2.8 s, the time to write its files.

That last number is what the next versions remove.

---

## 0.7 — copy-on-write workspaces

**The problem:** every workspace writes every file of its repository: 2.8 s and some
370 MB per agent on kubernetes. So do clones, `git worktree` and most agent tools.

**The idea:** one checkout per commit, shared and read only — a **template** — and for each
agent a layer of its own on top that holds only what it changes. Like transparent sheets
on a table: the table is the template, each agent writes on its own sheet.

### How a workspace is made from a template

gitd picks, by itself, the fastest that the machine allows, in this order:

| Strategy | How | New workspace | Disk per agent |
|---|---|---|---|
| **overlay** | overlayfs: the template below, read only; an empty layer per agent above | milliseconds | what the agent changes |
| **reflink** | the template copied with block cloning (Btrfs, XFS, bcachefs, ZFS ≥ 2.2): files share blocks until written | about 1 s on kubernetes | almost none |
| **copy** | the files written, as today | 2.8 s on kubernetes | the whole checkout |

**Who mounts an overlay.** Linux lets only privileged processes mount one, and gitd runs
unprivileged. So, in this order, again by itself:

1. **The sandbox's own runtime**, when it already may: Docker's `type=overlay` volumes,
   Kubernetes (through the CSI driver of 0.9), rootless Podman or bubblewrap inside a user
   namespace (kernel ≥ 5.11). gitd hands it the layers' paths.
2. **`gitd-mount`**, a tiny helper that holds the one permission to mount and unmount
   layers, and only inside gitd's roots, as `fusermount` does for FUSE. gitd itself stays
   unprivileged.
3. **fuse-overlayfs**, the same layering without any privilege: slower than the kernel's,
   still writing no file.
4. **copy**, when none of the above is there.

Layers and templates live on a real disk (a host directory or volume): an overlay cannot
be stacked on another overlay, which is what a container's own filesystem is. gitd checks
at start and says so.

**git must notice nothing.** With overlay or reflink, files are identical but their inodes
and ctime change; git would re-read the whole tree on its first `status`. Workspaces are
configured with `core.checkStat=minimal` and `core.trustctime=false`, which compare size and
modification time only, both kept. A conformance suite runs what an agent runs — status,
diff, commit, stash, rebase, checkout, submodules, publish — on every strategy in CI; a
strategy that fails anything is not chosen automatically.

### Work

| # | Task |
|---|---|
| 0.7.0 | **Spike**: kubernetes on overlay (ext4) and reflink (XFS) in CI: creation time, disk, first `git status`, and that git is not confused |
| 0.7.1 | `internal/template`: build a template per repository and commit, register who uses it, retire it when unused |
| 0.7.2 | Strategies behind one interface: copy (today's), reflink (`FICLONE`), overlay (kernel, helper, fuse-overlayfs); detection at start |
| 0.7.3 | `gitd-mount`, the mount helper, with its own tests and a systemd unit |
| 0.7.4 | Contract: `[workspaces] strategy = "auto"`; `strategy` on `Prepare`; the layers in the answer for runtimes that mount them |
| 0.7.5 | Lifecycle: unmount on remove, remount at start, retire templates in maintenance |
| 0.7.6 | Conformance suite on every strategy |
| 0.7.7 | Benchmark: 100 agents on kubernetes; README and DESIGN |

---

## 0.8 — any sandbox with a network: a local git endpoint

MicroVMs (Firecracker: E2B, Fly and others), remote managed sandboxes (Modal, Daytona,
Runloop, Codespaces) and CI runners on other machines cannot mount gitd's directories. They
can reach it over the network.

- gitd serves its pools over git's smart HTTP, read from the pool, on this network.
- A sandbox gets a **short-lived token of gitd's own**, for one repository, never the git
  host's: it clones from gitd at local speed (with `--filter=blob:none`, files arrive as
  they are read), and may push only to branches of its own, `agent/*`.
- gitd publishes those branches to the git host with the credential it keeps.

| # | Task |
|---|---|
| 0.8.0 | Smart HTTP endpoint over the pools (upload-pack; receive-pack into `agent/*` only) |
| 0.8.1 | Ephemeral, scoped tokens: issued by an RPC and an MCP tool, revoked on expiry or removal |
| 0.8.2 | Publish from an endpoint push, as from a workspace |
| 0.8.3 | TLS and listening address; examples for E2B and Modal |

---

## 0.9 — Kubernetes: a CSI driver

A pod asks for a workspace like any volume:

```yaml
volumes:
  - name: repo
    csi:
      driver: gitd.dev
      volumeAttributes: { repository: https://github.com/acme/web, branch: main }
```

A node plugin (DaemonSet) asks the gitd on its node for the workspace, mounts the overlay
— CSI drivers are privileged by design — and binds it into the pod. Works with gVisor
(GKE Sandbox) and Kata Containers.

| # | Task |
|---|---|
| 0.9.0 | CSI node plugin, ephemeral inline volumes |
| 0.9.1 | Helm chart: gitd and the plugin as DaemonSets |
| 0.9.2 | Examples: a pod, an agent Job |

---

## 1.0 — everywhere developers are

| # | Task |
|---|---|
| 1.0.0 | SDKs in Python and TypeScript, generated from the contract with buf |
| 1.0.1 | macOS: a binary with a `clonefile` strategy (APFS), for laptops |
| 1.0.2 | Guides by sandbox: Docker, Kubernetes, E2B, Modal, Claude Code and Codex (bubblewrap) |

---

## Sandboxes and how each gets a workspace

| Sandbox | Way |
|---|---|
| Docker, Podman, containerd, gVisor | Local mount (0.7) |
| bubblewrap, nsjail, Landlock — how Claude Code and Codex CLI isolate commands on Linux | Local mount (0.7) |
| Kubernetes, GKE Sandbox, Kata Containers | CSI driver (0.9) |
| Firecracker microVMs (E2B, Fly…) | Local git endpoint (0.8) |
| Managed remote sandboxes (Modal, Daytona, Runloop, Codespaces) | Local git endpoint (0.8) |
| CI runners | Local mount on the same host (0.7), the endpoint elsewhere (0.8) |
