# gitd

The service that keeps the local copies of git repositories on a server: one **pool** per
repository, which is the only copy that talks to the network, and the **workspaces** of
each environment, which borrow their objects from the pool. Ten environments of the same
repository take the disk of one clone, and a repository is fetched once however many of
them use it.

It reads from any git host: GitHub, GitLab (nested groups included), Bitbucket, Gitea,
Forgejo, Codeberg or a self-hosted one.

It clones, updates, checks out branches and commits, handles submodules, adopts clones
that already exist, and pushes what an environment committed, each with the one
credential that opens that repository: a token or an SSH key, never on a command line.

It speaks ConnectRPC (gRPC, gRPC-Web and Connect) on a Unix socket. The contract is in
[`proto/gitd/v1/gitd.proto`](proto/gitd/v1/gitd.proto) and the design in
[`docs/DESIGN.md`](docs/DESIGN.md).

## Build

```sh
make build        # bin/gitd
make test         # go test
make lint         # buf lint, go vet, gofmt
make generate     # regenerates gen/ from the .proto
```

`make generate` needs `buf`, `protoc-gen-go` and `protoc-gen-connect-go`.

## Run

```sh
bin/gitd -config packaging/gitd.toml
```

Every configuration key has a default ([`packaging/gitd.toml`](packaging/gitd.toml) shows
them all). `-socket` sets the socket over the configuration. As a service, use
[`packaging/gitd.service`](packaging/gitd.service) (user `gitd`, socket
`/run/gitd/gitd.sock`).

SSH host keys of github.com, gitlab.com, bitbucket.org and codeberg.org are built in. For a
self-hosted GitLab, Gitea or Forgejo, list its keys' file under `known_hosts`, and when it
takes SSH on another port or user, say so under `[ssh."<host>"]` (`port = 2222`). A token is
sent with the user name its host expects (`oauth2` on gitlab.com, `x-token-auth` on
bitbucket.org, `x-access-token` elsewhere) unless the request's `https_user` names one.

## Release

Push a tag such as `0.2.0`: the release workflow tests, builds and attaches
`gitd-linux-amd64` and its checksum to the GitHub release.

## Try it by hand

```sh
buf curl --http2-prior-knowledge --unix-socket /run/gitd/gitd.sock --schema . \
  -d '{"url": "https://gitlab.com/gitlab-org/cloud-native/gitlab-operator"}' \
  http://localhost/gitd.v1.RepositoryService/Probe
```

Credentials travel in the request body: pass it on stdin (`-d @-`), never on the command
line.

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md). To report a vulnerability, do not open an issue:
follow [SECURITY.md](SECURITY.md).

## License

[GNU Affero General Public License v3.0](LICENSE).
