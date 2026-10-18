# E2B, Modal and other remote sandboxes

A sandbox that cannot mount gitd's directories — a Firecracker microVM, a sandbox in
someone else's cloud — clones from gitd's endpoint with a token of gitd's own. Serve the
endpoint where the sandbox can reach it, over TLS:

```toml
[endpoint]
listen = "0.0.0.0:9420"
url = "https://gitd.example.com:9420"
tls_cert_file = "/etc/gitd/tls/cert.pem"
tls_key_file = "/etc/gitd/tls/key.pem"
```

Then, per sandbox:

```python
from gitd import Gitd

with Gitd(url="https://gitd.example.com:9420", key=open("/run/secrets/gitd-key").read()) as gitd:
    issued = gitd.endpoint.issue("https://github.com/acme/web")
    # in the sandbox: git clone <issued.authenticated_url()> work; … git push origin HEAD:agents/fix
    gitd.publish("", "agents/fix", sandbox=issued.info.sandbox)
    gitd.endpoint.revoke(issued.info.id, remove_sandbox=True)
```

[`examples/network/e2b_sandbox.py`](../../examples/network/e2b_sandbox.py) and
[`modal_sandbox.py`](../../examples/network/modal_sandbox.py) do exactly this from an E2B or
a Modal sandbox. They need those accounts, and are not run in CI.
[`examples/network`](../../examples/network) runs the same flow with a sandbox container on
its own network, in CI.
