package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"voltix/internal/control"
	"voltix/internal/device"
	"voltix/internal/dispatch"
	"voltix/internal/policy"
)

var version = "dev"

type config struct {
	ControllerID string `yaml:"controller_id"`
	Devices      struct {
		Inverter deviceRef `yaml:"inverter"`
		Meter    deviceRef `yaml:"meter"`
	} `yaml:"devices"`
	Limits struct {
		SoCMinPct        float64 `yaml:"soc_min_pct"`
		SoCMaxPct        float64 `yaml:"soc_max_pct"`
		SiteExportMaxKW  float64 `yaml:"site_export_max_kw"`
		ChargeMaxKW      float64 `yaml:"charge_max_kw"`
		DischargeMaxKW   float64 `yaml:"discharge_max_kw"`
		NormalSlewKWPerS float64 `yaml:"normal_slew_kw_per_s"`
	} `yaml:"limits"`
	Timing struct {
		CyclePeriodMs  int     `yaml:"cycle_period_ms"`
		ReadDeadlineMs int     `yaml:"read_deadline_ms"`
		MaxInputAgeS   float64 `yaml:"max_input_age_s"`
		MaxLoopGapS    float64 `yaml:"max_loop_gap_s"`
	} `yaml:"timing"`
	Upstream struct {
		DispatchListen string `yaml:"dispatch_listen"`
	} `yaml:"upstream"`
}

type deviceRef struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

func loadConfig(path string) (config, error) {
	var c config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	return c, yaml.Unmarshal(b, &c)
}

func main() {
	configPath := flag.String("config", "", "controller configuration file (required)")
	flag.Parse()
	if *configPath == "" {
		log.Fatal("-config is required")
	}
	c, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	token := os.Getenv("VOLTIX_DISPATCH_TOKEN")
	if token == "" {
		log.Fatal("VOLTIX_DISPATCH_TOKEN is not set")
	}

	timeout := time.Duration(c.Timing.ReadDeadlineMs) * time.Millisecond
	inverter := device.NewEndpoint(
		fmt.Sprintf("%s:%d", c.Devices.Inverter.Host, c.Devices.Inverter.Port), timeout)
	meter := device.NewEndpoint(
		fmt.Sprintf("%s:%d", c.Devices.Meter.Host, c.Devices.Meter.Port), timeout)
	defer inverter.Close()
	defer meter.Close()

	// SunSpec model-chain validation: fail fast on identity/configuration
	// faults rather than run on the wrong device.
	for name, ep := range map[string]*device.Endpoint{
		"inverter": inverter, "meter": meter,
	} {
		required := map[string][]uint16{
			"inverter": {1, 103, 124, 704},
			"meter":    {1, 203},
		}[name]
		if _, err := ep.Discover(required); err != nil {
			log.Fatalf("%s identity: %v", name, err)
		}
	}

	session := dispatch.NewSession(c.ControllerID, dispatch.RealClock())
	srv := dispatch.NewServer(session, token)

	snaps := make(chan control.Snapshot, 8)
	loop := control.New(control.LoopConfig{
		Inverter:  inverter,
		Meter:     meter,
		Session:   session,
		Snapshots: snaps,
		Period:    time.Duration(c.Timing.CyclePeriodMs) * time.Millisecond,
		Policy: policy.New(policy.Config{
			DischargeLimitW: int64(c.Limits.DischargeMaxKW * 1000),
			ChargeLimitW:    int64(c.Limits.ChargeMaxKW * 1000),
			ExportLimitW:    int64(c.Limits.SiteExportMaxKW * 1000),
			SoCFloorPct:     c.Limits.SoCMinPct,
			SoCCeilPct:      c.Limits.SoCMaxPct,
			SlewWPS:         int64(c.Limits.NormalSlewKWPerS * 1000),
			MaxSnapshotAge:  time.Duration(c.Timing.MaxInputAgeS * float64(time.Second)),
			MaxGap:          time.Duration(c.Timing.MaxLoopGapS * float64(time.Second)),
		}),
	})
	srv.StatusExtra = func() any {
		if s, ok := loop.Latest(); ok {
			return s
		}
		return nil
	}

	httpSrv := &http.Server{
		Addr:              c.Upstream.DispatchListen,
		Handler:           srv,
		ReadHeaderTimeout: 3 * time.Second,
	}
	ln, err := net.Listen("tcp", httpSrv.Addr)
	if err != nil {
		log.Fatalf("dispatch listen: %v", err)
	}
	go func() { _ = httpSrv.Serve(ln) }()
	go func() {
		// Drain the snapshot channel so a stuck consumer never exists.
		for range snaps {
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("voltix %s: controller_id=%s boot_session=%s dispatch=%s",
		version, c.ControllerID, session.BootSessionID(), httpSrv.Addr)
	loop.Run(ctx) // returns after the bounded zero write on shutdown

	shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutCtx)
	log.Print("voltix stopped")
}
