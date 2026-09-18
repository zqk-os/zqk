package idebridge

import (
	"os"
	"strings"
	"unicode/utf8"

	"github.com/zqk-os/zqk/pkg/brand"
)

// CmdWakeAttn is the core (always-on) extension command for mesh peer wake.
// Not gated by capability packs — works with a default mcp-only install.
const CmdWakeAttn = "zqk.wake.attn"

const maxWakeAttnRunes = 240

// WakeAttnDisabled reports ZQK_IDE_BRIDGE_WAKE=0|false|off (opt-out).
// TRACK: add zqkenv accessor when this env is promoted to a formal brand key.
func WakeAttnDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvVar("IDE_BRIDGE_WAKE"))))
	switch v {
	case "0", "false", "off", "no":
		return true
	default:
		return false
	}
}

// QueueWakeAttn appends zqk.wake.attn for the IDE bridge.
// ATTN/steer lines inject into the existing Composer (extension ≥ 0.1.10).
// PROOF-OF-LIFE-prefixed messages stay toast/status-bar only.
// Returns false when disabled or append fails — callers must not fail the wake path.
func QueueWakeAttn(projectRoot, message string) bool {
	if WakeAttnDisabled() {
		return false
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "ZQK mesh: check agent feed (whats-next / feed pending)"
	}
	msg = truncateRunes(msg, maxWakeAttnRunes)
	_, err := AppendRequest(projectRoot, CmdWakeAttn, "", []any{msg})
	return err == nil
}

func truncateRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "…"
}
