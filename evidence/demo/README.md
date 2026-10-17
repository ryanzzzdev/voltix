# Terminal demonstration recordings

Three distinct artifacts live here — they serve different purposes:

| Artifact | What it is | Use it for |
|---|---|---|
| `demo.typescript` + `demo.timing` | util-linux `script` capture of `scripts/demo-session.sh` — linear terminal output, replayable anywhere | Auditable evidence: text, diff-friendly, no extra tooling |
| `demo-watch.cast` | asciinema v2 capture of the three-pane zellij layout running `scripts/demo-runner.sh` | Watching components react *simultaneously*; plays with `asciinema play` |
| `demo-watch.gif` | `agg` render of the cast (1281×843, ~565 KB) | Embedding in a README / slides |

The typescript is the canonical artifact. The cast/GIF are convenience
renderings — regenerate them rather than editing by hand.

## Playback: typescript

```bash
scriptreplay -t evidence/demo/demo.timing -d 1 evidence/demo/demo.typescript
```

(`scriptreplay` is part of util-linux; `-d` sets the speed divisor.)

## Live three-pane demo

```bash
make demo-watch        # or: ./scripts/demo-watch.sh
```

Opens a zellij layout (`scripts/demo-watch.kdl`): controller logs
top-left, simulator physics (1 Hz refresh via `scripts/sim-state.sh`)
top-right, and an operator pane with `scripts/demo-cheatsheet.txt` at
the bottom. Starts the Compose stack if it isn't already up. Requires
`zellij` on PATH — a single static binary; see
https://github.com/zellij-org/zellij/releases.

`./scripts/demo-watch.sh --runner` is the scripted variant: it runs the
full narrative automatically and kills the session when done. That is
the mode used for the cast/GIF recordings.

## Regenerating the cast/GIF

```bash
make demo   # bring the stack up first for a clean capture
asciinema rec --cols 150 --rows 42 -i 2 \
  -c "./scripts/demo-watch.sh --runner" evidence/demo/demo-watch.cast
agg --font-size 14 --speed 1.3 \
  evidence/demo/demo-watch.cast evidence/demo/demo-watch.gif
```

(`asciinema` via `uv tool install asciinema`; `agg` is a single binary
from https://github.com/asciinema/agg/releases.)

## Contents

Recorded 2026-10-12 against the Compose demo via `scripts/demo-session.sh`; cast/GIF recorded 2026-10-17.

1. `make demo` — isolated stack bring-up with bounded readiness.
2. Baseline STANDBY, devices healthy.
3. Dispatch −200 kW charge, 25 s lease → accepted.
4. ACTIVE with requested/constrained/acknowledged/measured all −200 kW.
5. Field-link fault injected via sim control (`set_online=false`) →
   loop reports DEGRADED / InverterOK=false.
6. Independent fallback: sim watchdog_expired, output ramped to 0.
7. Link recovery → STANDBY, intent dropped, no jump.
8. Telemetry outage (mock pause) → queue depth grows, loop unaffected.
9. Resume → backlog drains to the deduplicating receiver.
10. `make demo-down` — all containers/networks/volumes removed.
