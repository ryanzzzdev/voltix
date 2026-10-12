GO ?= $(shell command -v go || echo $(HOME)/.local/go/bin/go)
UV ?= uv
SHELL := /bin/bash

.PHONY: all build test check sim-env sim-test integration demo demo-down native-test clean

all: build

build:
	$(GO) build -o bin/voltix ./cmd/voltix
	$(GO) build -o bin/upstream-mock ./cmd/upstream-mock

test:
	$(GO) test ./...

check:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "gofmt: files not formatted"; exit 1; }
	$(GO) vet ./...

sim-env:
	cd sim && $(UV) sync

sim-test:
	cd sim && $(UV) run pytest -q

integration:
	$(GO) test -tags=integration -v ./internal/device/ ./internal/control/

# Isolated Compose demo: field network (voltix<->sim) + upstream network
# (voltix<->mock). Host bindings are loopback-only (dispatch :8180, sim
# control :1504, mock :8081). Bounded readiness, deterministic teardown.
demo:
	docker compose -f deploy/compose.yaml up -d --build
	@echo "waiting for voltix dispatch API..."
	@for i in $$(seq 1 30); do \
		curl -sf -H "Authorization: Bearer $${VOLTIX_DISPATCH_TOKEN:-voltix-demo-dispatch}" \
			http://127.0.0.1:8180/api/status >/dev/null && break; \
		[ $$i -eq 30 ] && { echo "voltix not ready after 30s"; exit 1; }; sleep 1; done
	@for i in $$(seq 1 10); do \
		(echo > /dev/tcp/127.0.0.1/1504) 2>/dev/null && break; \
		[ $$i -eq 10 ] && { echo "sim control not ready"; exit 1; }; sleep 1; done
	@echo "ready. dispatch :8180 (auth) | status :8180/api/status | sim control :1504 | mock :8081"
	@echo "telemetry rows:  curl -s http://127.0.0.1:8081/control/count"

demo-down:
	docker compose -f deploy/compose.yaml down -v

# Native systemd --user service test (VTX-017). No root; install/remove
# are confined to ~/.config/systemd/user and ~/.local/state/voltix.
native-test:
	./scripts/native-test.sh

clean:
	rm -rf bin
