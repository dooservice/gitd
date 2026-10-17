# Contributing

## Before you start

- **Bugs**: open an issue with the version, the call you made and what it answered.
- **Changes**: for anything beyond a small fix, open an issue first and describe what you
  want to change and why. The design is in [docs/DESIGN.md](docs/DESIGN.md).
- **Vulnerabilities**: never in an issue. See [SECURITY.md](SECURITY.md).

## Making a change

You need Go (the version in `go.mod`) and git. To change the API you also need `buf`,
`protoc-gen-go` and `protoc-gen-connect-go`.

```sh
make test       # go test
make lint       # buf lint, go vet, gofmt
make generate   # after changing proto/, then commit gen/ with it
```

- Keep each pull request to one change, with its tests.
- A change to `proto/` must stay backwards compatible: add fields and methods, never
  rename or reuse them.
- Credentials never go on a command line or into a log.

## Style

Code, comments and commit messages in English. Commit messages say what the change does,
in one line: `fix: a submodule on a moved branch is fetched by its commit`.

## Sign your commits

Every commit must carry a `Signed-off-by` line, which says you wrote the change or have
the right to submit it under this project's license, as the
[Developer Certificate of Origin](https://developercertificate.org) describes. Add it with
`git commit -s`.

## Code of conduct

Everyone taking part follows the [Code of Conduct](CODE_OF_CONDUCT.md). Report unacceptable
behaviour to [soporte@apiservicesac.com](mailto:soporte@apiservicesac.com).

## License

By contributing, you agree that your contribution is licensed under the
[Apache License 2.0](LICENSE), like the rest of the repository.
