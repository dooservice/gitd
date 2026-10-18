# gitd — threat model

What gitd protects, from whom, and how; and what it leaves to whoever deploys it. Every
claim here names the code or the test that makes it true. Reviewed against the code at
version 0.7, and extended for what 0.8 adds — connections, GitHub Apps, publishing settings,
pull requests, reading and comparing what agents changed, watching, provenance and warm
layers — and for what 0.9 adds: the network endpoint and its tokens, sandbox repositories,
tenants and the API on the network. 0.9 shipped after an AI review, not a person's; a
review by a person outside the project is still due (see [Review](#review)).

## 1. What gitd protects

| Asset | Why it matters |
|---|---|
| **Credentials** — the tokens and SSH keys gitd keeps or is given, GitHub Apps' private keys, sign-ins' renewal tokens | They read and push private repositories; keeping them out of sandboxes is gitd's point |
| **The git hosts' branches** | An agent should add a branch for review, never change one people use |
| **gitd's own files** — pools, templates, layers, state, the audit log | Every workspace borrows from them; whoever writes them reaches every workspace |
| **The machine outside the roots** | gitd writes only where its configuration says |
| **The record of what was published** | Who published what, when and from where |

## 2. Who it protects them from

| Actor | Can | Trusted with |
|---|---|---|
| **An agent or a sandbox** | Write anything in its own workspace, `.git` included; run any program there; choose what it commits, `.gitmodules` included | Nothing but its workspace |
| **A repository's contents** | Name submodules, their paths and URLs; hold any file | Nothing |
| **An MCP client** (an agent's model) | Call the ten MCP tools, with any arguments | Nothing beyond what the tools do |
| **A sandbox on the network** with an endpoint token | Clone its token's repository from its sandbox repository, push there, for the token's life | That repository and its own sandbox repository, nothing else |
| **A tenant** (one customer of a platform hosting many) | Every call of the API with its key, on the socket or the network | Its own part of gitd only: nothing of another tenant's |
| **A caller on the socket** (the platform, the command line) | Every call of the API: credentials in requests, connecting GitHub, adding and removing credentials, setting publishing rules | Without tenants, everything gitd does; with tenants, everything its key's tenant has: see §5 |
| **gitd's own user** | Everything gitd can | Not with root: see gitd-mount |
| **The network and the git hosts** | Answer anything, redirect | Hosts' identities are checked (SSH host keys, TLS) |
| **A browser on gitd's machine** | Open the page `gitd connect github-app` serves on 127.0.0.1 while it waits | Nothing: the page takes back only the code that comes with its own state |

The line that matters: **a sandbox is not trusted; a caller on the socket is.** gitd keeps
the first from reaching credentials, its files or other branches; it does not keep the
second from anything, until 0.9 gives callers tokens and tenants.

## 3. How each threat is met

### Credentials never reach a sandbox

- **Never on a command line or in a file.** A token travels to git as an HTTP header in
  `GIT_CONFIG_*` of git's environment, scoped by URL to the one repository being read; a
  key is a path handed to ssh through `GIT_SSH_COMMAND`
  ([`credentials/access.go`](../internal/credentials/access.go)).
- **Never in a workspace.** Workspaces fetch from their pool on this disk; nothing in a
  workspace's configuration names a credential, and publishing pushes **from the pool**,
  where gitd owns the configuration, so a hostile `insteadOf` or `http.proxy` left in a
  workspace never sees where the token goes ([`pool/publish.go`](../internal/pool/publish.go)).
- **Never through MCP.** No tool takes a credential, and a test fails if one ever does
  (`TestNoToolEverAsksTheAgentForACredential`, [`mcpserver`](../internal/mcpserver)).
- **One credential, the most specific.** A repository is read with the longest prefix that
  opens it, once; gitd never tries credentials in turn
  (`TestARepositoryIsReadWithTheMostSpecificCredentialAndNoOther`).
- **No redirects.** git runs with `http.followRedirects=false`, so a request is answered by
  the host it went to or fails. (Before, with git's default, a test with a redirecting
  server showed the header not carried to the other host by git 2.39.5 and libcurl 7.88.1;
  turning redirects off removes the question for every later request too.)
- **Never to a host its prefix does not name.** A token kept with an empty prefix stops gitd
  at start, and one given without a host carries nothing, so a `.gitmodules` naming an
  arbitrary host gets no token (`TestATokenIsNeverSentToAHostItsPrefixDoesNotName`). A key
  may have an empty prefix: ssh checks every host against known keys (below).
- **No host trusted on first use.** ssh runs with `StrictHostKeyChecking=yes`, the host keys
  GitHub, GitLab, Bitbucket and Codeberg publish built in, and others only from files the
  configuration names.
- **git asks nobody.** Every git process gets a closed environment — no global or system
  configuration, no askpass, no terminal prompt — and `credential.helper` emptied on its
  command line ([`git/rules.go`](../internal/git/rules.go)).
- **Kept tokens in memory.** `token_file` is best a systemd credential (`LoadCredential=`),
  which lives in memory and stays root's on disk.

### Credentials kept through the API stay gitd's

- **Never answered.** `ListConnections` shows what a credential opens and whom it acts as,
  never its secret (`TestATokenAddedThroughTheAPIIsListedWithoutItsSecretAndRecorded`); none
  of `ConnectionService` or `PublishingService` is an MCP tool, and a test fails if one
  ever becomes one (`TestNoToolLetsAnAgentTouchConnectionsSettingsOrWhoIsAskedToReview`).
- **Kept on disk for gitd alone.** Each connection is a file under `<state>/connections`,
  `0600`, written atomically ([`internal/files`](../internal/files)); its secret is sealed
  with AES-256-GCM when `secrets_key_file` names a key, so a copy of the state directory
  opens nothing without it ([`internal/secrets`](../internal/secrets)). A secret that
  cannot be opened stops the start. IDs are made by gitd, never given, so no file is named
  outside the directory.
- **A token for one repository, for one purpose, for an hour.** A GitHub App's private key
  never leaves gitd: it signs a JWT, and the installation token gitd asks for is limited to
  the one repository and to what the operation needs — contents read to fetch, contents
  write to push, pull requests write to open one
  (`TestATokenIsForOneRepositoryAndWhatTheOperationNeeds`). Tokens are kept in memory only,
  renewed five minutes before they expire, and used as tokens are: a header in git's
  environment in the pool. The key is read again for each JWT, so replacing it rotates it.
- **GitHub says what an App is.** `AddGitHubApp` keeps an App only once GitHub has taken its
  key, and what it acts as comes from GitHub, not from the caller.
- **Creating an App from a manifest** holds a random state for an hour, good once
  (`TestAnAppIsCreatedFromAManifestOnceAndKept`); the code is checked as one GitHub sends;
  GitHub may send the person back only to an `https` address or to `localhost`. The
  command line's page listens on 127.0.0.1 and takes a code only with its state.
- **A sign-in** keeps its renewal token sealed, replaced each time GitHub renews it, and the
  token it renews in memory only; neither the sign-in nor the renewal takes a secret of
  gitd's public App.
- **GitHub's API is called as the git hosts are**: TLS, a timeout, answers bounded to a
  megabyte, and redirects never followed, so a token is sent only where it was meant for
  ([`githubapp/api.go`](../internal/githubapp/api.go)).

### A workspace cannot make gitd run, write or publish for it

A sandbox writes its workspace's `.git`, and gitd runs git there. Without care that is a
way in: a filter in `.git/config` runs as gitd's user on the next checkout. The checks below
hold for what is on disk when gitd looks, which is why **the operations that run git in a
workspace — update, checkout, submodules, publish, describe, and on a written workspace
changes, diff, compare and conflicts — are made while its sandbox is stopped or paused**
(§4, first rows). So:

- **Only listed configuration keys.** Before gitd runs git in a workspace it lists every key
  of its configuration, and of each submodule's, without following includes, and refuses
  any key not on an allow-list with `CONFIG_REFUSED`, naming the keys
  ([`workspace/config_keys.go`](../internal/workspace/config_keys.go),
  [`config_guard.go`](../internal/workspace/config_guard.go)). The list holds what git and
  gitd write and what only describes the repository or its author; nothing whose value git
  runs or loads. A key git adds tomorrow is refused until it is listed.
  (`TestAKeyThatRunsACommandIsRefused`, and end to end with `core.hookspath`.)
- **`.git` is the workspace's own.** A `.git` that is a link, or a link inside
  `.git/modules`, is refused; a submodule's `core.worktree` must name a directory of the
  workspace outside `.git` (`TestASubmoduleMayPointOnlyAtItsCheckoutInsideTheWorkspace`).
- **Nothing in a git directory leads elsewhere.** In the workspace's git directory and each
  submodule's, a `commondir` is refused, `objects`, `objects/info`, `refs` and `HEAD` must be
  plain directories and files, and `alternates` may name only the objects of pools that
  list the workspace as borrowing from them
  ([`gitdir_guard.go`](../internal/workspace/gitdir_guard.go),
  `TestAGitDirectoryLeadingOutOfTheWorkspaceIsRefused`). The alternates and `shallow` gitd
  writes go through an `os.Root` of the git directory.
- **No hooks, no fsmonitor, HTTPS and SSH only** — set on git's command line, where a
  repository's configuration cannot turn them back on. The file protocol is allowed only
  for the fetches gitd itself makes between a workspace and its pool.
- **A workspace cannot become one of another repository.** Where a publish goes is the
  workspace's `gitd.url`, which the sandbox can write; it counts only when that
  repository's pool lists the workspace among its own (`gitd-owned`), a list only gitd
  writes. A workspace naming a repository it merely borrows for a submodule is not one of
  its own (`TestAWorkspaceThatRenamesItsRepositoryIsNotOneGitdMade`, and end to end).
  A submodule is published only when gitd checked it out in that workspace — its
  repository's pool lists the workspace — never because `.gitmodules` names it
  (`TestASubmoduleGitdNeverCheckedOutIsNotPublished`).
- **gitd's writes stay in the workspace.** A submodule's directories and `.git` file, whose
  path the repository chooses and where the sandbox may have left a link, are made through
  an `os.Root` of the workspace, which refuses to follow a link out of it
  (`TestASubmoduleLinkedOutOfTheWorkspaceGetsNothingWritten`). A submodule's name and path
  must stay inside the workspace as git's own checks require.
- **Branches are checked** with `git check-ref-format`, and a leading `-` refused, whether a
  caller gives them or gitd reads one from a workspace.

### Reading what an agent changed runs nothing of the agent's

`Changes`, `Diff` and `Compare` say what an agent changed; `Conflicts` whether works merge
([`workspace/changes*.go`](../internal/workspace), [`compare.go`](../internal/workspace/compare.go),
[`conflicts.go`](../internal/workspace/conflicts.go)).

- **On overlay, no git runs in the workspace.** The candidates are read from the agent's
  own layer and the files are reached through an `os.Root` of the workspace, so a link
  there is read as a link, never followed; each is hashed as git would and compared with
  the base commit's blob, read from the pool. Which files git ignores is asked of the
  template, which gitd built and the sandbox never wrote
  (`TestChangesReadFromALayerAgreeWithGit`, on the kernel's overlay in CI).
- **Lines and diffs are counted on copies.** The changed files are copied aside into a
  private directory — a link as the path it names, a file over 16 MB as binary — and git
  compares the copies with `--no-index`, `--no-ext-diff` and `--no-textconv`, so neither a
  link nor a `.gitattributes` driver of the repository gets git to read or run anything
  else. A diff stops at a size the caller sets, 512 KiB by default.
- **On a written workspace**, git lists the changes there, after the same checks of `.git`
  as every call that runs git in a workspace; so does `Conflicts` before fetching a
  workspace's commit into the pool under a ref of its own, which it then forgets. The
  merge itself is `git merge-tree` in the pool, writing nothing to any work tree.
- **Watch says what happened, never what is in a file**: paths, kinds and commits, to a
  caller on the socket. A watcher too slow to read is dropped, not waited for, and a
  workspace's layer is looked at only while someone watches it.

### The endpoint opens one repository, with a token of gitd's, never the host's

Sandboxes that cannot mount gitd's directories clone over HTTP
([`internal/endpoint`](../internal/endpoint)):

- **A token of gitd's own.** `gitd_` and 32 random bytes, kept as its SHA-256 in memory,
  lost on restart, alive an hour unless told and a day at most, revocable; it opens one
  repository for reading and one sandbox repository for writing. The git host's credential
  never leaves gitd. Issuing and revoking are in the audit log.
- **Only git's smart HTTP, through git itself.** `git http-backend` runs with gitd's closed
  environment, no hooks, and its settings on the command line where no repository can
  change them; the address must name the token's repository, only the three smart requests
  are served, a service is named once, and a read-only token never reaches receive-pack
  (`TestATokenOpensOnlyItsRepositoryWhileAliveAndAsItWasIssued`).
- **Pushes land in the sandbox repository, never the pool.** Deletes and forced pushes are
  refused, objects checked, a push bounded (`max_push_bytes`), and none taken once the
  tenant's disk is full. **The pool's refs are never shown** to a sandbox: receive-pack
  would offer them, and with them the commits other sandboxes and workspaces staged and did
  not publish (`TestASandboxIsNeverShownWhatThePoolHoldsForOthers`); what a refused publish
  staged is forgotten.
- **Publishing from a sandbox repository** goes through the same branch prefix, rules, pull
  request and audit log as a workspace's (`TestWhatASandboxPushedIsPublishedUnderTheSameRules`).
  gitd runs no git in a sandbox's files: what it publishes it fetches from the sandbox
  repository, whose configuration only gitd writes. **For sandboxes on the endpoint, the rule
  to pause a sandbox while gitd runs git in its workspace does not apply.**
- **Bounded:** sixteen git processes at once per tenant, requests bounded in size, idle
  connections closed.

### Tenants share nothing

With `[[tenants]]` each tenant is a part of gitd of its own
([`internal/daemon`](../internal/daemon), [`internal/server/gateway.go`](../internal/server/gateway.go)):
its state under `<state>/tenants/<name>` — pools, templates, layers, sandbox repositories,
credentials, connections, publishing settings, tokens, audit log — its roots, and limits on
workspaces, disk (sandbox repositories counted), fetches and sandbox repositories. A call
reaches the part whose key it brings, compared in constant time with the hash of a key file
read again when it changes; an endpoint token reaches the part that issued it; anything else
is refused, on the socket too. `TestTwoTenantsOnOneGitdSeeNothingOfEachOther` and
`TestOnTheNetworkATokenOrAKeyOpensItsOwnTenantOnly` try each tenant's key and token on the
other's. Without tenants, the network address answers the API only with `api_key_file`'s key.

### A pod gets its workspace, never a credential

On Kubernetes ([`internal/csi`](../internal/csi), [`charts/gitd`](../charts/gitd)), gitd runs
unprivileged on every node with its credentials from a Secret mounted into it only; the CSI
plugin, privileged as CSI node plugins are, holds only the tenants' keys and mounts. A pod
gets its workspace and its pool's objects read-only inside it, nothing else: no socket, no
credential. A workspace carrying its pool is accepted only while the pool is truly mounted
there — a link or a copy at `.git/gitd-pool` is refused
(`TestAWorkspaceCarryingItsPoolIsAcceptedOnlyWithThePoolTrulyMountedInside`) — and is never
removed while it is mounted, so removing cannot reach into the pool. A pod's volume
attributes cannot make a path: namespace, pod and name must be Kubernetes names.

### Warm layers carry what a sandbox installed, and only that

A warm layer is an overlay workspace's own layer, sealed for later workspaces to stand on
([`workspace/seal.go`](../internal/workspace/seal.go),
[`template/warm.go`](../internal/template/warm.go)). It was written by a sandbox, so:

- **gitd never runs the install**: the platform runs it in its sandbox, then calls `Seal`.
  No MCP tool seals or picks a warm layer; only a caller on the socket does.
- **Never the workspace's `.git`.** It is set aside before the layer is moved, so nothing
  a sandbox wrote to git's configuration, hooks or alternates reaches the workspaces on it,
  which get a `.git` of their own from their template and every check above
  (`TestAWarmLayerCarriesWhatOneWorkspaceInstalledToTheNext`, on the kernel's overlay in CI).
- **Moved, never written after.** Sealing refuses, and puts the workspace back, while its
  overlay is still mounted anywhere gitd can see — a sandbox's bind of it lives on after
  gitd unmounts it, and what it wrote would land under every workspace on the layer
  (`ErrHeld`, tested with a bind mount). The layer is mounted read-only below each
  workspace's own; a workspace on it writes only to its own layer.
- **Bounded to what gitd-mount allows**: two lower layers at most, each pinned beneath the
  templates with `openat2` and owned by gitd's user, as a template is.
- **One layer of a name, replaced only when unused**, and retired after `retire_after` with
  no workspace on it.

### gitd writes only under its roots

- A workspace path must be absolute and lie under a root once its links are resolved
  ([`workspace/paths.go`](../internal/workspace/paths.go)), and not inside another
  workspace, whose directories a sandbox could change between gitd's check and its write.
- What is at a path is never replaced unless gitd made it; a workspace is built beside its
  destination and renamed into place.
- Removing deletes without following links.

### Agents publish only where they may, and only what the rules allow

- Publishing creates **new branches only**, and **never forces** a push.
- **Only a token publishes**: a deploy key is read only.
- A branch outside the prefix the publishing settings set is refused (`BRANCH_REFUSED`)
  before anything is staged.
- Nothing is pushed when the workspace's own repository brings no new commit.
- **The publishing settings are checked in the pool, on the commits to push, before any
  push** — never in the workspace, which the sandbox writes. Each commit is read on its own,
  so a change undone later is still refused; a rename is a deletion, so a test renamed away
  is a test deleted; a merge counts only what it brings beyond merging its parents
  (`--diff-merges=remerge`), so a change hidden in a merge is refused and merging what
  people changed on `main` is not. Every repository about to be pushed, submodules
  included, is checked against its own settings
  (`TestTheCommitsToPublishSayWhichPathsTheyTouch`,
  `TestAPublishThatBreaksARuleIsRefusedBeforeAnythingIsPushed`). git matches the paths;
  a pattern that would be git's pathspec magic is refused.
- **Every setting that opens a repository applies**, each checked on its own: the paths a
  setting allows free only its own rules, never what another setting protects or keeps
  (`TestAllowedPathsFreeOnlyTheirOwnSettingsRules`); the smallest file limit wins; and a
  narrower branch prefix must start with every broader one, so no setting loosens another.
  Presets are set by a caller on the socket, as settings are, and recorded.
- **A pull request is opened with the credential that pushed**, after the push; if it cannot
  be, the branch stays published and the answer says why. An agent may ask for one through
  MCP, but never say who asked for the work: that would let it have gitd ask anyone to
  review.

### Every publish is on the record

Every publish, done or refused — with who asked for it and its pull request — and every
change to connections and publishing settings, is one line of
`<state>/audit/publishes.jsonl`, synced
before the call answers; a line a crash cut short is skipped and never joins the next
([`internal/audit`](../internal/audit)). Read through `Audit`, `gitd audit`,
`read_audit_log` and both clients.

### gitd needs no privilege: gitd-mount

Overlay workspaces are mounted by gitd-mount, a separate root service
([`internal/mounthelper`](../internal/mounthelper)):

- Socket-activated, `0600`, owned by gitd's user; the caller is identified by the kernel
  (`SO_PEERCRED`) and anyone else refused.
- Its configuration is root's and writable by root only, and so is every directory above
  it; it is opened once, without following a link. It fixes the only directories that may be
  layers and the only roots a workspace may be mounted in. gitd cannot widen it.
- Every directory is opened with `openat2(RESOLVE_NO_SYMLINKS | RESOLVE_BENEATH)`, its base
  too, from `/`, and must be owned by gitd's user; the mount goes through that descriptor, so a directory swapped for a
  link after the check changes nothing. Mounts are `nosuid,nodev`.
- It unmounts only what is an overlay at its own mount point, through the pinned descriptor.
- Its unit keeps only what mounting an overlay takes: `CAP_SYS_ADMIN` to mount, and
  `CAP_DAC_OVERRIDE`, `CAP_FOWNER` and `CAP_CHOWN`, because the kernel writes an overlay's
  upper and work directories with its mounter's credentials. Its checks are fuzzed
  (`FuzzAMountIsNeverCheckedOutsideItsDirectories`) and its root tests run in CI.

A compromised gitd can therefore mount only overlays of its own directories inside its own
roots. `nosuid,nodev` holds for the mounted view; the filesystem holding the layers should
itself be `nosuid,nodev` (§5), so that a setuid file a sandbox leaves in its upper directory
is harmless there too.

### MCP tools say what they do

Each tool says whether it only reads or destroys (a test keeps the annotations right), and
none takes a credential. An MCP client reaches only what the tools offer.

## 4. What gitd does not protect against

Known, and left to the deployment or to a later version:

| Gap | Why | What to do |
|---|---|---|
| **A caller on the socket is fully trusted** | Without tenants the socket has no authentication: whoever can open it does anything gitd does. With tenants, a key opens all of its tenant, connections and settings included: there is no key for less | Give the socket's group to the platform only, and each tenant's key to that tenant's part of the platform |
| **No person has reviewed 0.9 yet** | The endpoint, tokens and tenants were reviewed by an AI agent (see Review); a review by a person outside the project is still due | Keep the network address on a private network or behind TLS, and read this section before exposing it |
| **An endpoint token in the sandbox** | A sandbox clones with the token in its URL, which stays in its `.git/config`; whatever runs in the sandbox can read it | It opens one repository and one sandbox repository for its life: issue it short (`--for`), read-only when the sandbox only reads, and revoke it when the sandbox ends |
| **A push may create many refs** | A push is bounded in bytes, not in how many refs it creates in the sandbox repository | Keep tokens short-lived; `limits.sandboxes` and the disk limit bound a tenant's sandbox repositories |
| **Endpoint tokens are lost on restart** | They are kept in memory only; a sandbox repository stays, a token for it must be issued again | Issue tokens again after a restart; publishing from a sandbox repository needs no token |
| **The CSI plugin is privileged** | It mounts into every pod of the node, as CSI node plugins do, and holds the tenants' keys | Run it from the chart only; it takes requests from kubelet alone, on a socket in kubelet's plugin directory |
| **Sandboxed runtimes are untested** | gVisor and Kata Containers were not tested with the plugin; Kata shares a volume through virtio-fs, where the pool's mount inside the workspace may not carry | Test on yours before relying on it |
| **The metrics are shared between tenants** | One registry; `gitd_pool_bytes` names repositories, of every tenant | Serve the metrics address to the operator only |
| **Kept secrets are on gitd's disk** | Without `secrets_key_file`, connections' secrets are files only gitd's user reads; with it, whoever reads the key and the files reads them | Set `secrets_key_file` from a systemd credential, and keep the key out of backups of the state directory |
| **Publishing rules see paths, not intent** | They refuse changes to protected paths, deleted kept paths and too many files; harmful code in any other file passes them | Keep reviewing pull requests; protect the branches they merge into on the host |
| **The CI runs what an agent changed** | Rules by path cannot know which files the CI executes: a test, a `Makefile`, `package.json`'s scripts. GitHub gives repository and organisation secrets to the workflows of every branch of the same repository, an agent's included | Keep secrets in environments agents' branches cannot use, make the workflow token read only, and run nothing privileged for `agents/*` ([GITHUB.md](GITHUB.md)) |
| **A sign-in acts as the person** | A person's token reaches what they and gitd's public App reach, and cannot be narrowed to one repository per operation as an App's can | Prefer the organisation's own App for anything shared; sign in only for one person's own work |
| **An agent with MCP is a caller on the socket** | `gitd mcp` calls the socket; an agent that runs it, or has a shell where the socket is reachable, can do what any caller can — to other agents' workspaces too | Run `gitd mcp` for one agent at a time, and never mount the socket into a sandbox. 0.9 scopes each token to one workspace |
| **Sandboxes read every pool they are given** | A workspace reads its pool's objects through `alternates`; a sandbox given the whole pools directory reads every repository in it | Mount only the pools a sandbox's workspace borrows from. 0.9 keeps tenants' pools apart |
| **Pools are readable by local users** | Pools and their files are `0755`/`0644`, so sandboxes running as other users can read them | On a shared host, give the state directory to a group (`StateDirectoryMode=0750`) and put sandboxes' users in it |
| **Tokens are visible to gitd's user and root** | A token is in the environment of the git processes gitd starts, which `/proc/<pid>/environ` shows to the same user and to root | Run gitd as its own user, and no sandbox as that user |
| **gitd fetches what repositories name** | A caller's URL, or a submodule URL in a repository, is fetched from gitd's network position — internal hosts included — and its contents end up in a workspace | Limit gitd's outbound network to the git hosts it serves |
| **A sandbox running while gitd runs git in its workspace** | gitd checks `.git` — configuration, links, alternates — and then runs git, which reads it again; a sandbox changing it in between undoes the checks. No check made before git runs can close that | **Run update, checkout, submodules, publish and describe while the workspace's sandbox is stopped or paused** (`docker pause` is instant); prepare needs no pause, as a new workspace has no sandbox yet. An agent calling an MCP tool waits for its answer: let no process of its sandbox run in the background meanwhile. 0.9's sandbox repositories end this: the sandbox pushes to gitd, which no longer reads its `.git` |
| **A warm layer holds what its install wrote** | A sandbox ran the install; an install script, or an agent in that sandbox, can leave anything in the layer — a changed dependency, a file a later test imports — and every workspace on it starts with it | Seal from a sandbox that ran only the install, from a commit people reviewed, before any agent works there; seal again rather than reuse one an agent touched |
| **A process outside every sandbox still in a sealed workspace** | Sealing finds the workspace's overlay in every mount table gitd can see; a process of gitd's own user with its working directory inside, after a lazy unmount, keeps writing and is in none | Seal once the sandbox and whatever ran in the workspace have stopped |
| **Provenance is told, not proven** | The agent, model, session and task a publish names are the caller's word, recorded and shown as given | Treat them as the platform's statement; the audit log says which caller made the publish |
| **Submodules choose what a kept token reads** | A repository's `.gitmodules` names its submodules, and gitd reads each with the most specific kept credential that opens it: anything under a token's prefix, private repositories included | Give tokens prefixes as narrow as the repositories agents work on (`https://github.com/acme/web`, not the whole organization) |
| **Adopting trusts the clone** | `Adopt` takes a clone's `origin` as its repository | Adopt only clones a sandbox has not had |
| **The audit log is gitd's to write** | It records honestly but is not tamper-proof against gitd's own user, and a publish is not undone if its entry cannot be written (that is logged as an error) | Ship the file to a log store the gitd user cannot change |
| **Container root on the host's disk** | A sandbox that is root in its container, without a user namespace, can leave setuid files or device nodes in its workspace on the host | Run sandboxes with user namespaces, and keep gitd's state and roots on a `nosuid,nodev` filesystem |
| **Disk and CPU** | `[limits]` bounds concurrent fetches, writes and queued calls, but not how much a sandbox writes into its workspace | Quotas on the roots' filesystem. 0.9 adds per-tenant limits |
| **git LFS** | gitd neither fetches LFS objects into pools nor allows `filter.lfs.*` in a workspace; a workspace gets LFS pointer files, and gitd says which (`lfs_patterns`) | Not supported; considered after 1.0, on demand |

## 5. Deploying it safely

1. Run gitd as its own unprivileged user (`packaging/gitd.service`), and no sandbox as it.
2. Give the socket's group to the platform only; never mount the socket into a sandbox.
3. Connect GitHub with the organisation's own App (`gitd connect github-app`, or
   `BeginGitHubApp` from the platform), installed only on the repositories agents work
   on; keep other tokens in systemd credentials (`LoadCredential=`), each with a prefix
   naming its host and organization: `https://github.com/your-org`. Set
   `secrets_key_file`.
4. Set publishing settings with a branch prefix and the `recommended` preset, and treat that
   prefix on the git host as you would any branch an outsider proposes: no secrets for its
   workflows, a read-only workflow token, and a person's approval before merging
   ([GITHUB.md](GITHUB.md)).
5. Mount into each sandbox its workspace, read-write, and only the pools it borrows from,
   read-only. Run sandboxes with user namespaces.
6. Stop or pause a workspace's sandbox while gitd updates, checks out, publishes or
   describes it, and on a written workspace while it reads its changes or conflicts;
   prepare, and changes on overlay, need no pause. Seal only once the sandbox has stopped,
   from one that ran only the install.
7. Install gitd-mount for overlay workspaces, with its configuration matching gitd's state
   and roots; it is the only part that runs as root.
8. Keep gitd's state and roots on a `nosuid,nodev` filesystem, limit gitd's outbound
   network to the git hosts it serves, and ship the audit log elsewhere.
9. Serve the network address (`[endpoint]`) over TLS, or on a private network only; never
   send a key over plain `http://` off the machine. Give each tenant its own key, from a
   systemd credential, and limits; with tenants, give gitd-mount each tenant's templates,
   layers and roots.

## Review

Reviewed against the code at the commit that added this file. The review found, and fixed
before writing this:

- **A workspace's configuration could run a command as gitd's user** (a filter, on the next
  checkout): now refused by the allow-list. Confirmed by reproducing it in a container
  before the fix, and its refusal end to end after.
- **A workspace could name another repository**, its submodule's included, and be published
  there with gitd's credential: now checked against `gitd-owned`. The first fix, against the
  pool's list of borrowers, let a submodule's repository through; the end-to-end run caught
  it, and the test now covers it.
- **gitd's own writes for a submodule followed links out of the workspace:** now made through
  `os.Root`.
- **A token with an empty prefix went to any host:** now refused.

It also made consistent, without a vulnerability behind them, how branches read from a
workspace and branches to publish are checked.

**What 0.8 adds** — connections and their store, GitHub Apps and sign-ins, publishing
settings checked in the pool, pull requests — was written against this document and
reviewed by its author; the outside review of 0.9 · 6 covers it before the network endpoint
ships.

**Independent review (0.7 · 10).** A second review, by a reviewer that started from this
document and the code without the author's context — an AI agent, not a person — found,
and these were fixed:

- **A submodule was published wherever the working tree's `.gitmodules` pointed:** now only
  submodules gitd checked out in the workspace.
- **Other files in `.git` led elsewhere** (`commondir`, `alternates`, links at `objects`):
  now refused, and alternates limited to pools lending to the workspace.
- **gitd wrote alternates and `shallow` through links inside `.git`:** now through `os.Root`.
- **Lock files were cleared through a `.git` that was a link:** no longer.
- **Redirects** are now off; gitd-mount checks the directories above its configuration and
  opens the base of every path without links; gitd's unit restricts setuid files.

Running the sandboxes example with gitd-mount as it is deployed — unprivileged gitd, the
helper with its unit's capabilities — then showed what no test had: with `CAP_SYS_ADMIN`
alone, gitd-mount could not enter the probe's directory (made private by `os.MkdirTemp`),
and an overlay it mounted came up read-only, as the kernel writes the layers with the
mounter's credentials. The probe's directories now have the layers' mode, the unit holds
`CAP_DAC_OVERRIDE`, `CAP_FOWNER` and `CAP_CHOWN` besides, the review's `0700` on the layers
was taken back (it shut gitd-mount out; the `nosuid` filesystem covers what it was for), and
CI runs the example on every push.

It also showed that no check before git runs can stand against a sandbox changing `.git`
at the same time: §4 now says to pause the sandbox for those operations, until 0.9. It
added §4's rows on what submodules let a token read and on container root.

**AI review of 0.9 (0.9 · 6).** 0.9 was released after a review by an AI agent given this
document and the code since 0.8.0 — not by a person; the project's owner chose to release
without waiting for one. It found, and these were fixed:

- **A writable token could read other sandboxes' and workspaces' unpublished commits**:
  receive-pack offered the tips of the pool's refs as `.have` lines, staged commits of
  refused publishes among them, and a push naming one took it. Reproduced in a test; the
  pool's refs are now never shown, and what a refused publish staged is forgotten.
- **One tenant could fill the shared disk through sandbox repositories**, which no limit
  counted: they now count toward the disk limit, are capped per tenant, and no push is
  taken once the disk is full.
- **No idle timeout, no bound on request bodies or git processes** on the network address:
  now bounded. A service named twice in a request, which Go and git read differently, is
  refused; a sandbox's branch is checked; a key file is reread on any change; the command
  line warns when a key would travel unencrypted.

It recorded as acceptable, in §4: shared metrics, refs per push, tokens in a sandbox's
`.git/config`. **A review by a person outside the project is still due.**
