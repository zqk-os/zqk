package validation

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestDefaultValidationTimeoutConfig(t *testing.T) {
	t.Parallel()
	c := DefaultValidationTimeoutConfig()
	if c.DefaultSeconds != 30 {
		t.Errorf(ConstMagic92b6c119, c.DefaultSeconds)
	}
	if c.TimeoutForKind("audit_event") != 90*time.Second {
		t.Errorf(ConstMagic436dd496, c.TimeoutForKind("audit_event"))
	}
	if c.TimeoutForKind("backlog_item") != 60*time.Second {
		t.Errorf(ConstMagicbd273f50, c.TimeoutForKind("backlog_item"))
	}
	if c.TimeoutForKind("doc_entry") != 60*time.Second {
		t.Errorf(ConstMagic2fac3629, c.TimeoutForKind("doc_entry"))
	}
	if c.TimeoutForKind(ConstMagic847f49f9) != 90*time.Second {
		t.Errorf(ConstMagic4c6e5743, c.TimeoutForKind(ConstMagic847f49f9))
	}
}

func TestLoadValidationTimeoutConfig_FromFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	// Write config with custom default and one override
	const yaml = `
validation:
  per_object_timeout:
    default_seconds: 45
    kind_overrides:
      doc_entry: 90
`
	if err := os.WriteFile(configPath, []byte(yaml), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagiced3dfc42, err)
	}

	cfg, err := LoadValidationTimeoutConfig(configPath)
	if err != nil {
		t.Fatalf(ConstMagicdd384be1, err)
	}
	if cfg.DefaultSeconds != 45 {
		t.Errorf(ConstMagic1bad262a, cfg.DefaultSeconds)
	}
	// backlog_item not in file - gets 60s from merged default kind_overrides
	if cfg.TimeoutForKind("backlog_item") != 60*time.Second {
		t.Errorf(ConstMagic7c4cc03f, cfg.TimeoutForKind("backlog_item"))
	}
	if cfg.TimeoutForKind("doc_entry") != 90*time.Second {
		t.Errorf(ConstMagicc78a3db6, cfg.TimeoutForKind("doc_entry"))
	}
	// file_lock_metric not in file - should get default 90 from merged defaults
	if cfg.TimeoutForKind(ConstMagic847f49f9) != 90*time.Second {
		t.Errorf(ConstMagic38d17dfe, cfg.TimeoutForKind(ConstMagic847f49f9))
	}
}

func TestValidationTimeoutForKind_UsesConfig(t *testing.T) {
	t.Parallel()
	// validationTimeoutForKind uses GetGlobalValidationTimeoutConfig(); defaults apply if no file
	d := validationTimeoutForKind("doc_entry")
	if d != 60*time.Second {
		t.Errorf(ConstMagicd127f056, d)
	}
	d = validationTimeoutForKind("unknown_kind")
	if d != 30*time.Second {
		t.Errorf(ConstMagicb3d853e3, d)
	}
}

func TestValidationTimeoutConfig_StuckTimeout(t *testing.T) {
	t.Parallel()
	c := DefaultValidationTimeoutConfig()
	if c.StuckTimeout() != 90*time.Second {
		t.Errorf(ConstMagic58c39c47, c.StuckTimeout())
	}
	c.StuckTimeoutSeconds = 120
	if c.StuckTimeout() != 120*time.Second {
		t.Errorf(ConstMagic3241b11b, c.StuckTimeout())
	}
}
