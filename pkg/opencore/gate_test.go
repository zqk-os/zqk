package opencore

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	gateSensitiveContent = "api_key = \"sk-1234567890abcdef\"\n"
	gateSafeContent      = "name = \"MyProject\"\nversion = \"1.0.0\"\n"
)

func writeGateFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
		t.Fatalf("writeGateFile: %v", err)
	}
}

func TestRunExportGate_PassOnCleanPayload(t *testing.T) {
	tmp := t.TempDir()
	writeGateFile(t, tmp, "README.md", gateSafeContent)

	report, err := RunExportGate(tmp, GateOptions{})
	if err != nil {
		t.Fatalf("RunExportGate failed: %v", err)
	}
	if report.Decision != GatePass {
		t.Fatalf("decision: got %v, want GatePass (reason %s)", report.Decision, report.Reason)
	}
	if report.PayloadReport == nil {
		t.Fatal("PayloadReport should never be nil on a pass")
	}
	if report.PayloadReport.TotalViolations != 0 {
		t.Errorf("TotalViolations: got %d, want 0", report.PayloadReport.TotalViolations)
	}
}

func TestRunExportGate_DenyOnSensitiveCredentials(t *testing.T) {
	tmp := t.TempDir()
	writeGateFile(t, tmp, "config.env", gateSensitiveContent)

	report, err := RunExportGate(tmp, GateOptions{})
	if err != nil {
		t.Fatalf("RunExportGate failed: %v", err)
	}
	if report.Decision != GateDeny {
		t.Fatalf("decision: got %v, want GateDeny", report.Decision)
	}
	if report.Reason == "" {
		t.Error("Reason should be non-empty on a deny")
	}
}

func TestRunExportGate_ToleranceAllowsFewViolations(t *testing.T) {
	tmp := t.TempDir()
	writeGateFile(t, tmp, "config.env", gateSensitiveContent)

	report, err := RunExportGate(tmp, GateOptions{MaxAllowedViolations: 3})
	if err != nil {
		t.Fatalf("RunExportGate failed: %v", err)
	}
	if report.Decision != GatePass {
		t.Fatalf("decision: got %v, want GatePass with tolerance=3 (reason %s)", report.Decision, report.Reason)
	}
}

func TestRunExportGate_ExcludedFilesDoNotTripGate(t *testing.T) {
	tmp := t.TempDir()
	writeGateFile(t, tmp, ".env", gateSensitiveContent)

	report, err := RunExportGate(tmp, GateOptions{ExcludedPatterns: []string{"*.env"}})
	if err != nil {
		t.Fatalf("RunExportGate failed: %v", err)
	}
	if report.Decision != GatePass {
		t.Fatalf("decision: got %v, want GatePass for excluded .env", report.Decision)
	}
}

func TestRunExportGate_DenyOnBinaryInStrictMode(t *testing.T) {
	tmp := t.TempDir()
	elf := []byte{0x7f, 0x45, 0x4c, 0x46, 0x02, 0x01, 0x01, 0x00}
	writeGateBytes(t, tmp, "libfoo.so", elf)

	report, err := RunExportGate(tmp, GateOptions{DenyOnBinary: true})
	if err != nil {
		t.Fatalf("RunExportGate failed: %v", err)
	}
	if report.Decision != GateDeny {
		t.Fatalf("decision: got %v, want GateDeny for binary in strict mode", report.Decision)
	}
}

func TestRunExportGate_BinaryAllowedWhenStrictOff(t *testing.T) {
	tmp := t.TempDir()
	elf := []byte{0x7f, 0x45, 0x4c, 0x46, 0x02, 0x01, 0x01, 0x00}
	writeGateBytes(t, tmp, "asset.bin", elf)

	report, err := RunExportGate(tmp, GateOptions{DenyOnBinary: false})
	if err != nil {
		t.Fatalf("RunExportGate failed: %v", err)
	}
	if report.Decision != GatePass {
		t.Fatalf("decision: got %v, want GatePass with DenyOnBinary off", report.Decision)
	}
	// But the evidence should still record the binary finding.
	found := false
	for _, v := range report.PayloadReport.Violations {
		if v.Category == CategoryBinaryDetect {
			found = true
		}
	}
	if !found {
		t.Error("expected binary-detect evidence recorded in payload report")
	}
}

func TestRunExportGate_InvalidExclusionIsError(t *testing.T) {
	tmp := t.TempDir()
	if _, err := RunExportGate(tmp, GateOptions{ExcludedPatterns: []string{"a[b"}}); err == nil {
		t.Fatal("expected an error for an invalid exclusion glob, got nil")
	}
}

func TestRunExportGate_NegativeToleranceIsError(t *testing.T) {
	tmp := t.TempDir()
	if _, err := RunExportGate(tmp, GateOptions{MaxAllowedViolations: -1}); err == nil {
		t.Fatal("expected an error for negative MaxAllowedViolations, got nil")
	}
}

func TestGateReport_EmptyDirPasses(t *testing.T) {
	tmp := t.TempDir()
	report, err := RunExportGate(tmp, GateOptions{})
	if err != nil {
		t.Fatalf("RunExportGate failed: %v", err)
	}
	if report.Decision != GatePass {
		t.Fatalf("decision: got %v, want GatePass on empty dir", report.Decision)
	}
	if report.Reason == "" {
		t.Error("Reason should be non-empty on a pass")
	}
}

func writeGateBytes(t *testing.T, dir, name string, content []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), content, 0600); err != nil {
		t.Fatalf("writeGateBytes: %v", err)
	}
}

// TestOpenCoreExportGate_FunctionalAcceptance verifies CRIT-1789670649350985000-5aac757f
func TestOpenCoreExportGate_FunctionalAcceptance(t *testing.T) {
	tmp := t.TempDir()
	writeGateFile(t, tmp, "README.md", gateSafeContent)

	report, err := RunExportGate(tmp, GateOptions{})
	if err != nil {
		t.Fatalf("RunExportGate failed: %v", err)
	}
	if report.Decision != GatePass {
		t.Fatalf("expected GatePass, got %s (reason: %s)", report.Decision, report.Reason)
	}
}

// TestOpenCoreExportGate_BoundaryAndErrorHandling verifies CRIT-1789670649350986000-2e289d53
func TestOpenCoreExportGate_BoundaryAndErrorHandling(t *testing.T) {
	tmp := t.TempDir()
	writeGateFile(t, tmp, "leak.env", gateSensitiveContent)

	report, err := RunExportGate(tmp, GateOptions{})
	if err != nil {
		t.Fatalf("RunExportGate failed: %v", err)
	}
	if report.Decision != GateDeny {
		t.Fatalf("expected GateDeny on sensitive credentials, got %s", report.Decision)
	}

	if _, err := RunExportGate(tmp, GateOptions{MaxAllowedViolations: -1}); err == nil {
		t.Fatal("expected error on negative tolerance, got nil")
	}
}

// TestOpenCoreExportGate_IntegrationAndConformance verifies CRIT-1789670649350987000-1b12e953
func TestOpenCoreExportGate_IntegrationAndConformance(t *testing.T) {
	tmp := t.TempDir()
	elf := []byte{0x7f, 0x45, 0x4c, 0x46, 0x02, 0x01, 0x01, 0x00}
	writeGateBytes(t, tmp, "binary.so", elf)

	report, err := RunExportGate(tmp, GateOptions{DenyOnBinary: true})
	if err != nil {
		t.Fatalf("RunExportGate failed: %v", err)
	}
	if report.Decision != GateDeny {
		t.Fatalf("expected GateDeny on strict binary, got %s", report.Decision)
	}
}
