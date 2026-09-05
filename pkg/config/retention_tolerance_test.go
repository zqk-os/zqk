package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		s       string
		want    time.Duration
		wantErr bool
	}{
		{"empty", "", 0, false},
		{"zero", "0", 0, false},
		{"24h", "24h", 24 * time.Hour, false},
		{"720h", "720h", 720 * time.Hour, false},
		{"30d", "30d", 30 * 24 * time.Hour, false},
		{"7d", "7d", 7 * 24 * time.Hour, false},
		{"1d", "1d", 24 * time.Hour, false},
		{"invalid", "x", 0, true},
		{"bad_d", "xd", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDuration(tt.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDuration(%q) err = %v, wantErr %v", tt.s, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestDefaultProtectStatuses(t *testing.T) {
	t.Parallel()
	got := DefaultProtectStatuses()
	if len(got) == 0 {
		t.Error("DefaultProtectStatuses() should not be empty")
	}
	// Should include common active-like statuses (archived is not protected so it can be deleted)
	for _, s := range []string{"in_progress", "planned", "draft"} {
		found := false
		for _, p := range got {
			if p == s {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("DefaultProtectStatuses() expected to include %q", s)
		}
	}
	// archived should not be in the list so archived objects are eligible for cleanup
	for _, p := range got {
		if p == "archived" {
			t.Error("DefaultProtectStatuses() should not include 'archived' (archived objects are deletable)")
			break
		}
	}
	// disconnected should not be protected so MCP session disconnect callback allows retention to prune
	for _, p := range got {
		if p == "disconnected" {
			t.Error("DefaultProtectStatuses() should not include 'disconnected' (disconnected MCP sessions are eligible for retention prune)")
			break
		}
	}
}

func TestGetToleranceForKind_mcp_session(t *testing.T) {
	t.Parallel()
	// mcp_session should have protect_statuses that only protect in_progress, so disconnected/archived can be pruned
	cfg := &RetentionToleranceConfig{
		Kinds: map[string]KindTolerance{
			"mcp_session": {
				ArchiveAfter:    "24h",
				CleanupAfter:    "168h",
				MaxCount:        50,
				ProtectStatuses: []string{"in_progress"},
			},
		},
	}
	tol, err := cfg.GetToleranceForKind("mcp_session")
	if err != nil {
		t.Fatal(err)
	}
	if tol.MaxCount != 50 {
		t.Errorf("mcp_session MaxCount = %d, want 50", tol.MaxCount)
	}
	if len(tol.ProtectStatuses) != 1 || tol.ProtectStatuses[0] != "in_progress" {
		t.Errorf("mcp_session ProtectStatuses = %v, want [in_progress]", tol.ProtectStatuses)
	}
	// disconnected must not be protected so retention can prune after disconnect callback
	for _, p := range tol.ProtectStatuses {
		if p == "disconnected" {
			t.Error("mcp_session protect_statuses must not include 'disconnected' so retention can prune")
		}
	}
}

func TestGetToleranceForKind(t *testing.T) {
	cfg := &RetentionToleranceConfig{
		Default: KindTolerance{
			ArchiveAfter: "48h",
			CleanupAfter: "720h",
			MaxCount:     0,
		},
		Kinds: map[string]KindTolerance{
			"audit_event": {
				ArchiveAfter:    "12h",
				CleanupAfter:    "24h",
				MaxCount:        1000,
				ProtectStatuses: []string{"custom_active"},
			},
		},
	}

	// kind in Kinds uses kind-specific values
	tol, err := cfg.GetToleranceForKind("audit_event")
	if err != nil {
		t.Fatal(err)
	}
	if tol.ArchiveAfter != 12*time.Hour || tol.CleanupAfter != 24*time.Hour || tol.MaxCount != 1000 {
		t.Errorf("audit_event tolerance: got archive=%v cleanup=%v max=%d", tol.ArchiveAfter, tol.CleanupAfter, tol.MaxCount)
	}
	if len(tol.ProtectStatuses) != 1 || tol.ProtectStatuses[0] != "custom_active" {
		t.Errorf("audit_event protect_statuses: got %v", tol.ProtectStatuses)
	}

	// kind not in Kinds uses default
	tol2, err := cfg.GetToleranceForKind("other_kind")
	if err != nil {
		t.Fatal(err)
	}
	if tol2.ArchiveAfter != 48*time.Hour || tol2.CleanupAfter != 720*time.Hour {
		t.Errorf("other_kind tolerance: got archive=%v cleanup=%v", tol2.ArchiveAfter, tol2.CleanupAfter)
	}
	if len(tol2.ProtectStatuses) == 0 {
		t.Error("other_kind should get default protect_statuses")
	}
}

func TestParseToleranceFromMap(t *testing.T) {
	t.Parallel()
	m := map[string]any{
		"archive_after":    "24h",
		"cleanup_after":    "168h",
		"max_count":        500,
		"protect_statuses": []any{"a", "b"},
	}
	tol, err := ParseToleranceFromMap(m, "test_kind")
	if err != nil {
		t.Fatal(err)
	}
	if tol.ArchiveAfter != 24*time.Hour || tol.CleanupAfter != 168*time.Hour || tol.MaxCount != 500 {
		t.Errorf("got archive=%v cleanup=%v max=%d", tol.ArchiveAfter, tol.CleanupAfter, tol.MaxCount)
	}
	if len(tol.ProtectStatuses) != 2 || tol.ProtectStatuses[0] != "a" || tol.ProtectStatuses[1] != "b" {
		t.Errorf("protect_statuses: got %v", tol.ProtectStatuses)
	}

	// nil map returns zero tolerance and default protect_statuses
	tol2, err := ParseToleranceFromMap(nil, "x")
	if err != nil {
		t.Fatal(err)
	}
	if len(tol2.ProtectStatuses) == 0 {
		t.Error("nil map should still get default protect_statuses")
	}
}

func TestEnabledKinds(t *testing.T) {
	t.Parallel()
	cfg := &RetentionToleranceConfig{}
	if got := cfg.EnabledKinds(); got != nil {
		t.Errorf("EnabledKinds() with nil Kinds = %v, want nil", got)
	}
	cfg.Kinds = map[string]KindTolerance{"a": {}, "b": {}}
	got := cfg.EnabledKinds()
	if len(got) != 2 {
		t.Errorf("EnabledKinds() = %v, want 2 kinds", got)
	}
}

func TestRealConfig_HighVolumeKinds_HaveEmptyProtectStatuses(t *testing.T) {
	t.Parallel()
	// Find project root
	projectRoot := "../../" // pkg/config -> pkg -> root
	loader := NewRetentionToleranceLoader(projectRoot)
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("failed to load real retention tolerance config: %v", err)
	}

	highVolumeKinds := []string{"change_journal_entry", "zqk_session"}
	for _, kind := range highVolumeKinds {
		tol, err := cfg.GetToleranceForKind(kind)
		if err != nil {
			t.Fatalf("kind %s not found in config", kind)
		}
		if tol.ProtectStatuses == nil {
			t.Errorf("kind %s must have an explicitly initialized empty protect_statuses slice, got nil (which implies default fallback)", kind)
		} else if len(tol.ProtectStatuses) > 0 {
			t.Errorf("kind %s must have exactly zero protected statuses, got %v", kind, tol.ProtectStatuses)
		}
	}
}

func TestRealConfig_HighVolumeKinds_DisableArchive(t *testing.T) {
	projectRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Skip("Could not determine project root")
	}
	cfg, err := NewRetentionToleranceLoader(projectRoot).Load()
	if err != nil {
		t.Fatalf("Failed to load real config: %v", err)
	}

	kindsToProtect := []string{"change_journal_entry", "zqk_session"}
	for _, kind := range kindsToProtect {
		tol, err := cfg.GetToleranceForKind(kind)
		if err != nil {
			t.Fatalf("Failed to get tolerance for %s: %v", kind, err)
		}
		if tol.ArchiveEnabled {
			t.Errorf("STRUCTURAL SAFEGUARD FAILED: %s MUST NOT have ArchiveEnabled=true in retention_tolerance.yaml to prevent write-amplification memory bloat", kind)
		}
	}
}
