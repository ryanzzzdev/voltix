# Voltix

Voltix is a planned Linux edge controller and telemetry gateway for simulated battery energy storage systems. The project explores supervisory control, device communication, and recovery from process and network failures.

## Project documentation

- [Documentation workflow](docs/project/documentation-guide.md)
- [Roadmap and milestone progress](docs/project/roadmap.md)
- [Backlog and next work](docs/project/backlog.md)
- [Architecture and design status](docs/project/architecture.md)
- [Blockers and open questions](docs/project/blockers.md)
- Behavioural contracts: [device profile](docs/project/features/device-profile.md), [control](docs/project/features/control.md), [telemetry](docs/project/features/telemetry.md), [verification](docs/project/features/verification.md)
- [Decision records](docs/project/decisions/)
- [Work journal](docs/project/journal/)
- [Initial discussions](docs/initial-discussion/) — background proposals, not the canonical implementation specification

## Development setup

Prerequisites verified on Linux x86_64 (2026-10-12):

- **Go 1.27.2** — install the official tarball to `~/.local/go` and add `~/.local/go/bin` to `PATH`:
  ```bash
  curl -sLO https://go.dev/dl/go1.27.2.linux-amd64.tar.gz
  tar -C ~/.local -xzf go1.27.2.linux-amd64.tar.gz
  ```
- **uv** — manages the simulator's Python environment (CPython 3.13) under `sim/`.

Commands:

```bash
make build     # build bin/voltix
make test      # Go unit tests
make check     # gofmt + go vet
make sim-env   # create/update the pinned simulator environment
./bin/voltix --config configs/example.yaml
```

Simulator dependencies are pinned in `sim/pyproject.toml` and `sim/uv.lock` (pymodbus 3.15.0, pytest 9.1.1). Application code beyond the minimal entry point has not been implemented yet.

Read [AGENTS.md](AGENTS.md) and the documentation workflow before meaningful project work.
