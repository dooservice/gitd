# gitd

A service that keeps the local copies of git repositories on a server: one **pool** per
repository, which is the only copy that talks to the network, and **workspaces** that
borrow their objects from it. Ten workspaces of the same repository take the disk of one
clone, and a repository is fetched once however many of them use it.

It reads from any git host: GitHub, GitLab (nested groups included), Bitbucket, Gitea,
Forgejo, Codeberg or a self-hosted one, over HTTPS or SSH, on any port.

It clones, updates, checks out branches and commits, rolls back, handles submodules, adopts
clones that already exist, and pushes what was committed in a workspace, each with the one
credential that opens that repository: a token or an SSH key, never on a command line nor
on disk.

It speaks ConnectRPC (gRPC, gRPC-Web and Connect) on a Unix socket. The contract is
[`proto/gitd/v1/gitd.proto`](proto/gitd/v1/gitd.proto) and the design
[`docs/DESIGN.md`](docs/DESIGN.md).

## Build

You need Go (the version in `go.mod`) and git.

```sh
make build        # bin/gitd
make test         # go test
make lint         # buf lint, go vet, gofmt
make generate     # regenerates gen/ from the .proto
```

`make generate` also needs `buf`, `protoc-gen-go` and `protoc-gen-connect-go`.

## Run

```sh
bin/gitd -config /etc/gitd/gitd.toml     # the default path; a missing file is the defaults
bin/gitd -socket /tmp/gitd.sock          # -socket wins over the configuration's
bin/gitd -version
```

As a service, [`packaging/gitd.service`](packaging/gitd.service) runs it as the user
`gitd`, on `/run/gitd/gitd.sock`, with its state in `/var/lib/gitd`.

## Configure

Every key has a default; [`packaging/gitd.toml`](packaging/gitd.toml) shows them all.

| Key | Default | What it is |
|---|---|---|
| `socket` | `/run/gitd/gitd.sock` | The Unix socket gitd answers on |
| `state` | `/var/lib/gitd` | Pools, lock files and the SSH host keys |
| `roots` | `["/var/lib/gitd/workspaces"]` | The only directories a workspace may be made in |
| `git` | `git` | The git binary |
| `metrics` | empty (off) | Where Prometheus metrics are served, e.g. `127.0.0.1:9419` |
| `known_hosts` | `[]` | Files of SSH host keys trusted besides the built-in ones |
| `[hosts."<host>"]` | — | `https_port`, `ssh_port`, `ssh_user` of a host not on the usual ones |
| `[limits]` | 4 fetches, 8 writes, 32 queued, 5m wait, 30m per call | How much runs at once and for how long |
| `[housekeeping]` | every 6h, retire after 30 days | Pool maintenance and retirement |

### Git hosts

- **SSH host keys** of github.com, gitlab.com, bitbucket.org and codeberg.org are built in.
  For a self-hosted GitLab, Gitea or Forgejo, put its keys in a file listed in
  `known_hosts`, checked against what its admin publishes (`ssh-keyscan` only shows what
  answers). Nothing is trusted on first use.
- **Ports**: a host reached on other ports than HTTPS on 443 and SSH on 22 says so:

  ```toml
  [hosts."git.example.com"]
  https_port = 3000
  ssh_port = 2222       # its key goes in known_hosts as [git.example.com]:2222 ssh-ed25519 …
  ```

  Every form of a repository's address — `https://git.example.com:3000/acme/web`,
  `ssh://git@git.example.com:2222/acme/web.git`, `git@git.example.com:acme/web` — is then
  the same repository, with one pool.
- **Tokens** are sent with the user name their host expects (`oauth2` on gitlab.com,
  `x-token-auth` on bitbucket.org, `x-access-token` elsewhere) unless the request's
  `https_user` names one.

## Try it by hand

```sh
buf curl --http2-prior-knowledge --unix-socket /run/gitd/gitd.sock --schema . \
  -d '{"url": "https://gitlab.com/gitlab-org/cloud-native/gitlab-operator"}' \
  http://localhost/gitd.v1.RepositoryService/Probe

echo '{"url": "git@github.com:acme/private.git", "path": "/var/lib/gitd/workspaces/acme/app",
       "credentials": [{"url_prefix": "https://github.com/acme", "https_token": "…"}]}' |
buf curl --http2-prior-knowledge --unix-socket /run/gitd/gitd.sock --schema . -d @- \
  http://localhost/gitd.v1.WorkspaceService/Prepare
```

Credentials travel in the request body: pass it on stdin (`-d @-`), never on the command
line.

## Release

Push a tag such as `0.5.0`: the release workflow tests, builds and attaches
`gitd-linux-amd64` and its checksum to the GitHub release.

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md). To report a vulnerability, do not open an issue:
follow [SECURITY.md](SECURITY.md).

## License

[GNU Affero General Public License v3.0](LICENSE).
