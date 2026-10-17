# gitd — threat model

What gitd protects, from whom, and how; and what it leaves to whoever deploys it. Every
claim here names the code or the test that makes it true. Reviewed against the code at
version 0.7, and extended for what 0.8 adds — connections, GitHub Apps, publishing settings
and pull requests — which awaits the outside review of 0.9 (see [Review](#review)).

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
| **An MCP client** (an agent's model) | Call the eight MCP tools, with any arguments | Nothing beyond what the tools do |
| **A caller on the socket** (the platform, the command line) | Every call of the API: credentials in requests, connecting GitHub, adding and removing credentials, setting publishing rules | Everything gitd does: see §5 |
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
workspace — update, checkout, submodules, publish and describe — are made while its sandbox
is stopped or paused** (§4, first rows). So:

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
| **A caller on the socket is fully trusted** | The socket has no authentication in 0.8: whoever can open it may prepare, remove or publish any workspace, send credentials, connect GitHub, add or remove credentials and change the publishing rules | Give the socket's group to the platform only. 0.9 adds tokens and tenants, and an administrator key for connections and settings |
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
   describes it; prepare needs no pause.
7. Install gitd-mount for overlay workspaces, with its configuration matching gitd's state
   and roots; it is the only part that runs as root.
8. Keep gitd's state and roots on a `nosuid,nodev` filesystem, limit gitd's outbound
   network to the git hosts it serves, and ship the audit log elsewhere.

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
added §4's rows on what submodules let a token read and on container root. **A review by a
person outside the project is still due before 0.9 opens a network service.**
