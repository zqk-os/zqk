package agentfeed

import (
	"strings"
	"testing"
)

func TestIsUnrepairedWake(t *testing.T) {
	t.Parallel()
	if ok, _, _ := IsUnrepairedWake(PeerWakeResult{Attempted: true, Live: true}); ok {
		t.Fatal("live successful attempt should not be unrepaired")
	}
	if ok, reason, detail := IsUnrepairedWake(PeerWakeResult{Attempted: true, Live: false, Transport: "tpm_stamp"}); !ok || reason != UnrepairedWakeStampNotLive || detail != "tpm_stamp" {
		t.Fatalf("stamp-not-live: ok=%v reason=%q detail=%q", ok, reason, detail)
	}
	if ok, reason, detail := IsUnrepairedWake(PeerWakeResult{Attempted: true, Error: "boom"}); !ok || reason != UnrepairedWakeFailed || detail != "boom" {
		t.Fatalf("failed wake: ok=%v reason=%q detail=%q", ok, reason, detail)
	}
	if ok, reason, detail := IsUnrepairedWake(PeerWakeResult{Skipped: "wake_script_missing"}); !ok || reason != UnrepairedWakeSkipped || detail != "wake_script_missing" {
		t.Fatalf("skipped wake: ok=%v reason=%q detail=%q", ok, reason, detail)
	}
}

func TestAlertUnrepairedWake_invokesDesktopHook(t *testing.T) {
	desktopNotifyMu.Lock()
	prev := desktopNotifyFn
	called := 0
	var gotTitle, gotBody string
	desktopNotifyFn = func(title, body string) error {
		called++
		gotTitle, gotBody = title, body
		return nil
	}
	desktopNotifyMu.Unlock()
	t.Cleanup(func() {
		desktopNotifyMu.Lock()
		desktopNotifyFn = prev
		desktopNotifyMu.Unlock()
	})

	reason := AlertUnrepairedWake("AFE-1", "script failed", UnrepairedWakeFailed)
	if reason != string(UnrepairedWakeFailed) {
		t.Fatalf("reason=%q", reason)
	}
	if called != 1 {
		t.Fatalf("desktop notify calls=%d", called)
	}
	if gotTitle != "ZQK peer wake unrepaired" {
		t.Fatalf("title=%q", gotTitle)
	}
	if !strings.Contains(gotBody, "wake_failed") || !strings.Contains(gotBody, "AFE-1") || !strings.Contains(gotBody, "script failed") {
		t.Fatalf("body=%q", gotBody)
	}
}
