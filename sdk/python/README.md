# gitd for Python

A client for [gitd](https://github.com/dooservice/gitd): instant, secretless git checkouts
for agent sandboxes. Your agents work in their own copy and publish a branch; they never
see your token.

```sh
pip install gitd
```

```python
from gitd import Gitd, GitdError

with Gitd() as gitd:                      # $GITD_SOCKET, then /run/gitd/gitd.sock
    workspace = gitd.prepare("https://github.com/acme/web", "/var/lib/gitd/workspaces/agent-1")
    print(workspace.path, workspace.commit, workspace.strategy)

    # … the agent works in workspace.path with plain git, and commits …

    published = gitd.publish(workspace.path, "agents/fix-login")
    gitd.remove(workspace.path)
```

No call takes a credential: gitd reads and publishes with the ones it keeps.

| Method | What it does |
|---|---|
| `prepare(url, path, *, branch, commit, submodules, full_history, strategy, on_progress)` | A workspace of a repository at a path under gitd's roots |
| `update(path, *, branch, submodules, on_progress)` | To the tip of its branch, or of the branch given |
| `checkout(path, commit, *, submodules, on_progress)` | To a commit, as a rollback does |
| `describe(path)` | Its repository, commit, branch, uncommitted changes and submodules |
| `publish(path, branch)` | Its commits pushed to a new branch |
| `remove(path)` | Deleted; its pool stays |
| `probe(url)` | Whether it can be read, its default branch and branches |
| `info()` | Which gitd answers, and how it makes workspaces here and why |

A refusal raises `GitdError`, with `reason` (`AUTH_REQUIRED`, `NOT_FOUND`, `PATH_REFUSED`,
`BUSY`, …) and `said`, what git said. Decide from the reason, never from the message.

[`examples/agent.py`](examples/agent.py) is an agent's whole round trip.

## Layout

| Path | What it is |
|---|---|
| `src/gitd/client.py` | `Gitd`, the client |
| `src/gitd/models.py` | What gitd answers, as dataclasses |
| `src/gitd/errors.py` | `GitdError` |
| `src/gitd/connection.py` | The channel to the socket, and how calls are made |
| `src/gitd/methods.py` | The gitd methods called, by their gRPC path |
| `src/gitd/messages.py` | The messages of the contract the client uses |
| `src/gitd/contract.py` | Reads the contract at run time, with protobuf's public API |
| `src/gitd/contract.json` | The contract, built from [`proto/gitd/v1/gitd.proto`](../../proto/gitd/v1/gitd.proto) by `make generate` |

No code is generated: the client reads gitd's contract when it starts.

Python 3.11 or later, on Linux. Apache-2.0.
