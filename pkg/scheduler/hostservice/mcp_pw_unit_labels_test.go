package hostservice_test

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/scheduler/hostservice"
)

func TestMCPDaemonAndPrivilegedWriterUnitLabels_RootScoped(t *testing.T) {
	t.Parallel()
	a := hostservice.RootID("/Users/me/proj-a")
	b := hostservice.RootID("/Users/me/proj-b")
	la := hostservice.MCPDaemonUnitLabel(a)
	lb := hostservice.MCPDaemonUnitLabel(b)
	if la == lb {
		t.Fatalf("different roots must not share mcp-daemon label: %s", la)
	}
	if !strings.Contains(la, a) || !strings.HasPrefix(la, "com.zqk.mcp-daemon.") {
		t.Fatalf("mcp-daemon label missing root_id: %s", la)
	}
	pa := hostservice.PrivilegedWriterUnitLabel(a)
	pb := hostservice.PrivilegedWriterUnitLabel(b)
	if pa == pb {
		t.Fatalf("different roots must not share privileged-writer label: %s", pa)
	}
	if !strings.Contains(pa, a) || !strings.HasPrefix(pa, "com.zqk.privileged-writer.") {
		t.Fatalf("privileged-writer label missing root_id: %s", pa)
	}

	legacyMCP := hostservice.LegacyMCPDaemonUnitLabel()
	legacyPW := hostservice.LegacyPrivilegedWriterUnitLabel()
	if legacyMCP != "com.zqk.mcp-daemon" || legacyPW != "com.zqk.privileged-writer" {
		t.Fatalf("unexpected legacy labels: %s %s", legacyMCP, legacyPW)
	}
	if hostservice.IsLegacyMCPDaemonLabel(la) || !hostservice.IsLegacyMCPDaemonLabel(legacyMCP) {
		t.Fatal("IsLegacyMCPDaemonLabel mismatch")
	}
	if hostservice.IsLegacyPrivilegedWriterLabel(pa) || !hostservice.IsLegacyPrivilegedWriterLabel(legacyPW) {
		t.Fatal("IsLegacyPrivilegedWriterLabel mismatch")
	}

	parsed := hostservice.ParseMCPDaemonLabel(la)
	if !parsed.Valid || parsed.Legacy || parsed.RootID != a {
		t.Fatalf("ParseMCPDaemonLabel = %+v", parsed)
	}
	parsedPW := hostservice.ParsePrivilegedWriterLabel(pa)
	if !parsedPW.Valid || parsedPW.Legacy || parsedPW.RootID != a {
		t.Fatalf("ParsePrivilegedWriterLabel = %+v", parsedPW)
	}
}
