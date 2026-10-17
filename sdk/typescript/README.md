# gitd-client

TypeScript client for [gitd](https://github.com/dooservice/gitd): instant, secretless git
checkouts for agent sandboxes.

```sh
npm install gitd-client
```

```ts
import { Gitd, GitdError } from "gitd-client";

const gitd = new Gitd(); // $GITD_SOCKET, then /run/gitd/gitd.sock

const workspace = await gitd.prepare({
  url: "https://github.com/your-org/your-repo",
  path: "/var/lib/gitd/workspaces/agent-1",
});
console.log(workspace.commit, workspace.strategy); // "overlay": made in milliseconds

// … the agent works in workspace.path with plain git, and commits …

await gitd.publish(workspace.path, "agents/fix-login"); // pushed with the token gitd keeps
await gitd.remove(workspace.path);
```

No call takes a credential: gitd reads and publishes with the ones it keeps.

| Method | What it does |
|---|---|
| `prepare({ url, path, branch, commit, submodules, fullHistory, strategy, onProgress })` | A workspace of a repository at a path under gitd's roots |
| `update(path, { branch, submodules, onProgress })` | To the tip of its branch, or of the branch given |
| `checkout(path, commit, { submodules, onProgress })` | To a commit, as a rollback does |
| `describe(path)` | Its repository, commit, branch, uncommitted changes and submodules |
| `publish(path, branch)` | Its commits pushed to a new branch |
| `remove(path)` | Deleted; its pool stays |
| `probe(url)` | Whether it can be read, its default branch and branches |
| `info()` | Which gitd answers, and how it makes workspaces here and why |
| `audit({ since, url, limit })` | Every publish, done or refused, newest first |

A refusal is a `GitdError` with its `reason` (`AUTH_REQUIRED`, `NOT_FOUND`, `PATH_REFUSED`,
`BRANCH_REFUSED`, `BUSY`, …) and what git said. The whole contract is under `proto`.

[`examples/agent.ts`](examples/agent.ts) is an agent's round trip: prepare, commit, publish.

Node 22 or later. Generated from gitd's contract with `npm run generate`; built with
`npm run build`.
