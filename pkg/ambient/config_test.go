package ambient

import (
	"os"
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestLoadConfig(t *testing.T) {
	// Ensure isolated test environment
	os.Unsetenv(zqkenv.EnableAmbientWatcher())

	cfg := LoadConfig()
	if cfg.Enabled {
		t.Error("expected config to be disabled by default")
	}

	os.Setenv(zqkenv.EnableAmbientWatcher(), "1")
	defer os.Unsetenv(zqkenv.EnableAmbientWatcher())

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
