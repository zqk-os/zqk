package validation

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDefaultValidationTimeoutConfig(t *testing.T) {
	t.Parallel()
	c := DefaultValidationTimeoutConfig()
	if c.DefaultSeconds != 5 {
		t.Errorf("DefaultSeconds: got %d want 5 (fail-fast)", c.DefaultSeconds)
	}
	if c.StuckTimeoutSeconds != 30 {
		t.Errorf("StuckTimeoutSeconds: got %d want 30", c.StuckTimeoutSeconds)
	}
	// No legacy multi-minute kind budgets — hangs must surface quickly.
	if len(c.KindOverrides) != 0 {
		t.Errorf("KindOverrides: got %v want empty (project config owns overrides)", c.KindOverrides)
	}
	if c.TimeoutForKind("backlog_item") != 5*time.Second {
		t.Errorf("backlog_item timeout: got %v want 5s", c.TimeoutForKind("backlog_item"))
	}
	if c.TimeoutForKind("audit_event") != 5*time.Second {
		t.Errorf("audit_event timeout: got %v want 5s", c.TimeoutForKind("audit_event"))
	}
}

func TestLoadValidationTimeoutConfig_FromFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	const yaml = `
validation:
  per_object_timeout:
    default_seconds: 3
    kind_overrides:
      doc_entry: 10
  stuck_timeout_seconds: 20
`
	if err := fileutil.WriteFile(configPath, []byte(yaml), paths.FilePerm644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadValidationTimeoutConfig(configPath)
	if err != nil {
		t.Fatalf("LoadValidationTimeoutConfig: %v", err)
	}
	if cfg.DefaultSeconds != 3 {
		t.Errorf("DefaultSeconds: got %d want 3", cfg.DefaultSeconds)
	}
	// File overrides replace defaults; backlog_item must NOT re-inherit a 60s code default.
	if cfg.TimeoutForKind("backlog_item") != 3*time.Second {
		t.Errorf("backlog_item: got %v want 3s (no merge of legacy kind budgets)", cfg.TimeoutForKind("backlog_item"))
	}
	if cfg.TimeoutForKind("doc_entry") != 10*time.Second {
		t.Errorf("doc_entry: got %v want 10s", cfg.TimeoutForKind("doc_entry"))
	}
	if cfg.StuckTimeout() != 20*time.Second {
		t.Errorf("StuckTimeout: got %v want 20s", cfg.StuckTimeout())
	}
}

func TestLoadValidationTimeoutConfig_EmptyOverridesDoNotMergeLegacy(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	const yaml = `
validation:
  per_object_timeout:
    default_seconds: 4
    kind_overrides: {}
`
	if err := fileutil.WriteFile(configPath, []byte(yaml), paths.FilePerm644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadValidationTimeoutConfig(configPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := cfg.TimeoutForKind("requirement"); got != 4*time.Second {
		t.Fatalf("requirement: got %v want 4s", got)
	}
}

func TestValidationTimeoutForKind_UsesConfig(t *testing.T) {
	t.Parallel()
	// Uses singleton from project/default; assert fail-fast ceiling for unknown kinds.
	d := validationTimeoutForKind("unknown_kind_xyz")
	if d > 15*time.Second {
		t.Errorf("unknown kind timeout %v is too loose for fail-fast posture", d)
	}
}

func TestValidationTimeoutConfig_StuckTimeout(t *testing.T) {
	t.Parallel()
	c := DefaultValidationTimeoutConfig()
	if c.StuckTimeout() != 30*time.Second {
		t.Errorf("StuckTimeout default: got %v want 30s", c.StuckTimeout())
	}
	c.StuckTimeoutSeconds = 12
	if c.StuckTimeout() != 12*time.Second {
		t.Errorf("StuckTimeout override: got %v want 12s", c.StuckTimeout())
	}
}
