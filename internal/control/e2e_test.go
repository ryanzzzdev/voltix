//go:build integration

// VTX-011 end-to-end control verification: drives the real `voltix`
// binary and Python simulator over TCP. Run with: make integration

package control_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const ctlToken = "e2e-token-0123456789abcdef"
const telToken = "e2e-tel-token-0123456789abcde"
const ctlID = "voltix-e2e"

// ---- sim control socket ---------------------------------------------------

type simCtl struct {
	conn net.Conn
	rd   *bufio.Reader
}

func (c *simCtl) call(t *testing.T, name string, value any) map[string]any {
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
		OK    bool           `json:"ok"`
		Error string         `json:"error"`
		State map[string]any `json:"state"`
	}
	if err := json.Unmarshal(line, &resp); err != nil || !resp.OK {
		t.Fatalf("control %s: %s (err=%v)", name, line, err)
	}
	return resp.State
}
func (c *simCtl) set(t *testing.T, name string, value any) { c.call(t, name, value) }
func (c *simCtl) state(t *testing.T) map[string]any {
	return c.call(t, "get_state", nil)
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

func startSim(t *testing.T) (invPort, metPort int, ctl *simCtl) {
	t.Helper()
	invPort, metPort, cp := freePort(t), freePort(t), freePort(t)
	cmd := exec.Command("uv", "run", "python", "-m", "voltix_sim",
		"--inverter-port", fmt.Sprint(invPort),
		"--meter-port", fmt.Sprint(metPort),
		"--control-port", fmt.Sprint(cp))
	cmd.Dir = filepath.Join("..", "..", "sim")
	var serr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &serr, &serr
	// uv spawns the python child; kill the whole group or the child is
	// orphaned holding the output pipes and Wait() below blocks forever.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sim: %v", err)
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
		t.Fatalf("sim control port never came up: %v\nsim output:\n%s", err, serr.String())
	}
	ctl = &simCtl{conn: conn, rd: bufio.NewReader(conn)}
	t.Cleanup(func() { conn.Close() })
	ctl.set(t, "ping", nil)
	return invPort, metPort, ctl
}

// ---- voltix process + API -------------------------------------------------

type voltixProc struct {
	cmd    *exec.Cmd
	apiURL string
	bootID string
}

func buildVoltix(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "voltix")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/voltix")
	cmd.Dir = filepath.Join("..", "..")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build: %s %v", out, err)
	}
	return bin
}

func voltixConfig(invPort, metPort, apiPort int, extra string) string {
	return fmt.Sprintf(`controller_id: %s
devices:
  inverter: {host: 127.0.0.1, port: %d}
  meter: {host: 127.0.0.1, port: %d}
limits:
  soc_min_pct: 5
  soc_max_pct: 95
  site_export_max_kw: 400
  charge_max_kw: 500
  discharge_max_kw: 500
  normal_slew_kw_per_s: 50
timing:
  cycle_period_ms: 200
  read_deadline_ms: 200
  max_input_age_s: 2
  max_loop_gap_s: 2.5
upstream:
  dispatch_listen: 127.0.0.1:%d
%s`, ctlID, invPort, metPort, apiPort, extra)
}

func startVoltixCfg(t *testing.T, bin string, cfg string) *voltixProc {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "voltix.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	apiPortStr := strings.Split(cfg, "dispatch_listen: 127.0.0.1:")[1]
	apiPortStr = strings.Split(apiPortStr, "\n")[0]
	cmd := exec.Command(bin, "-config", cfgPath)
	cmd.Env = append(os.Environ(),
		"VOLTIX_DISPATCH_TOKEN="+ctlToken,
		"VOLTIX_TELEMETRY_TOKEN="+telToken)
	var verr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &verr, &verr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start voltix: %v", err)
	}
	p := &voltixProc{cmd: cmd, apiURL: "http://127.0.0.1:" + apiPortStr}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	// Wait for the API and read the boot session id.
	var st map[string]any
	for i := 0; i < 100; i++ {
		code, body := api(t, http.MethodGet, p.apiURL+"/api/status", nil)
		if code == 200 {
			st = body
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if st == nil {
		t.Fatalf("voltix API never came up\nvoltix output:\n%s", verr.String())
	}
	p.bootID = st["boot_session_id"].(string)
	if p.bootID == "" {
		t.Fatal("empty boot_session_id")
	}
	return p
}

// startVoltix keeps the original signature: telemetry enabled with a
// per-test state dir and no reachable upstream (uploads fail harmlessly).
func startVoltix(t *testing.T, bin string, invPort, metPort, apiPort int) *voltixProc {
	t.Helper()
	dir := t.TempDir()
	extra := "telemetry:\n  state_dir: " + dir + "\n"
	return startVoltixCfg(t, bin, voltixConfig(invPort, metPort, apiPort, extra))
}

func api(t *testing.T, method, url string, body []byte) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+ctlToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1, nil
	}
	defer resp.Body.Close()
	var v map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&v)
	return resp.StatusCode, v
}

