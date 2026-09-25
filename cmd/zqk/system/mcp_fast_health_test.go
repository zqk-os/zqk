package system

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

func TestGetSystemHealthDataMCPFast_WithCachedSummary(t *testing.T) {
	tmpDir := t.TempDir()
	healthDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.SystemHealthDir)
	if err := os.MkdirAll(healthDir, paths.FilePerm755); err != nil {
		t.Fatalf("failed to create temp health dir: %v", err)
	}

	payload := map[string]any{
		"summary": map[string]any{
			"total_objects":    1500,
			"blocking_issues":  0,
			"warnings":         2,
			"informational":    5,
			"recommendations": 10,
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal summary: %v", err)
	}
	cacheFile := filepath.Join(healthDir, systemHealthLatestFile)
	if err := os.WriteFile(cacheFile, data, paths.FilePerm644); err != nil {
		t.Fatalf("failed to write summary file: %v", err)
	}

	health := getSystemHealthDataMCPFast(tmpDir)
	if health["mcp_fast"] != true {
		t.Errorf("expected mcp_fast=true, got %v", health["mcp_fast"])
	}
	if health[objects.FieldKeyStatus] != "warning" {
		t.Errorf("expected status=warning, got %v", health[objects.FieldKeyStatus])
	}
	if health["warnings"] != 2 {
		t.Errorf("expected warnings=2, got %v", health["warnings"])
	}
	if health["total_objects"] != 1500 {
		t.Errorf("expected total_objects=1500, got %v", health["total_objects"])
	}

	// Now test degraded status when blocking issues > 0
	payload["summary"].(map[string]any)["blocking_issues"] = 3
	data, _ = json.Marshal(payload)
	_ = os.WriteFile(cacheFile, data, paths.FilePerm644)

	healthDegraded := getSystemHealthDataMCPFast(tmpDir)
	if healthDegraded[objects.FieldKeyStatus] != "degraded" {
		t.Errorf("expected status=degraded, got %v", healthDegraded[objects.FieldKeyStatus])
	}
	if healthDegraded["blocking_issues"] != 3 {
		t.Errorf("expected blocking_issues=3, got %v", healthDegraded["blocking_issues"])
	}

	// Now test healthy when both 0
	payload["summary"].(map[string]any)["blocking_issues"] = 0
	payload["summary"].(map[string]any)["warnings"] = 0
	data, _ = json.Marshal(payload)
	_ = os.WriteFile(cacheFile, data, paths.FilePerm644)

	healthHealthy := getSystemHealthDataMCPFast(tmpDir)
	if healthHealthy[objects.FieldKeyStatus] != "healthy" {
		t.Errorf("expected status=healthy, got %v", healthHealthy[objects.FieldKeyStatus])
	}
}

func TestGetSystemHealthDataMCPFast_WithoutCache(t *testing.T) {
	tmpDir := t.TempDir()
	health := getSystemHealthDataMCPFast(tmpDir)
	if health["mcp_fast"] != true {
		t.Errorf("expected mcp_fast=true, got %v", health["mcp_fast"])
	}
	if health[objects.FieldKeyStatus] != "unknown" {
		t.Errorf("expected status=unknown when cache absent, got %v", health[objects.FieldKeyStatus])
	}
}
