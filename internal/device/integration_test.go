//go:build integration

// VTX-007 device-integration slice: exercises the adapter against the real
// Python simulator over TCP. Run with: make integration

package device

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

type simCtl struct {
	conn net.Conn
	rd   *bufio.Reader
}

func (c *simCtl) cmd(t *testing.T, name string, value any) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"cmd": name, "value": value})
	if _, err := c.conn.Write(append(body, '\n')); err != nil {
		t.Fatalf("control write: %v", err)
	}
	line, err := c.rd.ReadBytes('\n')
	if err != nil {
		t.Fatalf("control read: %v", err)
	}
	var resp struct {
		OK    bool            `json:"ok"`
		Error string          `json:"error"`
		State json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal(line, &resp); err != nil || !resp.OK {
		t.Fatalf("control %s: %s (err=%v)", name, line, err)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// startSim launches `python -m voltix_sim` and waits for its control socket.
func startSim(t *testing.T) (invAddr, metAddr string, ctl *simCtl) {
	t.Helper()
	inv, met, cp := freePort(t), freePort(t), freePort(t)
	cmd := exec.Command("uv", "run", "python", "-m", "voltix_sim",
		"--inverter-port", fmt.Sprint(inv),
		"--meter-port", fmt.Sprint(met),
		"--control-port", fmt.Sprint(cp))
	cmd.Dir = filepath.Join("..", "..", "sim")
	var serr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &serr, &serr
	// uv spawns the python child; kill the whole group or the child is
	// orphaned holding the output pipes and Wait() below blocks forever.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start simulator: %v", err)
	}
	t.Cleanup(func() { syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); cmd.Wait() })

	var conn net.Conn
	var err error
	for i := 0; i < 300; i++ {
		conn, err = net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", cp), 100*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("control port never came up: %v\nsim output:\n%s", err, serr.String())
	}
	ctl = &simCtl{conn: conn, rd: bufio.NewReader(conn)}
	t.Cleanup(func() { conn.Close() })
	ctl.cmd(t, "ping", nil)
	return fmt.Sprintf("127.0.0.1:%d", inv), fmt.Sprintf("127.0.0.1:%d", met), ctl
}

func poll(t *testing.T, d time.Duration, f func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if f() {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return f()
}

func TestDeviceIntegrationSlice(t *testing.T) {
	invAddr, metAddr, ctl := startSim(t)
	inv := NewEndpoint(invAddr, 250*time.Millisecond)
	met := NewEndpoint(metAddr, 250*time.Millisecond)
	defer inv.Close()
	defer met.Close()

	// --- discovery and initial readings ------------------------------------
	models, err := inv.Discover(context.Background(), InverterModels)
	if err != nil || len(models) != 4 {
		t.Fatalf("inverter discovery: %v %v", models, err)
	}
	if models, err = met.Discover(context.Background(), MeterModels); err != nil || len(models) != 2 {
		t.Fatalf("meter discovery: %v %v", models, err)
	}
	r, err := inv.ReadInverter(context.Background())
	if err != nil || r.SoCPct < 49 || r.SoCPct > 51 {
		t.Fatalf("initial reading %+v err %v", r, err)
	}

	// Changing simulated state must show up on the wire.
	ctl.cmd(t, "set_soc_pct", 60.0)
	ctl.cmd(t, "set_baseline_kw", 10.0)
	r, _ = inv.ReadInverter(context.Background())
	if r.SoCPct != 60.0 {
		t.Fatalf("SoC change not observed: %v", r.SoCPct)
	}
	m, err := met.ReadMeter(context.Background())
	if err != nil || m.Watts != 10000 {
		t.Fatalf("baseline export not observed: %+v err %v", m, err)
	}

	// --- accepted vs measured power, positive and negative ------------------
	if err := inv.SetPowerTargetW(context.Background(), 250000); err != nil {
		t.Fatalf("discharge write: %v", err)
	}
	if !poll(t, 3*time.Second, func() bool {
		r, err = inv.ReadInverter(context.Background())
		return err == nil && r.Watts == 250000
	}) {
		t.Fatalf("measured output never reached +250 kW (last %+v)", r)
	}

	// Wire cross-check: the applied setpoint window must match the golden
	// write-target registers independently of the sim's physics.
	win, err := inv.readRegs(context.Background(), AddrCtlWSet, 4)
	if err != nil {
		t.Fatal(err)
	}
	if win[0] != 1 || win[1] != 1 || win[2] != 0x0003 || win[3] != 0xD090 {
		t.Fatalf("WSet window %v != golden [1 1 0003 D090]", win)
	}

	if err := inv.SetPowerTargetW(context.Background(), -250000); err != nil {
		t.Fatalf("charge write: %v", err)
	}
	lastWrite := time.Now() // the device's 10 s deadline runs from this accept
	if !poll(t, 4*time.Second, func() bool {
		r, err = inv.ReadInverter(context.Background())
		return err == nil && r.Watts == -250000
	}) {
		t.Fatalf("measured output never reached -250 kW (last %+v)", r)
	}

	// --- independent command watchdog ---------------------------------------
	// Keep polling (reads must not refresh the 10 s deadline which runs from
	// lastWrite, not from the polls); before expiry the target still tracks;
	// after expiry the device ramps to zero and sets Evt1 bit0.
	for time.Now().Before(lastWrite.Add(8500 * time.Millisecond)) {
		if r, err = inv.ReadInverter(context.Background()); err != nil || r.Watts != -250000 {
			t.Fatalf("poll broke tracking before watchdog: %+v %v", r, err)
		}
		time.Sleep(400 * time.Millisecond)
	}
	if !poll(t, 6*time.Second, func() bool {
		r, err = inv.ReadInverter(context.Background())
		return err == nil && r.Watts == 0 && r.Evt1&1 == 1
	}) {
		t.Fatalf("watchdog did not trip to zero: %+v err %v", r, err)
	}

	// --- bounded failure and recovery ----------------------------------------
	ctl.cmd(t, "set_delay_ms", 400)
	start := time.Now()
	if _, err = met.ReadMeter(context.Background()); !errors.Is(err, ErrTimeout) {
		t.Fatalf("delayed reply: want ErrTimeout, got %v", err)
	}
	// Two bounded attempts: request timeout -> reconnect -> request timeout.
	if el := time.Since(start); el > 1200*time.Millisecond {
		t.Fatalf("delayed reply unbounded: %v", el)
	}
	ctl.cmd(t, "set_delay_ms", 0)

	ctl.cmd(t, "set_online", false)
	start = time.Now()
	if _, err = met.ReadMeter(context.Background()); err == nil {
		t.Fatal("offline device must error")
	}
	if el := time.Since(start); el > 1200*time.Millisecond {
		t.Fatalf("offline reply unbounded: %v", el)
	}
	ctl.cmd(t, "set_online", true)
	if m, err = met.ReadMeter(context.Background()); err != nil {
		t.Fatalf("no recovery after online restore: %v", err)
	}

	ctl.cmd(t, "set_corrupt", true)
	if _, err = inv.ReadInverter(context.Background()); !errors.Is(err, ErrInvalidData) {
		t.Fatalf("corrupt values: want ErrInvalidData, got %v", err)
	}
	ctl.cmd(t, "set_corrupt", false)
}
