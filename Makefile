VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build generate lint test clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/gitd ./cmd/gitd

generate:
	buf generate

lint:
	buf lint
	go vet ./...
	test -z "$$(gofmt -l .)"

test:
	go test ./...

clean:
	rm -rf bin
