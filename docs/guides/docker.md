# Docker, Podman and containerd

gitd makes each agent's workspace on the host — an overlay on a shared template, in
milliseconds — and the container mounts it like any directory, with its repository's pool
read-only at the same path, where the workspace borrows its objects from.

```sh
gitd prepare https://github.com/acme/web /var/lib/gitd/workspaces/agent-1
docker run --rm -it \
  -v /var/lib/gitd/workspaces/agent-1:/work \
  -v /var/lib/gitd/pools/github.com/acme/web.git:/var/lib/gitd/pools/github.com/acme/web.git:ro \
  -w /work --user 1000:1000 your-agent-image
# the agent commits with plain git; then, on the host:
gitd publish /var/lib/gitd/workspaces/agent-1 agents/fix
```

- Mount only the pool the workspace borrows from (`.git/objects/info/alternates` names it),
  read-only. Never mount gitd's socket or state.
- Run the container as gitd's user, or a user that can write the workspace.
- Pause the container (`docker pause`) while gitd updates, checks out or publishes the
  workspace: gitd checks `.git` and then runs git there.

[`examples/sandboxes`](../../examples/sandboxes) runs two such containers with gitd
unprivileged beside `gitd-mount`, as it is deployed: `./demo.sh`.
