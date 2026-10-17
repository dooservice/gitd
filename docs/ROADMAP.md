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

## Where gitd started: 0.6 (released)

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

## 0.7 — copy-on-write workspaces, and what platforms need first (released)

### Goal

A workspace of kubernetes in **milliseconds** and **a few megabytes** per agent, instead
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
  use it on the layers gitd hands over in 0.10 — and gitd does not test it.
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
   `type=overlay`), Kubernetes (the CSI driver of 0.10), rootless Podman or bubblewrap inside
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
  branch and commit, the new commits' subjects, and, from 0.9, which tenant and token — in
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
| D5b | fuse-overlayfs is not tested in a user namespace | A mount made in one is seen only there, so gitd could not hand it to a sandbox: the result could not change a decision. It is left to runtimes, on the layers of 0.10 |
| D6 | `update` and `checkout` stay plain git inside the workspace | Correct in every case |
| D7 | New contract fields only | Principle 2 |
| D7b | Handing a runtime the layers to mount itself moves to 0.10 | Docker needs none — gitd-mount mounts the workspace on the host, and the container binds it like any directory; the CSI driver is the first that needs layers, so it shapes that part of the contract |
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
| The audit log grows without end | Rotated by size, rotated files deleted by age; both configurable |

### Tasks

| Task | What | Done when |
|---|---|---|
| **0.7 · 0 Spike** ✓ | Overlay, fuse-overlayfs, reflink and copy on kubernetes, 100 agents | Done: the table above |
| **0.7 · 1 Templates** ✓ | `internal/template`: build, register users, retire; housekeeping retires the unused | Done: built once even when asked for at once, shared, never rebuilt, retired only when unused and rested, swept after a crash; built by the workspace package, detached at its commit and tuned for copies |
| **0.7 · 2 Strategies** ✓ | One interface; copy (today's code moved behind it), overlay (external and helper), reflink (`FICLONE`); detection at start | Done: overlay and reflink makers, detection (a state on an overlay told apart), Prepare and Remove through them; end to end with the kernel's overlay, workspaces after the first in 26–32 ms. Whole-history and submodule workspaces are still written, until templates hold them |
| **0.7 · 3 `gitd-mount`** ✓ | The helper: a small root service with only the capabilities mounting an overlay takes, socket-activated, with systemd units | Done: refuses any path outside its directories or through a symlink, any caller but gitd's user, any unmount of what is not an overlay; tests (root ones in CI) and a fuzz target; end to end, an unprivileged gitd made workspaces in 34–36 ms through it |
| **0.7 · 4 Contract** ✓ | `[workspaces] strategy`; `strategy` on `Prepare`; `strategy` and `layers` in `WorkspaceResult` and `Describe`; strategy, its reason and templates in `Info`; CLI flags and MCP output | Done: buf breaking finds nothing against 0.6.0, and the 0.6.0 command line works against it |
| **0.7 · 5 Lifecycle** ✓ | Unmount on remove; remount at start; templates and layers in the start-up sweep and in `Info` | Done: remounted at start and every round, never over files written while unmounted; half-built and orphaned layers swept, their templates let go; an agent's commit survives a reboot, end to end |
| **0.7 · 6 Conformance** ✓ | What agents do — status, diff, add, commit, stash, rebase, checkout, publish — on every strategy in CI | Done: copy, overlay and reflink pass, the other workspace sees nothing and the template never changes; CI runs it as root with an XFS disk. fuse-overlayfs is not tested: it could not join gitd's choice whatever the result (above) |
| **0.7 · 7 SDKs** ✓ | Python and TypeScript clients from the contract with buf; published; one example each | Done: `gitd-client` (TypeScript, generated by buf) and `gitd` (Python, which reads the contract at run time from JSON built by buf: no generated code); each with an example, tested end to end against gitd; the release publishes both by trusted publishing, with no token kept |
| **0.7 · 8 Publishing rules** ✓ | `[publish] branch_prefix`; the audit log with its API, CLI and MCP reads, and rotation | Done: a branch outside the prefix is refused (`BRANCH_REFUSED`); every publish, done or refused, is a JSON line in `<state>/audit/publishes.jsonl`, synced, rotated by size and deleted by age; read by `Audit`, `gitd audit`, `read_audit_log` and both clients; tested end to end |
| **0.7 · 9 Threat model** ✓ | `docs/THREAT_MODEL.md`: what gitd protects, from whom, and how — credentials, roots, `gitd-mount`, MCP | Done, reviewed against the code; the review found and fixed four flaws first — a workspace's configuration could run a command as gitd's user, a workspace could name another repository to publish to, gitd's writes for a submodule followed links out, a token with no prefix went to any host — and records what is left to the deployment |
| **0.7 · 10 Security review** ✓ | An outside review, even informal, of the threat model, `gitd-mount` and credential handling, before 0.9 opens a network service | Done as an independent review by an AI reviewer without the author's context: gitd-mount and token scoping held; four flaws at the workspace boundary and five smaller ones fixed, and the rule to pause a sandbox while gitd runs git in its workspace recorded (THREAT_MODEL.md, Review). A review by a person outside the project moves to 0.9, before the endpoint ships |
| **0.7 · 11 Proof and docs** ✓ | Benchmark "100 agents on kubernetes"; the sandboxes example with overlay volumes; README and DESIGN | Done: 100 workspaces in 14.5 s (23 ms each after the first) and 850 MB, against 980 s and 41,870 MB for 100 shallow clones, all clean to git, one fetch; the sandboxes example runs unprivileged gitd beside gitd-mount, in CI on every push — where gitd-mount's real capabilities were found wanting and fixed |

---

## 0.8 — publishing a company can trust, and what platforms plug into

### Goal

Once agents publish for real, a platform and the company behind it ask three things gitd
can answer better than anything inside the sandbox:

- **A credential that renews itself and opens only what it must**: gitd as a GitHub App,
  with tokens of one hour for one repository, and nothing anyone renews by hand.
- **Rules on what a publish may contain**, enforced by gitd, outside the agent: no change to
  the CI, no deleted tests, no more than so many files.
- **What a platform needs to show agents' work and plug gitd in**: the changes and diff of
  a workspace, the difference between two agents and whether their work merges, a stream
  of what happens, a pull request opened on publish, who asked for the work, workspaces that
  start with their dependencies installed — and an integration with an agent platform
  people already use.

**Done when:** gitd fetches, publishes and opens a pull request as a GitHub App, with no
token made by hand; a publish that changes a protected path is refused and recorded; a
platform lists a workspace's changes, its diff and the difference between two workspaces
through gitd, measured on kubernetes; a workspace starts with its dependencies installed
without gitd running any of the repository's code; OpenHands runs an agent on a gitd
workspace with no git host token in its sandbox, end to end in CI; and callers written for
0.7 notice nothing.

### Why these, and why now

- **Agents' git credentials are what attacks go for.** In 2026, prompt injection through a
  pull request or an issue made agents in CI post their secrets ("Comment and Control",
  April 2026: Claude Code Security Review, Gemini CLI Action and Copilot Agent), and a
  coding agent's `GITHUB_TOKEN` was shown stolen through an issue. Injection cannot be fully
  patched; the defence agreed on is that the agent never holds the secret and a check
  outside it decides. gitd already keeps the secret; 0.8 adds the check.
- **Reviewing agents' work is where teams are slow**, not writing it: a reviewer needs what
  changed, cheaply, and the difference between attempts.
- **Platforms already ask for gitd's shape.** OpenHands's workspace interface requires
  `git_changes` and `git_diff`, and its SDK clones with the token inside the URL, which stays
  in the workspace's `.git/config`, where the agent can read it.
- **A short-lived token alone is not the fix** — an agent that can read it uses it while it
  lives. A GitHub App's tokens are worth having because gitd holds them, never the agent.

### How it is built

**Set through the API, not only a file.** gitd is run by platforms, which connect it to
GitHub, choose its rules and open pull requests from their own pages; editing a TOML file
on a server for each would be tedious. So in 0.8:

- **The configuration file keeps what installing gitd decides** — socket, state, roots,
  limits. Credentials and publishing rules may still be declared there, as configuration
  as code; the API shows those as read-only.
- **Connections and publishing settings are managed through the API**: a
  `ConnectionService` and a `PublishService`, in both SDKs. gitd keeps what is set under its
  state directory and applies it at once, without a restart.
- **The command line is one more client of that API** (`gitd connect github`, `gitd publish
  settings`), as the SDKs are.
- **None of it reaches MCP.** An agent can never see, add or remove a connection or a
  rule; a test enforces it, as it does for credentials today.
- **Secrets at rest** are files readable only by gitd's user under `<state>/connections/`,
  encrypted with AES-256-GCM when `secrets_key_file` names a key (best a systemd
  credential). Adding or removing a connection and changing a rule are recorded in the
  audit log.

**Connecting GitHub.** A platform puts a "Connect GitHub" button on its own page; behind
it, one of three ways:

| Way | What the person does | Pull requests appear as | For |
|---|---|---|---|
| **The organisation's own App** (recommended) | One click: GitHub's "Create GitHub App" from a manifest gitd writes, then choose the repositories | The organisation's bot, e.g. `acme-agents[bot]` | Teams and production |
| **Signing in** | Enters a code at github.com/login/device, as with `gh auth login` | The person | One person, or trying gitd |
| **The platform's existing App** | Nothing: the platform registers it once, or keeps sending a token with each call as today | The platform's bot | Platforms that already have one |

- **The organisation's own App**: `BeginGitHubApp(org, name)` returns the address and the
  manifest — permissions Contents and Pull requests (read and write) and Metadata, no
  webhook. GitHub sends the person back to the platform's page with a code, which the
  platform hands to `CompleteGitHubApp(code)`; gitd exchanges it for the App's ID and
  private key and keeps them. Nobody copies a key.
- **Signing in** uses GitHub's device flow with gitd's public App, whose client ID is in the
  binary: neither the sign-in nor renewing its tokens takes a secret. Its tokens last eight
  hours and are renewed for six months, kept only on the gitd server.
- **The platform's existing App**: `AddGitHubApp(app_id, private_key)`.
- **Why each organisation has its own App, not one `gitd[bot]`**: a bot's tokens are made
  with its App's private key, and a key shared by every gitd would open every customer's
  repositories. With its own App, the key stays on the organisation's server and nobody
  else — dooservice included — holds it.
- `ListConnections` says what each opens, as whom and until when, never a secret;
  `RemoveConnection` disconnects.

**Tokens of an App.** For every operation gitd signs a short JWT with the App's private
key, finds the App's installation for the repository's owner, and asks for an installation
token **limited to that one repository** and to what the operation needs: contents read to
fetch, contents write to push, pull requests write to open one.

- Tokens are kept in memory until shortly before they expire (an hour) — a hundred fetches
  at once ask for one — and never written to disk or logged.
- They are used as tokens are today: a header in git's environment in the pool, never in
  the workspace or on a command line.
- The private key is read again each time a JWT is signed, so replacing its file rotates it
  without a restart.
- The agent's commits stay the agent's; the push and the pull request come from the App,
  so a reviewer sees at once that an agent published it — and the person who asked, not
  being the author, may approve it.
- GitHub Enterprise Server is reached through the host's configured address. GitLab's
  equivalents (group and project access tokens) follow on demand.

**Publishing settings.** Per URL prefix, set through `PublishService.SetSettings` (or the
file): the branch prefix, the rules a publish must pass, and how pull requests are opened.
A platform need not write patterns: it picks a preset and adjusts it.

| Preset `recommended` | |
|---|---|
| May not be changed | `.github/workflows/**`, `.gitlab-ci.yml`, `CODEOWNERS`, `.gitmodules` |
| May change, not be deleted | tests: `**/*_test.*`, `**/*.test.*`, `**/test/**`, `**/tests/**`, `**/__tests__/**` |
| At most | 200 files changed |

- **Checked in the pool, commit by commit, before anything is pushed** — never in the
  workspace, which is the sandbox's to write. Every repository about to be pushed,
  submodules included, is checked against the settings of its own prefix.
- **git matches the paths** (`:(glob)` pathspecs), so `**` means what it means to git.
- Renaming a test away counts as deleting it (`--no-renames`). A change undone by a later
  commit is still refused: that history would be pushed too. A merge is checked on what it
  brings itself (`--diff-merges=remerge`): merging `main` with people's changes passes,
  hiding a change inside a merge does not.
- Every setting whose prefix opens the repository applies, not only the most specific: a
  narrower setting never loosens a broader one.
- A refusal is a new reason, `POLICY_REFUSED`, naming the rule and the paths ("agents/fix
  changes .github/workflows/ci.yml, which the settings for https://github.com/acme
  protect"); it is recorded in the audit log, and nothing is pushed.
- No settings, no change: 0.7's publishing stays as it is.

**Pull requests on publish.** The settings say whether a publish opens one (always, or when
asked), as a draft or not, with which labels, and whether to ask the person who asked for
the work to review it. A publish says who that was (`requested_by`) and may override the
title, body, base or draft.

- gitd opens it through the host's API with the same App it pushed with — GitHub first,
  GitLab's merge requests on demand — and returns its address and number, which the audit
  log records too.
- Its body says who asked and **what gitd checked**: the commits, the files and lines they
  change, the rules they passed, the submodules published — a reviewer reads in a minute
  what they would otherwise dig for. The platform's own text, when given, comes first.
- If the push succeeds and the pull request does not, the branch stays published and the
  answer says why the pull request failed (`pull_request_failure`): retrying the publish
  would only meet the branch it made.
- A host whose API gitd does not know answers `UNSUPPORTED` in that field.

**Changes.** `Changes(path)` lists what an agent changed: each file added, modified,
deleted, renamed or untracked, with lines added and removed, against the commit the
workspace was made at (or one given).

- **On overlay, gitd already knows what the agent touched**: everything is in its layer,
  and a deleted file leaves a mark there. gitd compares only those files with the template,
  instead of walking the whole tree as `git status` does. On kubernetes that is a handful of
  files against 31,441 — measured in the benchmark before any number is claimed.
- **No git runs in the workspace on overlay**: gitd reads the layer as data and runs git
  only in the template, which no sandbox can write. On copy, where git must run in the
  workspace, 0.7's rule holds: the sandbox is paused while it does.

**Diff.** `Diff(path, files)` gives the unified diff of some files or all, with a size
limit, and says when it cut it short — a diff of megabytes never reaches a page whole.

**Compare.** `Compare(a, b)` gives the difference between two workspaces, or two published
branches: "how do agent 3 and agent 7 differ?" — what choosing between several attempts at
one task needs. Two workspaces on the same template differ only in their layers, so gitd
compares those, not the whole tree.

**Conflicts.** `Conflicts(branches or workspaces)` says whether the work of several agents
merges cleanly, and where it does not, before anyone merges: "agents 2 and 5 both changed
`auth.go`". gitd runs `git merge-tree --write-tree` in the pool, which merges without a
working tree and runs none of the repository's code. Whether the merged code passes its
tests is for the platform's CI, never for gitd.

**Watch.** `Watch` is a stream of what happens: a workspace made, changed, published or
removed, a publish refused. A page or a platform follows it instead of asking every few
seconds. On overlay, a change is seen in the agent's layer (inotify) without touching the
workspace.

**Provenance.** `Publish` may carry where the work came from — the agent, the model, the
session, the task's address — as short fields, never a secret. They go into the audit entry
and the pull request's body. gitd records what it is told and says so; it never rewrites a
commit.

**Workspaces with their dependencies ready.** Each new workspace today starts without
`node_modules` or `.venv`, and each agent installs them again — the complaint most often
heard about worktrees.

- The platform makes a workspace, runs its setup (`npm ci`, `uv sync`) in **its own
  sandbox**, and calls `Seal(path, name)`. gitd turns that workspace's layer into a
  read-only **warm layer** of its template.
- `Prepare` with that name stacks template, warm layer and the agent's own layer —
  overlay takes several lower layers — so each agent starts with everything installed, in
  milliseconds, and only what it changes is its own.
- **gitd never runs the repository's code**: the setup ran in the platform's sandbox. A
  warm layer was written by a sandbox, so gitd treats it as it treats a workspace — the
  same guards on its `.git` — and retires it, like a template, when no workspace uses it.

**Command line and MCP.** Every new call has its command (`gitd changes`, `gitd diff`,
`gitd compare`, `gitd conflicts`, `gitd watch`, `gitd seal`) and, where an agent needs it,
an MCP tool — none of which takes a credential, as today.

**OpenHands, as the first integration.** OpenHands (MIT, the most-starred open agent
platform) runs each conversation in a Docker sandbox, and its SDK clones the repository
into it with the token in the URL. With gitd:

1. A workspace made by gitd is bound into the sandbox instead of a clone — in
   milliseconds, with dependencies ready from a warm layer.
2. Its `git_changes` and `git_diff` come from gitd's `Changes` and `Diff`.
3. Pushing and the pull request go through gitd's `Publish`, under its policies, with no
   token in the sandbox.

It lands first as `examples/openhands`, run in CI, and is then proposed to OpenHands.

### Decisions

| # | Decision | Why |
|---|---|---|
| D19 | GitHub App before any other host's equivalent | Most agent platforms publish to GitHub; it is what companies install |
| D19b | Connections and publishing settings through the API; the file only for what installing gitd decides | gitd is run by platforms, from their own pages; editing a file on a server for each is tedious |
| D19c | Pull requests as the organisation's own App, created in one click; signing in only for one person | A bot shows an agent did it and lets the requester approve; a shared `gitd[bot]` would need one key that opens every customer's repositories |
| D20 | Installation tokens scoped to one repository and the operation's permissions, kept in memory only | A token that leaks opens one repository for minutes, and the agent never holds one anyway |
| D21 | Policies checked in the pool on the commits to push, never in the workspace | The workspace is the sandbox's to write; the pool is gitd's |
| D22 | Changes, Diff and Compare read the agent's layer on overlay, and run git only in the template; Conflicts merges in the pool with `git merge-tree` | Fast where the tree is large, and no git in a workspace a sandbox may be writing |
| D23 | Warm layers are built by the platform's sandbox and sealed by gitd; gitd never runs a setup | Running a repository's install would run its code as gitd's user |
| D24 | Provenance is recorded as told, never verified and never written into commits | gitd cannot know who asked; rewriting commits would change what the agent made |
| D25 | Publishing rules, pull requests and the GitHub App before the network endpoint | What companies ask first, mostly local, and the endpoint (0.9) publishes through them |
| D26 | The first integration is OpenHands, as an example in CI before a proposal upstream | The most-used open platform, with a workspace interface that already asks for changes and diff |

### Risks and how they are handled

| Risk | Handling |
|---|---|
| A copy of the disk or a backup carries a connection's secret | Files readable only by gitd's user; encrypted with a key kept apart (`secrets_key_file`) when given; an App's tokens live in memory only |
| A caller connects or changes rules it should not | Only a caller on the socket, trusted in full until 0.9, where these calls take a tenant's administrator key; never through MCP; every change in the audit log |
| The App's private key leaks | Read from a credential file (systemd `LoadCredential=`), never logged; its tokens open one repository each, for an hour; rotated in GitHub without restarting gitd |
| A policy is written too loosely | Refusals are recorded with the rule and paths; rules are tested against real repositories in CI; no rules means 0.7's behaviour |
| Changes on overlay disagree with git | The conformance suite compares `Changes` with `git status` on every strategy |
| A warm layer carries something harmful | It is a sandbox's work, checked as a workspace is; it never reaches gitd's configuration or the pool, and a workspace on it is as isolated as any |
| Watch floods a caller | Events are coalesced per workspace and the stream is bounded; a slow reader is dropped, not waited for |
| OpenHands changes its workspace interface | The example pins a version and runs in CI; upstream review keeps it in step |

### Tasks

| Task | What | Done when |
|---|---|---|
| **0.8 · 0 GitHub App** ✓ | `ConnectionService`: the organisation's own App from a manifest, signing in by device flow, a platform's App; tokens per repository and purpose, kept in memory; connections kept under the state, encrypted when a key is given, applied without a restart | A platform connects GitHub through the API with no file edited; fetch and publish as an App, end to end against a test App; no token on disk. **Done**: all three ways, the store, tokens per repository and purpose, `gitd connect` and `gitd connections`, both clients; gitd's public App, `gitd-app`, registered; github-e2e reads a private repository and publishes with the gitd-e2e App on every push, no token in any workspace |
| **0.8 · 1 Publishing settings** ✓ | `PublishingService`: presets, protected and kept paths, maximum files, branch prefix, per prefix; checked in the pool commit by commit; `POLICY_REFUSED`; audit | Each rule refuses what it should and nothing else, merges included; recorded; settable through the API and the file. **Done**: presets as data — gitd's `recommended`, replaceable, and any named through the API (`gitd publishing preset`) or the file — and paths a setting allows, freeing only its own rules; refusals before any push, renames as deletions, merges of main passing and hidden changes refused, submodules against their own settings; `gitd publishing`, both clients, `[[publish.settings]]` |
| **0.8 · 2 Pull requests** ✓ | Opened on publish as the App, per the settings: draft, labels, the requester asked to review; the body says what gitd checked; `pull_request_failure`; audit | A publish opens a pull request as `<org>-agents[bot]` with the requester as reviewer, end to end. **Done**: github-e2e opens one as `gitd-e2e[bot]`, a draft, labelled, the actor asked to review, its body counting the files and lines changed; the first runs found and fixed a token that could not read the branches and a count made after the push |
| **0.8 · 3 Changes and Diff** ✓ | From the layer on overlay, from git on copy; limits; CLI and MCP | Agree with `git status` and `git diff` on every strategy; measured on kubernetes. **Done**: against the commit gitd last put the workspace on, which the pool keeps; on overlay from the agent's layer — its files, deletions and remade directories — with no git in the workspace, agreeing with git on the kernel's overlay in CI; one file changed on kubernetes read in 0.012 s against 0.130 s for `git status`; `gitd changes`, `gitd diff`, `list_changes`, `show_diff`, both clients |
| **0.8 · 4 Compare, Conflicts and Watch** | The difference between two workspaces or branches; whether several merge cleanly; a stream of events | Compare agrees with `git diff` between them; Conflicts agrees with a real merge and runs no repository code; Watch sees a change, a publish and a removal |
| **0.8 · 5 Provenance** | Origin fields on `Publish`, into the audit log and the pull request | Recorded, bounded, shown; never a commit rewritten |
| **0.8 · 6 Warm layers** | `Seal`; `Prepare` on a warm layer; retirement | A workspace starts with `node_modules` installed in milliseconds, measured; gitd ran no setup |
| **0.8 · 7 OpenHands** | `examples/openhands`, in CI; then a proposal upstream | An OpenHands agent works and publishes on a gitd workspace with no token in its sandbox |
| **0.8 · 8 Proof and docs** | Benchmark of Changes and warm layers; README, THREAT_MODEL, DESIGN; a demo of a policy stopping an injected agent; the release checklist: npm's trusted publisher added again (it lapsed on 19 October 2026, unvalidated), the SDKs at 0.8.0, the release notes | Numbers measured; docs reviewed against the code; release 0.8.0. **Done ahead**: `docs/GITHUB.md`, so agents' branches get no secrets, and the threat model's gap that rules by path cannot see what the CI runs |

---

## 0.9 — any sandbox with a network, and many tenants on one gitd

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
workspaces, logs or tokens; and a gitd with no tenants configured behaves exactly as 0.8.

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
  exactly as before — the configuration and the calls of 0.8 stay valid.

### Decisions

| # | Decision | Why |
|---|---|---|
| D10 | The official `git http-backend`, not a reimplementation | Principle 5 |
| D11 | A sandbox repository per token for writes | No ref rules to get wrong on a shared pool; publishing reuses `publish` |
| D12 | Tokens short-lived, scoped to one repository of one tenant, hashed, revocable | A leaked token opens little, for little time |
| D13 | Tenants in 0.9, on the same tokens | A multi-customer platform asks first; the tokens are half of it |
| D14 | Nothing shared between tenants, public pools included, at first | Isolation that cannot be got wrong; sharing public pools can come later, measured |
| D15 | No tenants configured means one implicit tenant | Principle 2: nothing changes for anyone who does not need tenants |
| D16 | The endpoint (0.9) before Kubernetes' CSI driver (0.10) | It covers more sandboxes — every microVM and remote one, CI — with less code |

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
| **0.9 · 0 Endpoint** | Smart HTTP over the pools with `git http-backend`; upload only | `git clone http://gitd/…` works with a token, fails without one |
| **0.9 · 1 Tokens** | Issue, check and revoke; an RPC, a CLI command and an MCP tool | Expired and revoked tokens refused; a token opens only its repository |
| **0.9 · 2 Sandbox repositories** | Receive into a repository per token; publish from it, with the prefix and the audit log | A push from a sandbox becomes a branch on the git host through gitd |
| **0.9 · 3 Tenants** | `[[tenants]]`: keys, roots, credentials, limits; every call scoped; per-tenant pools, templates, workspaces, tokens and audit | Isolation tests pass; no tenants configured behaves as 0.8 |
| **0.9 · 4 Network** | Listen address, TLS, limits; gitd's API served there too (ConnectRPC over TLS, a tenant's key on every call), so a platform on another machine calls it as it calls the socket | Tests with TLS and without; refused when unconfigured; the SDKs reach a remote gitd |
| **0.9 · 5 Examples** | E2B and Modal (or any microVM) cloning from gitd; a two-tenant setup | Each example runs end to end |
| **0.9 · 6 Outside review** | A person outside the project reviews the endpoint, tokens, tenants and the threat model before the network service ships; sandbox repositories also retire 0.7's rule to pause a sandbox while gitd runs git in its workspace | Findings fixed or recorded; THREAT_MODEL.md updated |

---

## 0.10 — Kubernetes: a CSI driver

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
| **0.10 · 0 Node plugin** | CSI NodePublish and NodeUnpublish for ephemeral volumes; the contract's way of handing the plugin a workspace's layers to mount itself (from 0.7, D7b) | A pod on kind gets a workspace |
| **0.10 · 1 Helm chart** | gitd and the plugin as DaemonSets, Secrets, configuration | `helm install` on kind and a real cluster |
| **0.10 · 2 Examples** | A pod and an agent Job that commits and publishes | Run end to end in CI |

---

## 1.0 — everywhere developers are

| Task | What | Done when |
|---|---|---|
| **1.0 · 0 macOS** | A darwin binary with a `clonefile` strategy (APFS's copy-on-write), for laptops | Workspaces on a Mac with almost no disk, measured |
| **1.0 · 1 Guides** | One per sandbox: Docker, Kubernetes, E2B, Modal, Claude Code and Codex with bubblewrap | Each guide runs as written |
| **1.0 · 2 Stability** | The contract declared stable; a deprecation policy | Documented |

**After 1.0, on demand: git LFS.** Today a workspace holds LFS pointer files, and gitd says
which (`lfs_patterns`). Supporting LFS means pools fetching LFS objects — another API, often
another host for the token — and allowing `filter.lfs.*` with git-lfs's own values only.
Coding agents rarely need it; it waits until users ask.

---

## Sandboxes and how each gets a workspace

| Sandbox | Way | Version |
|---|---|---|
| Docker, Podman, containerd, gVisor | Local mount of a copy-on-write workspace | 0.7 |
| bubblewrap, nsjail, Landlock — how Claude Code and Codex CLI isolate commands on Linux | Local mount | 0.7 |
| Firecracker microVMs (E2B, Fly…) | Local git endpoint | 0.9 |
| Managed remote sandboxes (Modal, Daytona, Runloop, Codespaces) | Local git endpoint | 0.9 |
| CI runners | Local mount on the same host; the endpoint elsewhere | 0.7 / 0.9 |
| Kubernetes, GKE Sandbox (gVisor), Kata Containers | CSI driver | 0.10 |
| macOS laptops | `clonefile` | 1.0 |

---

## Launch: reaching the people gitd is for

The engineering is ahead of the audience: the repository has no stars yet. A **soft launch
runs alongside the code** — the MCP directories, a post with the story, a message to two or
three platforms — so that what people answer reaches the design while it is being written;
the Show HN waits for 0.8 and its security demo. Every review is named for what it was: the
0.7 review was an AI reviewer's, and the review by a person comes in 0.9. The headline
number is one anyone can measure again with the *hundred agents* workflow: *100 agents on
kubernetes: a workspace each in 23 ms and 4.3 MB, one fetch, no token in any sandbox; 67×
faster and 49× less disk than 100 clones.*

| Where | What |
|---|---|
| Hacker News | A "Show HN": the problem, the benchmark, the demo |
| Claude Code, Codex and Cursor communities | `gitd mcp` for parallel agents, with a short demo |
| MCP directories | The official MCP registry, the awesome-mcp-servers lists, Smithery and Glama |
| A post and a demo | The why and the numbers; a terminal recording of ten agents and a published branch |
| Agent and sandbox platforms | Talk to two or three (E2B, Daytona, Modal, Runloop or similar): would they integrate it, what is missing — the answers shape 0.8 and 0.9 |
| OpenHands | The integration of 0.8 · 7, proposed upstream: an agent on a gitd workspace, no token in its sandbox. The proposal links the exact lines of OpenHands' SDK where the token goes into the clone URL and stays in `.git/config`, so it reads as a fact, not a claim |
| A security demo | An agent told by an injected issue to change the CI and take the token: the token is not there, and the policy refuses the publish |
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
| 10 | 0.7 · 8 ✓ | Branch prefix and audit log | 6 |
| 11 | 0.7 · 9 ✓, 10 ✓ | Threat model and security review | 5, 10 |
| 12 | 0.7 · 11 ✓ | Benchmark "100 agents", example, docs; **release 0.7.0** ✓ and launch | 7–11 |
| 13 | 0.8 · 0–2 ✓ | GitHub App and connections through the API, publishing settings, pull requests | 12 |
| 14 | 0.8 · 3–5 | Changes, Diff, Compare, Conflicts, Watch, provenance | 12 |
| 15 | 0.8 · 6–8 | Warm layers, OpenHands, proof and docs; release 0.8.0 | 13, 14 |
| 16 | 0.9 · 0–6 | Endpoint, tokens, sandbox repositories, tenants, network, examples, outside review; release 0.9.0 | 15 |
| 17 | 0.10 · 0–2 | CSI node plugin, Helm chart, examples; release 0.10.0 | 16 |
| 18 | 1.0 · 0–2 | macOS, guides, stability; release 1.0.0 | 16, 17 |

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
| Multi-tenancy in 0.9, isolated per tenant, nothing shared at first | A multi-customer platform asks first; isolation must not be gettable wrong |
| A security review before 0.9's network service | The more surface gitd exposes, the more "secretless" must be proved |
| A soft launch alongside 0.8, the Show HN with 0.8's security demo | What people answer reaches the design while it is written; the demo of a refused publish is what gets attention |
| The local git endpoint (0.9) before the CSI driver (0.10) | It reaches more sandboxes with less code |
| No interface of gitd's own; gitd is what platforms and interfaces build on | Interfaces for parallel agents are many (Cursor, Codex, Conductor, Vibe Kanban…); a git layer that keeps credentials and enforces rules is not |
| The difference is security: the agent never holds a credential, and gitd decides what may be published | Agents' credentials are what prompt injection goes for, and worktrees, VM forks and git mirrors leave that open |
| GitHub App, publishing policies, pull requests, Changes/Diff/Compare/Watch, provenance and warm layers in 0.8, before the endpoint | What companies and platforms ask first; mostly local; the endpoint publishes through them |
| OpenHands is the first integration | The most-used open agent platform; it asks for changes and diff, and today puts the token in the sandbox |
