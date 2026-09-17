package healthcheck

import (
	"context"
	"errors"
	"testing"
)

type stubMonitor struct {
	id  string
	res *Result
	err error
}

func (s stubMonitor) ID() string   { return s.id }
func (s stubMonitor) Name() string { return s.id }
func (s stubMonitor) Run(context.Context, string) (*Result, error) {
	return s.res, s.err
}

func TestRegistryRun_EmptyEnabledMonitorsIsNotOK(t *testing.T) {
	t.Parallel()
	r := NewRegistry("")
	res, err := r.Run(context.Background(), "", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res == nil || res.Status == statusOK {
		t.Fatalf("empty registry must not be ok: %#v", res)
	}
	if res.Status != statusSkip {
		t.Fatalf("status=%q want %q", res.Status, statusSkip)
	}
}

func TestRegistryRun_NilResultIsFail(t *testing.T) {
	t.Parallel()
	r := NewRegistry("")
	r.Register(stubMonitor{id: "nil-monitor", res: nil})
	res, err := r.Run(context.Background(), "", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res == nil || res.Status != statusFail {
		t.Fatalf("nil Result must fail: %#v", res)
	}
}

func TestRegistryRun_MonitorErrorIsFail(t *testing.T) {
	t.Parallel()
	r := NewRegistry("")
	r.Register(stubMonitor{id: "err-monitor", err: errors.New("boom")})
	res, err := r.Run(context.Background(), "", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res == nil || res.Status != statusFail {
		t.Fatalf("Run error must fail: %#v", res)
	}
}

func TestRegistryRun_OKMonitorStaysOK(t *testing.T) {
	t.Parallel()
	r := NewRegistry("")
	r.Register(stubMonitor{id: "ok-monitor", res: &Result{Status: statusOK, Summary: "fine"}})
	res, err := r.Run(context.Background(), "", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res == nil || res.Status != statusOK {
		t.Fatalf("got %#v", res)
	}
}
