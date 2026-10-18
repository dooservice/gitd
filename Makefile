VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# The public GitHub App people sign in through, from the environment: GITD_LOGIN_CLIENT_ID and
# GITD_LOGIN_APP (the repository's variables in CI).
CONFIG := github.com/dooservice/gitd/internal/config
LDFLAGS := -s -w -X main.version=$(VERSION) -X $(CONFIG).loginClientID=$(GITD_LOGIN_CLIENT_ID) -X $(CONFIG).loginApp=$(GITD_LOGIN_APP)

.PHONY: build generate lint test clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/gitd ./cmd/gitd
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/gitd-mount ./cmd/gitd-mount
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/gitd-csi ./cmd/gitd-csi

# gen/ for Go, and the contract the Python client reads at run time, as indented JSON.
generate:
	buf generate
	buf build --exclude-source-info --as-file-descriptor-set -o -#format=json | python3 -m json.tool --indent 2 > sdk/python/src/gitd/contract.json

lint:
	buf lint
	go vet ./...
	test -z "$$(gofmt -l .)"

test:
	go test ./...

clean:
	rm -rf bin
