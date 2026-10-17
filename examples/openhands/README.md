# OpenHands on a gitd workspace

An [OpenHands](https://github.com/OpenHands/software-agent-sdk) agent works in a workspace
gitd made, inside a sandbox that holds no credential, and gitd publishes what it committed
as a pull request from a GitHub App's bot. CI runs it on every push to `main`, against
`dooservice/gitd-e2e` (the *openhands* job of `.github/workflows/github-e2e.yml`).

```
platform (openhands_task.py)              sandbox (sandbox/agent.py)
  gitd.prepare(repo, path)  ── bind ──▶     /workspace: OpenHands' agent, its terminal,
                                            plain git; no token, no socket, no network
  gitd.changes(path)        ◀── exits ──
  gitd.publish(path, branch, pull_request=…, provenance=…)
      └─▶ pushed with the App's token, checked against the publishing settings,
          the pull request opened as <app>[bot], saying where the work came from
```

What each side holds:

| | Holds |
|---|---|
| gitd | The App's private key, its tokens in memory, the publishing settings |
| The platform | The gitd socket |
| The sandbox | The workspace, and the pool it borrows its history from, read-only. Nothing else: `env` has no token, `.git/config` names `https://github.com/…` with none in it |

## Run it

With a gitd serving and a GitHub App kept in it, Docker and the Python client
(`pip install gitd`):

```sh
docker build -t gitd-openhands-sandbox examples/openhands/sandbox
GITD_SOCKET=/run/gitd/gitd.sock REPOSITORY=https://github.com/acme/web \
  ROOT=/var/lib/gitd/workspaces python examples/openhands/openhands_task.py
```

Without a model, a scripted one takes the steps a model would — look, add a note, commit,
show that no credential is there — through OpenHands' own agent, terminal tool and
conversation, so the run is the same every time and needs no key. To have a model do it,
set `LLM_MODEL` and `LLM_API_KEY` (and `LLM_BASE_URL`, `TASK` if you like); the sandbox
then gets a network for the model, still no git credential.

`demo.sh` is the whole run as CI does it, gitd and its App included: it needs
`GITD_E2E_APP_ID`, `GITD_E2E_APP_KEY`, `bin/gitd` (`make build`) and Docker.

## What it changes in OpenHands

Today OpenHands' SDK clones each repository into the sandbox with the token in the clone
URL (`_build_clone_url` and `_build_clone_command` in
[`openhands-sdk/openhands/sdk/workspace/repo.py`](https://github.com/OpenHands/software-agent-sdk/blob/5966c781ed014efc96c028a99bddd4ff78426741/openhands-sdk/openhands/sdk/workspace/repo.py#L297-L352)),
so it stays in the clone's `.git/config`, where the agent can read it. With gitd:

1. The workspace is made by gitd and bound into the sandbox instead of cloned there — in
   milliseconds on overlay, with dependencies ready from a warm layer (`prepare --warm`).
2. What the agent changed comes from gitd's `Changes` and `Diff`, read from its own layer.
3. Pushing and the pull request go through gitd's `Publish`, under the publishing settings,
   recorded in the audit log; the sandbox never holds a token.
