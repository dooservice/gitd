# gitd for Go

A client for [gitd](https://github.com/dooservice/gitd): instant, secretless git checkouts
for agent sandboxes. Your agents work in their own copy and publish a branch; they never
see your token. It is the client gitd's own command line and MCP server use.

```sh
go get github.com/dooservice/gitd/client@latest
```

```go
gitd, err := client.Open(client.Options{}) // $GITD_SOCKET, then the system's default socket
if err != nil {
	return err
}
path := "/var/lib/gitd/workspaces/agent-1"
workspace, err := gitd.Prepare(ctx, &gitdv1.PrepareRequest{Url: "https://github.com/acme/web", Path: path}, nil)
if err != nil {
	return err
}
fmt.Println(workspace.GetCommit(), workspace.GetStrategy())

// … the agent works in path with plain git, and commits …

published, err := gitd.Publish(ctx, &gitdv1.PublishRequest{Path: path, Branch: "agents/fix-login"})
```

No call takes a credential: gitd reads and publishes with the ones it keeps. Requests and
answers are the contract's own messages, from `github.com/dooservice/gitd/gen/go/gitd/v1`.

| Method | What it does |
|---|---|
| `Prepare`, `Update`, `Checkout`, `SyncSubmodules`, `Adopt` | A workspace made, moved to a branch's tip or a commit, its submodules brought in, or one made elsewhere taken in; each takes a `Progress` for what git is doing, or nil |
| `Describe`, `Changes`, `Diff`, `Compare`, `Conflicts`, `Watch` | What a workspace is and what its agent changed, read without running anything of the agent's |
| `Publish` | Its commits pushed to a new branch, or with `Update` an earlier publish's branch moved forward; its pull request opened or updated |
| `Seal`, `ListWarm` | Warm layers: a workspace's installed dependencies kept for the next ones |
| `Remove` | Deleted; its pool stays |
| `Probe`, `Info`, `Retire`, `Audit` | A repository checked, which gitd answers and how, unused pools retired, every publish on record |
| `IssueToken`, `RevokeToken`, `ListTokens` | Tokens of gitd's own for a sandbox cloning from its endpoint |
| `ListConnections`, `AddToken`, `AddGitHubApp`, `BeginGitHubApp`, … | The credentials gitd keeps, never a secret |
| `GetSettings`, `SetSettings`, `ListPresets`, … | Publishing rules |

A gitd on another machine is called at its address, with the key naming the caller's
tenant, and the authority of its certificate when the system does not trust it:

```go
gitd, err := client.Open(client.Options{URL: "https://gitd.internal:9420", Key: key, CAFile: "/etc/gitd/ca.pem"})
```

A refusal is an error whose reason `client.Reason(err)` reads (`REASON_AUTH_REQUIRED`,
`REASON_NOT_FOUND`, `REASON_PATH_REFUSED`, …), with what git said. Decide from the reason,
never from the message.

Go 1.27 or later, on Linux, macOS and Windows; the same version as gitd. Apache-2.0.
