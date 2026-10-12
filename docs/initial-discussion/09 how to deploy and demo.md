
> I forgot to ask. How will I deploy the system (after it is finished) for demo? For a traditional software with UI, it needs to be deployed somehwere on cloud and UI accessed by browser or mobile app. How will this system work? My native understanding is it will be "deployed" to a linux system, and demostrated in that linux system command line, is that right? If so then does it mean no more cloud or what so ever, it might just be my computer (ubuntu desktop) or any other computer with linux (aws ec2 linux instance)?

---

**Your intuition is 100% correct.**

For embedded systems, edge controllers, and infrastructure engineering, there is no customer-facing web app or mobile UI to host on the public internet. The "product" is a **Linux daemon**, and the primary demonstration interface is the **Linux terminal, system logs, and process metrics**.

Hiring managers at Tesla, Fluence, and similar companies do not evaluate edge engineers on CSS or frontend hosting. They evaluate **how comfortably and defensively you operate in a Linux environment**: managing `systemd`, debugging sockets, inspecting logs with `journalctl`, and handling process crashes.

Here is exactly how this deployment and demonstration work in practice:

---

### 1. Where Does It Physically Run?

You do **not** need a complex multi-cloud deployment. You have three practical options:

* **Option A: Your Local Computer (Ubuntu Desktop / WSL2 / Linux VM) — *Recommended for Building***
* Everything runs locally on one machine.
* You use **Docker Compose** or **Linux network namespaces** to simulate the three distinct physical machines (the Edge Controller, the Inverter, and the Upstream Cloud) on isolated virtual subnets.


* **Option B: A Single Cloud Linux Instance (AWS EC2 / Hetzner / DigitalOcean $5/mo instance)**
* A dedicated Ubuntu server where your repo is cloned and running.
* *Advantage:* Clean, isolated environment free from desktop clutter, and you can easily share terminal recordings or live SSH sessions if an interviewer ever asks.


* **Option C: An ARM Development Board (e.g., Raspberry Pi 4 / CM4) — *Optional Bonus***
* Cross-compile your Go binary for `linux/arm64` (`GOOS=linux GOARCH=arm64 go build`), copy it to a Raspberry Pi on your local network, and run it under `systemd`.
* *Interview Impact:* Showing that your binary runs natively on a resource-constrained ARM edge device directly matches the physical realities of Tesla’s industrial hardware.



---

### 2. How the Topology Works on a Single Linux Machine

Even though everything runs on one computer, you simulate the physical network topology using Docker networks:

```text
       [Your Ubuntu Host / AWS EC2]
                     |
       +-------------+-------------+
       |                           |
[WAN Bridge: 172.20.0.0/24]   [LAN Bridge: 192.168.1.0/24]
       |                           |
       v                           v
+---------------+           +---------------+
| Upstream Mock |           | Mock Inverter |
| (Cloud API)   |           | (Modbus TCP)  |
+---------------+           +---------------+
       ^                           ^
       |                           |
       +-------------+-------------+
                     |
         +-----------------------+
         | bess-edge-controller  |
         | (Native / Container)  |
         +-----------------------+

```

Because the **WAN (cloud)** and **LAN (inverter)** are separated, you can sever the cloud connection (via `iptables` or `tc netem`) without affecting the Modbus connection to the battery.

---

### 3. How to Present the Demo to Recruiters & Interviewers

Since recruiters cannot just "open a link", you deliver the project through three concrete assets:

#### Asset 1: The 3-Minute Terminal Recording (Embedded in GitHub README)

In edge/systems engineering, the gold standard portfolio artifact is a clean terminal screencast (using tools like **Asciinema**, **VHS**, or a screen-recorded GIF/video).

A typical demo layout uses a **split-screen terminal** (via `tmux` or multiple terminal windows):

* **Pane 1 (Top Left — Edge Controller):** Live logs showing the 1 Hz supervisory loop, battery kW regulation, and state machine (`ACTIVE`, `STANDBY`).
* **Pane 2 (Top Right — Mock Inverter):** Inverter power output, SoC updating, and its independent 10-second watchdog.
* **Pane 3 (Bottom — Test Runner):** You run `./scripts/run_fault_tests.sh`. The audience watches the automated script sever the cloud connection, kill the controller process, and restore the link—while watching Panes 1 and 2 react deterministically in real time.

#### Asset 2: One-Command Reproducibility (`make demo`)

Your GitHub repository should allow anyone with Docker and Linux to clone it and run:

```bash
git clone https://github.com/your-username/bess-edge-controller
cd bess-edge-controller
make demo

```

This single command spins up the mock inverter, starts the controller, runs the automated failure injection tests, and prints a clean report indicating all five test scenarios passed.

#### Asset 3 (Optional): A Lightweight Terminal UI (TUI)

If you want visual polish without building a web frontend, you can build a terminal dashboard using Go's popular TUI libraries (like `bubbletea` or `termui`). It displays real-time ASCII gauges for:

* Battery State of Charge (SoC: `[████████░░] 82%`)
* Active Power Setpoint (`+450 kW`)
* Watchdog Countdowns (`Cloud Lease: 24s`, `Inverter Watchdog: 8s`)
* Telemetry Queue Backlog (`Queued: 0 records`)

---

### Summary

* You do **not** need a web frontend or cloud hosting.
* The system is deployed natively to a **Linux environment** (Ubuntu desktop, VM, EC2 instance, or Raspberry Pi).
* The demo is delivered via a **reproducible automated test script** and an **embedded terminal recording** in your repository README.