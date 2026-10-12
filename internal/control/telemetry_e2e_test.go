//go:build integration

// VTX-015 telemetry verification: real voltix + sim + upstream receiver
// over TCP. Run with: make integration

package control_test

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"voltix/internal/upstream"
)

// startReceiver runs the mock upstream on a real port (not httptest —
// voltix is a separate process and needs a stable URL).
func startReceiver(t *testing.T) (url string, ctl func(string) map[string]any) {
	t.Helper()
	r, err := upstream.OpenReceiver(t.TempDir(), telToken)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: r}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close(); r.Close() })
	url = "http://" + ln.Addr().String()
	return url, func(path string) map[string]any {
		method := http.MethodPost
		if path == "/control/count" {
			method = http.MethodGet
		}
		req, _ := http.NewRequest(method, url+path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var v map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&v)
		return v
	}
}

func telBlock(t *testing.T, p *voltixProc) map[string]any {
	t.Helper()
	v, ok := status(t, p)["telemetry"].(map[string]any)
	if !ok {
		t.Fatalf("no telemetry block: %+v", status(t, p))
	}
	return v
}

func telNum(t *testing.T, p *voltixProc, key string) float64 {
	t.Helper()
	v, _ := telBlock(t, p)[key].(float64)
	return v
}

func telExtra(dir, url string, maxStatus, maxAlarm, apiPort int) string {
	return fmt.Sprintf(`telemetry:
  state_dir: %s
  max_status_events: %d
  max_alarm_events: %d
  max_payload_bytes: 1024
  handoff_status_slots: 900
  handoff_alarm_slots: 100
upstream:
  dispatch_listen: 127.0.0.1:%d
  telemetry_url: %s/api/telemetry
`, dir, maxStatus, maxAlarm, apiPort, url)
}

func TestTelemetryEndToEnd(t *testing.T) {
	invPort, metPort, ctl := startSim(t)
	apiPort := freePort(t)
	bin := buildVoltix(t)
	recvURL, rctl := startReceiver(t)

	stateDir := t.TempDir()
	base := func(extra string) string {
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
%s`, ctlID, invPort, metPort, extra)
	}
	cfg := base(telExtra(stateDir, recvURL, 90000, 10000, apiPort))
	p := startVoltixCfg(t, bin, cfg)

	// 1. Baseline: events queue and drain to the receiver (pending depth
	//    hovers between 5 s flushes — it need not reach zero).
	if !poll(8*time.Second, func() bool {
		return telNum(t, p, "uploaded") > 0
	}) {
		t.Fatalf("baseline upload failed: %+v", telBlock(t, p))
	}

	// 2. Telemetry-only outage: dispatch stays valid, queue grows.
	rctl("/control/pause")
	code, _ := postDispatch(t, p, 1, 100, 30*time.Second)
	if code != 202 {
		t.Fatalf("dispatch during outage: %d", code)
	}
	if !poll(6*time.Second, func() bool {
		return loopState(t, p) == "ACTIVE" && outputKW(ctl.state(t)) > 10
	}) {
		t.Fatal("control degraded by telemetry outage")
	}
	if !poll(4*time.Second, func() bool {
		return telNum(t, p, "depth_status") > 2
	}) {
		t.Fatalf("queue did not grow during outage: %+v", telBlock(t, p))
	}
	// Catch-up while collection continues. The outage grows the uploader's
	// bounded backoff, so the first post-resume retry may be ~15 s out.
	rctl("/control/resume")
	if !poll(25*time.Second, func() bool {
		return telNum(t, p, "depth_status") == 0
	}) {
		t.Fatalf("backlog did not drain: %+v", telBlock(t, p))
	}
	recvCount := rctl("/control/count")["count"].(float64)
	if recvCount < telNum(t, p, "uploaded") {
		t.Fatalf("receiver count %v < uploaded %v", recvCount,
			telNum(t, p, "uploaded"))
	}

	// 3. Lost-ack replay: one-shot drop — the replay is deduped and the
	//    batch is still delivered exactly once at the receiver.
	up0 := telNum(t, p, "uploaded")
	rctl("/control/drop-after-commit")
	if !poll(8*time.Second, func() bool {
		return telNum(t, p, "uploaded") > up0+2 &&
			telNum(t, p, "upload_failed") > 0
	}) {
		t.Fatalf("lost-ack replay did not recover: %+v", telBlock(t, p))
	}

	// 4. Crash/reopen: SIGKILL, then restart with the same state dir —
	//    stream identity persists and queued events still upload.
	sid0 := telBlock(t, p)["stream_id"]
	p.cmd.Process.Signal(syscall.SIGKILL)
	p.cmd.Wait()
	p = startVoltixCfg(t, bin, cfg)
	if sid := telBlock(t, p)["stream_id"]; sid != sid0 {
		t.Fatalf("stream identity changed across crash: %v -> %v", sid0, sid)
	}
	if !poll(8*time.Second, func() bool {
		return telNum(t, p, "uploaded") > 0
	}) {
		t.Fatalf("no upload after crash/reopen: %+v", telBlock(t, p))
	}

	// 5. Overflow: restart with tiny caps — evictions counted, control OK.
	p.cmd.Process.Signal(syscall.SIGKILL)
	p.cmd.Wait()
	smallDir := t.TempDir()
	p = startVoltixCfg(t, bin,
		base(telExtra(smallDir, recvURL, 10, 5, apiPort)))
	rctl("/control/pause") // hold uploads so the queue overflows
	if !poll(8*time.Second, func() bool {
		return telNum(t, p, "evicted_status") > 0
	}) {
		t.Fatalf("no eviction at small cap: %+v", telBlock(t, p))
	}
	// Local control still works under a full store.
	code, _ = postDispatch(t, p, 1, 50, 15*time.Second)
	if code != 202 {
		t.Fatalf("dispatch under full store: %d", code)
	}
	if !poll(6*time.Second, func() bool { return loopState(t, p) == "ACTIVE" }) {
		t.Fatalf("control degraded under store pressure: %s", loopState(t, p))
	}

	// 6. Measurements: footprint + process memory (logged for findings).
	entries, _ := os.ReadDir(smallDir)
	var dbBytes int64
	for _, e := range entries {
		if fi, err := e.Info(); err == nil {
			dbBytes += fi.Size()
		}
	}
	var rssKB string
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", p.cmd.Process.Pid)); err == nil {
		for _, ln := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(ln, "VmRSS:") {
				rssKB = strings.TrimSpace(strings.TrimPrefix(ln, "VmRSS:"))
			}
		}
	}
	t.Logf("MEASURE: db+wal=%d bytes, voltix_rss=%s, evicted=%v uploaded=%v",
		dbBytes, rssKB, telNum(t, p, "evicted_status"), telNum(t, p, "uploaded"))
}
