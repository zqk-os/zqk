package agentfeed

import (
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/execwrap"
)

// UnrepairedWakeReason classifies why a peer wake did not succeed after feed append.
type UnrepairedWakeReason string

const (
	UnrepairedWakeFailed       UnrepairedWakeReason = "wake_failed"
	UnrepairedWakeSkipped      UnrepairedWakeReason = "wake_skipped"
	UnrepairedWakeStampNotLive UnrepairedWakeReason = "stamp_not_live" // TPM stamp / no live subscriber
)

// desktopNotifyFn is overridden in tests. Best-effort OS banner; never blocks feed success.
var desktopNotifyFn = defaultDesktopNotify

var desktopNotifyMu sync.Mutex

// AlertUnrepairedWake records that feed append succeeded but peer wake did not.
// Best-effort Darwin/Linux desktop notify + returns a stable reason string for CLI output.
//
// TRACK: [REDACTED-ID] — Phase B fail-loud unrepaired-wake slice;
// session last_activity lease lives in pkg/mcp (TouchSessionLastActivity).
func AlertUnrepairedWake(eventID, detail string, reason UnrepairedWakeReason) string {
	detail = strings.TrimSpace(detail)
	eventID = strings.TrimSpace(eventID)
	title := "ZQK peer wake unrepaired"
	body := fmt.Sprintf("%s event=%s", reason, eventID)
	if detail != "" {
		body = body + " — " + truncateRunes(detail, 180)
	}
	desktopNotifyMu.Lock()
	fn := desktopNotifyFn
	desktopNotifyMu.Unlock()
	_ = fn(title, body)
	return string(reason)
}

// IsUnrepairedWake reports whether a wake result should fail-loud to the human.
func IsUnrepairedWake(wake PeerWakeResult) (bool, UnrepairedWakeReason, string) {
	if !wake.Attempted {
		if sk := strings.TrimSpace(wake.Skipped); sk != "" {
			return true, UnrepairedWakeSkipped, sk
		}
		return false, "", ""
	}
	if err := strings.TrimSpace(wake.Error); err != "" {
		return true, UnrepairedWakeFailed, err
	}
	// Stamp-only TPM membrane must not look like a successful live wake.
	if !wake.Live {
		detail := strings.TrimSpace(wake.Transport)
		if detail == "" {
			detail = "non_live_transport"
		}
		return true, UnrepairedWakeStampNotLive, detail
	}
	return false, "", ""
}

func defaultDesktopNotify(title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(`display notification %q with title %q`, body, title)
		return execwrap.Command("osascript", "-e", script).Run()
	case "linux":
		return execwrap.Command("notify-send", "-u", "critical", title, body).Run()
	default:
		return nil
	}
}

func truncateRunes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	// byte-safe enough for ASCII log/CLI detail
	return s[:max] + "…"
}
