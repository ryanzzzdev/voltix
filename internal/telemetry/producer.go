// Package telemetry maps control-loop snapshots onto the durable event
// store. Status snapshots become TierStatus events; operating-state
// transitions additionally raise a TierAlarm event, so every failed
// cycle does not create an alarm storm. Offering is nonblocking —
// storage can never stall the control path.
package telemetry

import (
	"encoding/json"
	"time"

	"voltix/internal/control"
	"voltix/internal/policy"
	"voltix/internal/store"
)

// Producer consumes snapshots and enqueues store events.
type Producer struct {
	st      *store.Store
	last    policy.State
	hasLast bool
	dropped int64 // payloads rejected for size (should not happen)
}

func NewProducer(st *store.Store) *Producer { return &Producer{st: st} }

// Consume is the snapshot-channel drain: one status event per cycle,
// plus an alarm event on each operating-state transition.
func (p *Producer) Consume(snaps <-chan control.Snapshot) {
	for s := range snaps {
		p.status(s)
		if !p.hasLast || s.State != p.last {
			p.alarm(s)
		}
		p.last, p.hasLast = s.State, true
	}
}

func (p *Producer) status(s control.Snapshot) {
	body, _ := json.Marshal(map[string]any{
		"kind":  "status",
		"at":    s.At.UTC().Format(time.RFC3339Nano),
		"state": s.State.String(),
		"req_w": s.RequestedW, "con_w": s.ConstrainedW,
		"ack_w": s.Acknowledged,
		"inv_w": s.MeasuredInvW, "site_w": s.MeasuredSiteW,
		"rd_ms":  s.ReadDuration.Milliseconds(),
		"wr_ms":  s.WriteDuration.Milliseconds(),
		"inv_ok": s.InverterOK, "met_ok": s.MeterOK,
	})
	if err := p.st.Offer(store.TierStatus, s.At, body); err != nil {
		p.dropped++
	}
}

func (p *Producer) alarm(s control.Snapshot) {
	body, _ := json.Marshal(map[string]any{
		"kind": "transition", "at": s.At.UTC().Format(time.RFC3339Nano),
		"from": p.last.String(), "to": s.State.String(),
		"reasons": s.Reasons,
	})
	if err := p.st.Offer(store.TierAlarm, s.At, body); err != nil {
		p.dropped++
	}
}

// Dropped reports events rejected at Offer time (oversize only —
// handoff/volatile drops are counted inside the store).
func (p *Producer) Dropped() int64 { return p.dropped }