func postDispatch(t *testing.T, p *voltixProc, rev int64, kw float64,
	validFor time.Duration) (int, map[string]any) {
	t.Helper()
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]any{
		"controller_id":   ctlID,
		"boot_session_id": p.bootID,
		"command_id":      fmt.Sprintf("eeeeeeee-0000-4000-8000-%012d", rev),
		"revision":        rev,
		"issued_at":       now.Format(time.RFC3339Nano),
		"valid_until":     now.Add(validFor).Format(time.RFC3339Nano),
		"target_kw":       kw,
	})
	return api(t, http.MethodPost, p.apiURL+"/api/dispatch", body)
}

func status(t *testing.T, p *voltixProc) map[string]any {
	t.Helper()
	code, v := api(t, http.MethodGet, p.apiURL+"/api/status", nil)
	if code != 200 {
		t.Fatalf("status: %d %v", code, v)
	}
	return v
}

func loopState(t *testing.T, p *voltixProc) string {
	t.Helper()
	if l, ok := status(t, p)["loop"].(map[string]any); ok {
		if s, ok := l["State"].(string); ok {
			return s
		}
	}
	return ""
}

func constrained(t *testing.T, p *voltixProc) float64 {
	t.Helper()
	l, _ := status(t, p)["loop"].(map[string]any)
	v, _ := l["ConstrainedW"].(float64)
	return v
}

func outputKW(st map[string]any) float64 { v, _ := st["output_kw"].(float64); return v }

