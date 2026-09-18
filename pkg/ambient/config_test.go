package ambient

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestLoadConfig(t *testing.T) {
	// Ensure isolated test environment
	_ = zqkenv.EnableAmbientWatcher().Unset()

	cfg := LoadConfig()
	if cfg.Enabled {
		t.Error("expected config to be disabled by default")
	}

	_ = zqkenv.EnableAmbientWatcher().Set("1")
	defer zqkenv.EnableAmbientWatcher().Unset()

	cfg = LoadConfig()
	if !cfg.Enabled {
		t.Error("expected config to be enabled when env var is set")
	}
}

func TestResolveService(t *testing.T) {
	svc := ResolveService(Config{Enabled: false})
	if _, ok := svc.(*NoopService); !ok {
		t.Errorf("expected NoopService when disabled, got %T", svc)
	}

	// Currently falls back to NoopService even when enabled until implemented
	svcEnabled := ResolveService(Config{Enabled: true})
	if _, ok := svcEnabled.(*NoopService); !ok {
		t.Errorf("expected NoopService fallback when enabled, got %T", svcEnabled)
	}
}
