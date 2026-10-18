# gitd on the network, with tenants

A sandbox that cannot mount gitd's directories — a Firecracker microVM, a remote sandbox
(E2B, Modal, Daytona, Runloop), a CI runner on another machine — clones from gitd's endpoint
over HTTP with a **token of gitd's own**: it opens one repository of one tenant for reading
and one sandbox repository for writing, for an hour unless told, and is never the git
host's credential. What the sandbox pushes is published by the platform through gitd, with
the same branch prefix, rules, pull request and audit log as a workspace's.

```
platform ──key──▶ gitd ──token──▶ sandbox (own network, nothing mounted)
   gitd token issue URL                 git clone https://x:<token>@gitd:9420/git/<repo>.git
   gitd publish --sandbox ID BRANCH     git push origin HEAD:agents/fix
```

`./demo.sh` runs it with Docker Compose: one gitd, two tenants (acme and globex), and a
sandbox container on its own network. It shows a call without a key refused, acme's
sandbox cloning, committing and pushing with its token, that token opening nothing of
globex's, and globex seeing none of acme's tokens. CI runs it on every push.

## In production

- Serve the endpoint over TLS (`tls_cert_file`, `tls_key_file`) or on a private network
  only, and set `url` to the address sandboxes reach.
- Keep each tenant's key in a systemd credential (`LoadCredential=`); replacing the file
  replaces the key without a restart.
- A platform on another machine calls the API at the same address with the tenant's key:
  `gitd --url https://gitd.internal:9420 --key-file /run/secrets/acme …`, or
  `Gitd(url=…, key=…)` in Python, `new Gitd({ url, key })` in TypeScript.

## E2B and Modal

[`e2b_sandbox.py`](e2b_sandbox.py) and [`modal_sandbox.py`](modal_sandbox.py) do the same
from an E2B or a Modal sandbox. They need an account (`E2B_API_KEY`, or Modal's token) and a
gitd those sandboxes can reach over the internet, over TLS (`GITD_URL`, `GITD_KEY_FILE`).
They are not run in CI, which has neither.
