package hostservice

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/brand"
)

// MCP daemon and PrivilegedWriter LaunchAgent labels are root-scoped so concurrent
// ZQK checkouts cannot last-install-wins on a single host-global unit
// (studio :8443 / project PW socket). Same sovereignty pattern as scheduler + seat-workers.
//
// Shape: com.<brand>.mcp-daemon.<root_id>
//        com.<brand>.privileged-writer.<root_id>
// Legacy (unscoped): com.<brand>.mcp-daemon / com.<brand>.privileged-writer
//
// TRACK: BLI-1794029815318590500-e7b20dbd / BLI-1793916946555828500-77934cb1

func mcpDaemonLabelPrefix() string {
	return "com." + brand.ExecutableName() + ".mcp-daemon"
}

func privilegedWriterLabelPrefix() string {
	return "com." + brand.ExecutableName() + ".privileged-writer"
}

// MCPDaemonUnitLabel returns the root-scoped LaunchAgent label for the MCP KeepAlive daemon.
func MCPDaemonUnitLabel(rootID string) string {
	return mcpDaemonLabelPrefix() + "." + strings.TrimSpace(rootID)
}

// LegacyMCPDaemonUnitLabel is the pre-sovereignty host-global MCP daemon label.
func LegacyMCPDaemonUnitLabel() string {
	return mcpDaemonLabelPrefix()
}

// PrivilegedWriterUnitLabel returns the root-scoped LaunchAgent label for PrivilegedWriter.
func PrivilegedWriterUnitLabel(rootID string) string {
	return privilegedWriterLabelPrefix() + "." + strings.TrimSpace(rootID)
}

// LegacyPrivilegedWriterUnitLabel is the pre-sovereignty host-global PrivilegedWriter label.
func LegacyPrivilegedWriterUnitLabel() string {
	return privilegedWriterLabelPrefix()
}

// MCPDaemonLabelParts is the parse result for an MCP daemon unit label.
type MCPDaemonLabelParts struct {
	RootID string // empty when Legacy
	Legacy bool
	Valid  bool
	Label  string
}

// ParseMCPDaemonLabel classifies root-scoped vs legacy MCP daemon labels.
func ParseMCPDaemonLabel(label string) MCPDaemonLabelParts {
	label = strings.TrimSpace(label)
	out := MCPDaemonLabelParts{Label: label}
	prefix := mcpDaemonLabelPrefix()
	if label == prefix {
		out.Valid = true
		out.Legacy = true
		return out
	}
	dotPrefix := prefix + "."
	if !strings.HasPrefix(label, dotPrefix) {
		return out
	}
	rest := strings.TrimPrefix(label, dotPrefix)
	if !rootIDHexRe.MatchString(rest) {
		return out
	}
	out.Valid = true
	out.RootID = rest
	return out
}

// PrivilegedWriterLabelParts is the parse result for a PrivilegedWriter unit label.
type PrivilegedWriterLabelParts struct {
	RootID string // empty when Legacy
	Legacy bool
	Valid  bool
	Label  string
}

// ParsePrivilegedWriterLabel classifies root-scoped vs legacy PrivilegedWriter labels.
func ParsePrivilegedWriterLabel(label string) PrivilegedWriterLabelParts {
	label = strings.TrimSpace(label)
	out := PrivilegedWriterLabelParts{Label: label}
	prefix := privilegedWriterLabelPrefix()
	if label == prefix {
		out.Valid = true
		out.Legacy = true
		return out
	}
	dotPrefix := prefix + "."
	if !strings.HasPrefix(label, dotPrefix) {
		return out
	}
	rest := strings.TrimPrefix(label, dotPrefix)
	if !rootIDHexRe.MatchString(rest) {
		return out
	}
	out.Valid = true
	out.RootID = rest
	return out
}

// IsLegacyMCPDaemonLabel reports the host-global (no root_id) MCP daemon label.
func IsLegacyMCPDaemonLabel(label string) bool {
	p := ParseMCPDaemonLabel(label)
	return p.Valid && p.Legacy
}

// IsLegacyPrivilegedWriterLabel reports the host-global (no root_id) PrivilegedWriter label.
func IsLegacyPrivilegedWriterLabel(label string) bool {
	p := ParsePrivilegedWriterLabel(label)
	return p.Valid && p.Legacy
}
