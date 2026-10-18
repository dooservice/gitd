# Kubernetes

Install gitd and its CSI node plugin on every node, then ask for a workspace like any
volume:

```sh
helm install gitd charts/gitd \
  --set credentialsSecret=gitd-credentials \
  --set-string config='[[credentials]]
prefix = "https://github.com/acme"
github_app = { app_id = 123, private_key_file = "/etc/gitd/credentials/app.pem" }'
```

```yaml
apiVersion: v1
kind: Pod
metadata: {name: agent-1, namespace: agents}
spec:
  securityContext: {runAsUser: 1000, runAsGroup: 1000}
  containers:
    - name: agent
      image: your-agent-image
      workingDir: /work
      volumeMounts: [{name: repo, mountPath: /work}]
  volumes:
    - name: repo
      csi:
        driver: gitd.dev
        volumeAttributes: {repository: https://github.com/acme/web, branch: main}
```

The workspace lives on the node at `/var/lib/gitd/workspaces/<namespace>/<pod>`. Publish
what the pod committed from gitd's container on that node, while the pod still runs:

```sh
kubectl exec <gitd pod on the node> -c gitd -- gitd publish /var/lib/gitd/workspaces/agents/agent-1 agents/fix
```

When the pod goes, its workspace goes with it. [`examples/kubernetes`](../../examples/kubernetes)
runs all of this on kind: `./demo.sh`.
