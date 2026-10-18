# Guides: gitd with each kind of sandbox

| Sandbox | How it gets a workspace | Guide | Runs in CI |
|---|---|---|---|
| Docker, Podman, containerd | A copy-on-write workspace mounted into the container | [docker.md](docker.md) | yes, `examples/sandboxes` |
| Kubernetes | A `gitd.dev` CSI volume | [kubernetes.md](kubernetes.md) | yes, `examples/kubernetes` on kind |
| Claude Code, Codex CLI (bubblewrap) | The workspace as the agent's working directory | [claude-code-and-codex.md](claude-code-and-codex.md) | the bubblewrap part, `examples/bubblewrap` |
| E2B, Modal, any microVM or remote sandbox | A clone from gitd's endpoint with a token | [remote-sandboxes.md](remote-sandboxes.md) | the endpoint, `examples/network`; not E2B or Modal, which need accounts |
| macOS laptops | A workspace cloned by APFS | [macos.md](macos.md) | yes, the macOS job |

In every case the sandbox gets its workspace and nothing else: no credential of the git
host, no socket of gitd's. The platform publishes through gitd.