func toStrings(v any) []string {
	var out []string
	if arr, ok := v.([]any); ok {
		for _, x := range arr {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func poll(d time.Duration, f func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if f() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return f()
}

// ---- the end-to-end scenario ----------------------------------------------

func TestControlEndToEnd(t *testing.T) {
	invPort, metPort, ctl := startSim(t)
	apiPort := freePort(t)
	bin := buildVoltix(t)
	p := startVoltix(t, bin, invPort, metPort, apiPort)

	// 1. Standby: no dispatch, output stays at/near zero.
	if !poll(3*time.Second, func() bool { return loopState(t, p) == "STANDBY" }) {
		t.Fatalf("never reached STANDBY: %s", loopState(t, p))
	}
	if got := outputKW(ctl.state(t)); got != 0 {
		t.Fatalf("standby output %v kW", got)
	}

	// 2. Valid discharge: +200 kW accepted, ramps at <=50 kW/s to target.
	code, _ := postDispatch(t, p, 1, 200, 30*time.Second)
	if code != 202 {
		t.Fatalf("dispatch accept: %d", code)
	}
	time.Sleep(1200 * time.Millisecond) // ~2-3 cycles in, must NOT be at target yet
	if st := ctl.state(t); outputKW(st) > 150 {
		t.Fatalf("output jumped to %v kW — slew not applied", outputKW(st))
	}
	if !poll(8*time.Second, func() bool {
		return outputKW(ctl.state(t)) >= 195 && loopState(t, p) == "ACTIVE"
	}) {
		t.Fatalf("discharge never reached target: %+v", ctl.state(t))
	}

	// 3. Valid charge: -100 kW crosses zero and settles negative.
	code, _ = postDispatch(t, p, 2, -100, 30*time.Second)
	if code != 202 {
		t.Fatalf("charge accept: %d", code)
	}
	if !poll(10*time.Second, func() bool { return outputKW(ctl.state(t)) <= -95 }) {
		t.Fatalf("charge never reached -100 kW: %+v", ctl.state(t))
	}

	// 4. Expiry fallback: +100 kW for 4 s. Fallback start (DEGRADED) must be
	//    distinguishable from time-to-zero.
	code, _ = postDispatch(t, p, 3, 100, 4*time.Second)
	if code != 202 {
		t.Fatalf("expiry accept: %d", code)
	}
	poll(3*time.Second, func() bool { return outputKW(ctl.state(t)) > 30 })
	// Fallback start is the intent dropping (dispatch_active -> false); the
	// DEGRADED state itself lasts one cycle and may race the poller.
	var fallbackAt, zeroAt time.Time
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) && (fallbackAt.IsZero() || zeroAt.IsZero()) {
		st := status(t, p)
		if fallbackAt.IsZero() && st["dispatch_active"] != true {
			fallbackAt = time.Now()
		}
		if zeroAt.IsZero() && !fallbackAt.IsZero() && outputKW(ctl.state(t)) < 5 {
			zeroAt = time.Now()
		}
		time.Sleep(100 * time.Millisecond)
	}
	if fallbackAt.IsZero() {
		t.Fatal("intent never expired")
	}
	if zeroAt.IsZero() {
		t.Fatal("output never fell back to zero")
	}
	if zeroAt.Sub(fallbackAt) < 300*time.Millisecond {
		t.Fatalf("fallback start and zero indistinguishable: %v", zeroAt.Sub(fallbackAt))
	}

	// 5. SoC floor: at 4% discharge is prohibited even with fresh intent.
	ctl.set(t, "set_soc_pct", 4.0)
	code, _ = postDispatch(t, p, 4, 80, 8*time.Second)
	if code != 202 {
		t.Fatalf("SoC dispatch: %d", code)
	}
	time.Sleep(1500 * time.Millisecond)
	if got := constrained(t, p); got != 0 {
		t.Fatalf("SoC floor: constrained %v W", got)
	}
	ctl.set(t, "set_soc_pct", 50.0)

	// 5b. Infeasible baseline: site export alone exceeds the limit while
	//     SoC sits at the ceiling (charging prohibited) — the only feasible
	//     request is zero, reported as infeasible.
	ctl.set(t, "set_soc_pct", 95.0)
	ctl.set(t, "set_baseline_kw", 450.0)
	code, _ = postDispatch(t, p, 5, -50, 8*time.Second) // charge to reduce export
	if code != 202 {
		t.Fatalf("infeasible dispatch: %d", code)
	}
	if !poll(4*time.Second, func() bool {
		l, _ := status(t, p)["loop"].(map[string]any)
		for _, r := range toStrings(l["Reasons"]) {
			if r == "SITE_LIMIT_INFEASIBLE" {
				return true
			}
		}
		return false
	}) {
		t.Fatalf("infeasible not reported: %+v", status(t, p)["loop"])
	}
	ctl.set(t, "set_soc_pct", 50.0)
	ctl.set(t, "set_baseline_kw", 0.0)

	// 6. Device fault inhibits nonzero commands.
	ctl.set(t, "set_fault", true)
	if !poll(4*time.Second, func() bool { return loopState(t, p) == "FAULT" }) {
		t.Fatalf("never FAULT: %s", loopState(t, p))
	}
	if !poll(4*time.Second, func() bool { return outputKW(ctl.state(t)) < 5 }) {
		t.Fatalf("fault output: %+v", ctl.state(t))
	}
	ctl.set(t, "set_fault", false)

	// 7. Stale/invalid readings degrade.
	ctl.set(t, "set_online", false)
	if !poll(5*time.Second, func() bool { return loopState(t, p) == "DEGRADED" }) {
		t.Fatalf("offline not DEGRADED: %s", loopState(t, p))
	}
	ctl.set(t, "set_online", true)

	// 8. Old-session replay after restart is rejected.
	oldBody, _ := json.Marshal(map[string]any{
		"controller_id": ctlID, "boot_session_id": p.bootID,
		"command_id": "eeeeeeee-0000-4000-8000-000000000099",
		"revision":   99, "issued_at": time.Now().UTC().Format(time.RFC3339Nano),
		"valid_until": time.Now().Add(30 * time.Second).UTC().Format(time.RFC3339Nano),
		"target_kw":   50.0,
	})
	p.cmd.Process.Kill()
	p.cmd.Wait()
	p = startVoltix(t, bin, invPort, metPort, apiPort)
	if code, _ := api(t, http.MethodPost, p.apiURL+"/api/dispatch", oldBody); code != 409 {
		t.Fatalf("old-session replay: want 409, got %d", code)
	}
	// And a fresh command with revision 1 on the NEW session is accepted —
	// revision state died with the old boot.
	if code, _ := postDispatch(t, p, 1, 40, 10*time.Second); code != 202 {
		t.Fatalf("new session rev1: %d", code)
	}

	// 9. Malformed / auth failures.
	code, _ = api(t, http.MethodPost, p.apiURL+"/api/dispatch",
		[]byte(`{"controller_id":"x","nope":1}`))
	if code != 400 {
		t.Fatalf("unknown field: want 400, got %d", code)
	}
	req, _ := http.NewRequest(http.MethodPost, p.apiURL+"/api/dispatch",
		bytes.NewReader([]byte(`{}`)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 401 {
		t.Fatalf("no-token: %v %v", resp.StatusCode, err)
	}
	resp.Body.Close()

	// 10. Delayed device responses mark output uncertain, then recover.
	ctl.set(t, "set_delay_ms", 400) // > 200 ms endpoint deadline
	if !poll(5*time.Second, func() bool {
		l, _ := status(t, p)["loop"].(map[string]any)
		ack, present := l["Acknowledged"]
		return present && ack == nil
	}) {
		t.Fatal("delayed write never marked acknowledged=null")
	}
	ctl.set(t, "set_delay_ms", 0)
	if !poll(5*time.Second, func() bool {
		l, _ := status(t, p)["loop"].(map[string]any)
		return l["Acknowledged"] != nil
	}) {
		t.Fatal("ack did not recover after delay cleared")
	}

	// 11. Prolonged loop pause (SIGSTOP > gap bound) degrades and drops intent.
	if code, _ := postDispatch(t, p, 2, 60, 30*time.Second); code != 202 {
		t.Fatalf("pre-pause dispatch: %d", code)
	}
	if err := p.cmd.Process.Signal(syscall.SIGSTOP); err != nil {
		t.Fatalf("SIGSTOP: %v", err)
	}
	time.Sleep(3 * time.Second)
	p.cmd.Process.Signal(syscall.SIGCONT)
	// The gap invalidates the intent: the single DEGRADED cycle races the
	// poller, so assert on the durable evidence — the dropped lease.
	if !poll(5*time.Second, func() bool {
		return status(t, p)["dispatch_active"] == false
	}) {
		t.Fatal("intent survived the pause — must be invalidated")
	}

	// 12. No recovery jump to an old target: fresh +200 kW after recovery
	//     must ramp from measured (~0), never appearing at full value early.
	// The contract requires a dispatch *after* status is clear, and any
	// degraded cycle — a missed read or a timing gap on a starved host —
	// legitimately invalidates intent, so retry the
	// STANDBY → dispatch → ramp sequence rather than assuming the first
	// dispatch survives.
	reached := false
	for rev := int64(3); rev <= 6 && !reached; rev++ {
		if !poll(10*time.Second, func() bool { return loopState(t, p) == "STANDBY" }) {
			t.Fatalf("loop did not return to STANDBY for recovery dispatch: %+v",
				status(t, p)["loop"])
		}
		// A retry after a mid-ramp invalidation starts from a decaying
		// output — wait for it to settle so the jump check stays valid.
		if !poll(10*time.Second, func() bool { return outputKW(ctl.state(t)) < 5 }) {
			t.Fatalf("output did not settle before recovery dispatch: %+v", ctl.state(t))
		}
		code, _ = postDispatch(t, p, rev, 200, 30*time.Second)
		if code != 202 {
			t.Fatalf("recovery dispatch: %d", code)
		}
		jumped := false
		rampDeadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(rampDeadline) {
			if outputKW(ctl.state(t)) > 130 {
				jumped = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if jumped {
			t.Fatal("output jumped to old-scale target on recovery")
		}
		for t0 := time.Now(); time.Since(t0) < 8*time.Second; {
			if outputKW(ctl.state(t)) >= 195 {
				reached = true
				break
			}
			if status(t, p)["dispatch_active"] == false {
				break // a degraded cycle dropped the intent; re-dispatch
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	if !reached {
		t.Fatalf("recovery never reached target: sim=%+v loop=%+v",
			ctl.state(t), status(t, p)["loop"])
	}
}
