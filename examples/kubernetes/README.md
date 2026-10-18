# Workspaces as Kubernetes volumes

A pod asks for a workspace like any volume, and gets a copy-on-write checkout made by the
gitd on its node, with no credential in the pod:

```yaml
volumes:
  - name: repo
    csi:
      driver: gitd.dev
      volumeAttributes: { repository: https://github.com/acme/web, branch: main }
```

The Helm chart ([`charts/gitd`](../../charts/gitd)) runs, on every node, gitd — its pools
and templates on the node's disk, its credentials from a Secret mounted into it only — and
`gitd-csi`, the CSI node plugin. When a pod starts, the plugin asks gitd for a workspace
that carries its pool inside it, mounts the pool's objects read-only there, and binds the
workspace onto the pod's volume; when the pod goes, it unmounts both and gitd removes the
workspace.

`./demo.sh` runs it on a kind cluster: an agent's pod (`agent.yaml`) works in its workspace
with plain git and commits, gitd on the node sees the change, and deleting the pod removes
the workspace. With a credential (`CREDENTIALS_DIR`, `CONFIG`) and `BRANCH`, the platform
publishes what the pod committed:

```sh
kubectl exec <gitd pod> -c gitd -- gitd publish /var/lib/gitd/workspaces/<namespace>/<pod> agents/fix
```

CI runs it with the gitd-e2e App on every push, publishing the pod's commit.

## Notes

- A workspace's files are gitd's user's (1000 in the image): run agents' pods as that user
  to write, as `agent.yaml` does.
- One workspace per pod and volume, at `<root>/<namespace>/<pod>`; give a pod's second
  gitd volume a `name` attribute, and it lives at `<pod>-<name>`.
- Attributes: `repository` (required), `branch` or `commit`, `tenant` (with `[[tenants]]`
  and the chart's `tenants`), `name`.
- Ephemeral inline volumes only: a workspace lives as long as its pod.
