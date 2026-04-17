.PHONY: build lint test test-e2e clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/niklod/lazylab/internal/version.version=$(VERSION)

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/lazylab ./cmd/lazylab

lint:
	golangci-lint run

test:
	go test -race ./internal/... ./cmd/...

test-e2e:
	go test -race -tags=e2e -count=1 ./tests/e2e/...

clean:
	rm -rf bin/ dist/
