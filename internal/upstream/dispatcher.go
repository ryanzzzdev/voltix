package upstream

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// Dispatcher is a test upstream that issues dispatch commands to a voltix
// controller. It learns the boot session from GET /api/status and issues
// monotonically increasing revisions, like a real energy-market bridge.
type Dispatcher struct {
	base  string
	token string
	ctlID string
	http  *http.Client

	bootID string
	rev    atomic.Int64
}

func NewDispatcher(controllerAPI, token, controllerID string) *Dispatcher {
	return &Dispatcher{
		base: controllerAPI, token: token, ctlID: controllerID,
		http: &http.Client{Timeout: 3 * time.Second},
	}
}

// LearnBootSession refreshes the boot session id (e.g. after a restart).
func (d *Dispatcher) LearnBootSession() error {
	req, _ := http.NewRequest(http.MethodGet, d.base+"/api/status", nil)
	req.Header.Set("Authorization", "Bearer "+d.token)
	resp, err := d.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var v struct {
		BootSessionID string `json:"boot_session_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return err
	}
	d.bootID = v.BootSessionID
	d.rev.Store(0)
	return nil
}

// Dispatch issues one command with the next revision.
func (d *Dispatcher) Dispatch(targetKW float64, validFor time.Duration) (int, error) {
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]any{
		"controller_id":   d.ctlID,
		"boot_session_id": d.bootID,
		"command_id": fmt.Sprintf("d1517a7e-0000-4000-8000-%012d",
			d.rev.Load()+1),
		"revision":    d.rev.Load() + 1,
		"issued_at":   now.Format(time.RFC3339Nano),
		"valid_until": now.Add(validFor).Format(time.RFC3339Nano),
		"target_kw":   targetKW,
	})
	req, _ := http.NewRequest(http.MethodPost, d.base+"/api/dispatch",
		bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+d.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.http.Do(req)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusAccepted {
		d.rev.Add(1)
	}
	return resp.StatusCode, nil
}
