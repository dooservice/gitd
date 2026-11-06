# gitd-client for Rust

A client for [gitd](https://github.com/dooservice/gitd): instant, secretless git checkouts
for agent sandboxes. Your agents work in their own copy and publish a branch; they never
see your token.

```sh
cargo add gitd-client
```

The crate is `gitd-client`; the library is `gitd`. It is async, on tokio.

```rust
use gitd::{Gitd, Options, PrepareOptions, PublishOptions};

let gitd = Gitd::connect(Options::new()).await?; // $GITD_SOCKET, then /run/gitd/gitd.sock
let path = "/var/lib/gitd/workspaces/agent-1";
let workspace = gitd.prepare("https://github.com/acme/web", path, PrepareOptions::default()).await?;
println!("{} at {}, by {:?}", path, workspace.commit, workspace.strategy());

// … the agent works in the workspace with plain git, and commits …

gitd.publish(path, "agents/fix-login", PublishOptions::default()).await?;
gitd.remove(path).await?;
```

No call takes a credential: gitd reads and publishes with the ones it keeps.

| Method | What it does |
|---|---|
| `prepare(url, path, PrepareOptions { branch, commit, submodules, full_history, strategy, warm, new_branch, on_progress })` | A workspace of a repository at a path under gitd's roots, on a new branch of its own with `new_branch` |
| `update(path, UpdateOptions { branch, merge, submodules, on_progress })` | To the tip of its branch, or of the branch given; with `merge`, that branch merged into the one it is on |
| `checkout(path, commit, CheckoutOptions { submodules, on_progress })` | To a commit, as a rollback does |
| `describe(path)` | Its repository, commit, branch, uncommitted changes and submodules |
| `changes(path, include_ignored)` | Each file changed since gitd put it on its commit, with its lines |
| `diff(path, DiffOptions { files, max_bytes, include_ignored })` | The unified diff of those changes, cut short at a size |
| `compare(first, second, CompareOptions)`, `conflicts(sides, url)` | How two works differ; which pairs do not merge cleanly |
| `publish(path, branch, PublishOptions { requested_by, pull_request, provenance, sandbox, update })` | Its commits pushed to a new branch, or to the branch it is on when `branch` is empty; with `update`, or a branch it published before, that branch moved forward; its pull request opened or updated |
| `remove(path)` | Deleted; its pool stays |
| `seal(path, name)`, `warm_layers(url)` | Warm layers: what installing dependencies wrote, for later workspaces to stand on |
| `watch(paths)` | What happens to workspaces, as a stream |
| `probe(url)` | Whether it can be read, its default branch and branches |
| `info()` | Which gitd answers, and how it makes workspaces here and why |
| `audit(AuditOptions { since, url, limit })` | Every publish, done or refused, newest first |

What gitd answers is the contract's own messages, in `gitd::proto`. The rest of the
contract — the credentials gitd keeps, publishing settings, the endpoint's tokens — is on
the contract's clients, which bring the key too: `gitd.connections()`,
`gitd.publishing()`, `gitd.endpoint()`, and `workspaces()`, `repositories()`, `server()`.

A refusal is `Error::Refused`, whose `Refusal` has the `reason` (`Reason::AuthRequired`,
`NotFound`, `PathRefused`, `BranchRefused`, `Busy`, …; `reason_name()` gives
`AUTH_REQUIRED` as the other clients do) and `said`, what git said. Decide from the reason,
never from the message. A call that failed without a refusal of gitd's — gitd not
reachable, a key refused — is `Error::Call`, with its gRPC status; `error.code()` reads
either.

## Which gitd

`Gitd::connect(Options::new())` calls the gitd the environment names, as gitd's command
line does: on the network at `GITD_URL`, else on the socket at `GITD_SOCKET`, then gitd's
default socket (`/run/gitd/gitd.sock`; `C:\ProgramData\gitd\gitd.sock` on Windows). The
key naming your tenant is read from the file `GITD_KEY_FILE` names, else from `GITD_KEY`,
and goes on every call as `Authorization: Bearer <key>`. `GITD_CA_FILE` is the authority
of a certificate the system does not trust, trusted besides the system's. The options say
the same in code, and win over the environment:

```rust
let options = Options::new()
    .url("https://gitd.internal:9420")
    .key_file("/run/secrets/acme")
    .ca_file("/etc/gitd/ca.pem")
    .timeout(std::time::Duration::from_secs(120));
let gitd = Gitd::connect(options).await?;
```

Nothing is dialled until the first call. TLS is rustls's, which takes a server's
certificate only when it is not itself a certificate authority's: a self-signed one made
with `openssl req -x509` needs `-addext basicConstraints=critical,CA:FALSE`.

On Windows gitd serves its socket as AF_UNIX, which tokio cannot dial there; the client
connects with `uds_windows` and reads and writes through `async-io`.

[`examples/agent.rs`](examples/agent.rs) is an agent's whole round trip:
`cargo run --example agent -- <repository url> <workspace path>`.

## Layout

| Path | What it is |
|---|---|
| `src/client.rs` | `Gitd`: connecting, the contract's clients, `info`, `probe`, `audit` |
| `src/workspaces.rs`, `src/review.rs` | The calls on workspaces, and those that read their changes |
| `src/options.rs`, `src/target.rs`, `src/environment.rs` | Which gitd, and the channel to it |
| `src/socket/` | The Unix socket: tokio's, and Windows' own |
| `src/key.rs`, `src/http_refusal.rs` | The key on every call; a plain HTTP refusal read as its gRPC status |
| `src/error.rs`, `src/refusal.rs` | `Error`, and the `GitFailure` gitd attaches to a refusal |
| `src/proto.rs` | The generated contract, `gitd::proto` |
| `src/gen/` | Generated from [`proto/gitd/v1/gitd.proto`](../../proto/gitd/v1/gitd.proto); never edited by hand |

`src/gen` is generated with `buf generate --template buf.gen.yaml ../../proto`, by buf's
remote plugins for prost and tonic, and kept in the repository so that building the crate
needs neither `buf` nor `protoc`. CI regenerates it and fails when it differs.

`tests/gitd.rs` calls a running gitd: `e2e/rust-sdk.sh` starts one and runs them on its
socket and on its network address. Rust 1.85 or later (edition 2024). Apache-2.0.
