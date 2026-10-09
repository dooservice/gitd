# gitd

The service that keeps the local copies of git repositories on a dooservice server: one
**pool** per repository, which is the only copy that talks to the network, and the
**workspaces** of each environment, which borrow their objects from the pool.

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
them all). On servers it runs as `dooservice-gitd`
([`packaging/dooservice-gitd.service`](packaging/dooservice-gitd.service)), installed and
updated by the agent.

## Release

Push a tag such as `0.2.0`: the release workflow tests, builds and attaches
`dooservice-gitd-linux-amd64` and its checksum to the GitHub release. The agent's release
picks the version it pins from there.

## Try it by hand

```sh
buf curl --http2-prior-knowledge --unix-socket /run/dooservice/gitd.sock --schema . \
  -d '{"url": "https://github.com/OCA/web"}' \
  http://localhost/gitd.v1.RepositoryService/Probe
```

Credentials travel in the request body: pass it on stdin (`-d @-`), never on the command
line.
