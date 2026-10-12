**This should definitely be a single Git repository (a monorepo).**

For an engineering portfolio project, splitting this into multiple repositories would create unnecessary maintenance friction and weaken your presentation to hiring managers.

Here is why a single repository is the superior choice, along with how to structure it cleanly:

---

### 1. The "One-Clone" Rule for Hiring Managers

Recruiters and engineers reviewing your application will only click **one link** from your resume: `[github.com/ryanzhang/voltix](https://github.com/ryanzhang/voltix)`.

* **If it’s multiple repos:** A hiring manager has to clone `voltix-daemon`, then find `voltix-mock-inverter`, then figure out `voltix-test-harness`, and link local network ports manually. In reality, almost nobody will take the time to do that.
* **If it’s a single repo:** They clone one repo, run `make demo`, and the entire virtual site (controller + mock inverter + test runner) spins up immediately. Your README has one unified place for the architecture diagram, the GIF demo, and the documentation.

---

### 2. Technical Advantages of a Single Repo

* **Atomic Protocol Synchronization:** When you add SunSpec Model 124 registers or adjust the 10-second watchdog timeout in the mock inverter, you update the Go client and the simulator in the **exact same Git commit**. You never suffer from version drift between the simulator and the controller.
* **Seamless Local Orchestration:** `docker-compose.yml` can build the controller binary and spin up the mock containers using relative paths without submodule headaches.
* **Single CI Pipeline:** A single GitHub Actions workflow can compile the Go binary, spin up the simulator, execute the four fault-injection tests (`run_fault_tests.sh`), and verify that all integration tests pass on every pull request.

---

### 3. Recommended Repository Layout

A single repository does not mean messy code. Using standard Go and systems engineering conventions, the layout separates the production daemon from the test fixtures:

```text
voltix/
├── cmd/
│   └── voltix/                   # Main entry point for the edge daemon
│       └── main.go
│
├── internal/                     # Production Go code (private packages)
│   ├── driver/                   # Hardware Abstraction Layer & SunSpec Modbus TCP client
│   │   ├── modbus.go
│   │   └── sunspec.go
│   ├── engine/                   # 1 Hz supervisory control loop & safety limits
│   │   ├── loop.go
│   │   └── limits.go
│   ├── queue/                    # SQLite WAL buffer & sequence tracker
│   │   ├── sqlite.go
│   │   └── queue.go
│   └── syncer/                   # Upstream HTTP/JSON telemetry syncer with retries
│       └── syncer.go
│
├── sim/                          # Test fixtures & simulators
│   ├── inverter/                 # Python/pymodbus mock BESS with 10s watchdog
│   │   ├── Dockerfile
│   │   └── server.py
│   └── cloud/                    # Minimal mock dispatcher & telemetry receiver
│       ├── Dockerfile
│       └── server.go
│
├── deploy/                       # Linux OS packaging & deployment
│   ├── systemd/
│   │   └── voltix.service        # Production systemd unit (User=voltix, cgroups)
│   └── docker-compose.yml        # Multi-bridge network topology (WAN vs LAN)
│
├── scripts/                      # Automated fault-injection & demo harnesses
│   ├── run_fault_tests.sh        # Executes Tests A, B, C1, C2, and D
│   └── demo.sh                   # Launches split-screen tmux demo
│
├── Makefile                      # make build, make test, make sim, make demo
├── README.md                     # Overview, architecture diagram, 3-min GIF demo
├── go.mod
└── go.sum

```

### Summary

Keep everything in **`[github.com/ryanzhang/voltix](https://github.com/ryanzhang/voltix)`**. It provides the cleanest development workflow for you and delivers a cohesive, one-click evaluation experience for interviewers.