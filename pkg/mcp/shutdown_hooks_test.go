package mcp

import (
	"context"
	"errors"
	"testing"
)

func TestShutdownHookManager_LifetimeCounters(t *testing.T) {
	var m *ShutdownHookManager
	rNil, eNil, errNil := m.GetShutdownHookStats()
	if rNil != 0 || eNil != 0 || errNil != 0 {
		t.Fatalf("expected nil manager stats (0, 0, 0), got r=%d e=%d err=%d", rNil, eNil, errNil)
	}

	mgr := NewShutdownHookManager()
	rInit, eInit, errInit := mgr.GetShutdownHookStats()
	if rInit != 0 || eInit != 0 || errInit != 0 {
		t.Fatalf("expected new manager stats (0, 0, 0), got r=%d e=%d err=%d", rInit, eInit, errInit)
	}

	// Register a successful hook
	mgr.RegisterHook(func(ctx context.Context) error {
		return nil
	})

	// Register a failing hook
	dummyErr := errors.New("shutdown error")
	mgr.RegisterHook(func(ctx context.Context) error {
		return dummyErr
	})

	rAfterReg, eAfterReg, errAfterReg := mgr.GetShutdownHookStats()
	if rAfterReg != 2 || eAfterReg != 0 || errAfterReg != 0 {
		t.Errorf("expected stats (2, 0, 0) after register, got r=%d e=%d err=%d", rAfterReg, eAfterReg, errAfterReg)
	}

	// Notify hooks -> 2 executed, 1 error
	errs := mgr.NotifyHooks()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error from NotifyHooks, got %d", len(errs))
	}

	rFinal, eFinal, errFinal := mgr.GetShutdownHookStats()
	if rFinal != 2 || eFinal != 2 || errFinal != 1 {
		t.Errorf("expected stats (2, 2, 1) after notify, got r=%d e=%d err=%d", rFinal, eFinal, errFinal)
	}
}
