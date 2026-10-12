GO ?= go
UV ?= uv

.PHONY: all build test check sim-env clean

all: build

build:
	$(GO) build -o bin/voltix ./cmd/voltix

test:
	$(GO) test ./...

check:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "gofmt: files not formatted"; exit 1; }
	$(GO) vet ./...

sim-env:
	cd sim && $(UV) sync

clean:
	rm -rf bin
