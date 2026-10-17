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

No call takes a credential: gitd reads and publishes with the ones it keeps. A refusal is a
`GitdError` with its `reason` (`AUTH_REQUIRED`, `NOT_FOUND`, `PATH_REFUSED`, `BUSY`, …) and
what git said. The whole contract is under `proto`.

[`examples/agent.ts`](examples/agent.ts) is an agent's round trip: prepare, commit, publish.

Node 22 or later. Generated from gitd's contract with `npm run generate`; built with
`npm run build`.
