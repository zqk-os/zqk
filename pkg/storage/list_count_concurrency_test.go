package storage

import (
	"runtime"
	"testing"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestGetListReadWorkers_DefaultBounded(t *testing.T) {
	t.Setenv(zqkenv.ListReadWorkers().Name(), "")
	n := getListReadWorkers()
	if n < 4 || n > 8 {
		t.Fatalf("default list read workers = %d, want 4..8 (GOMAXPROCS=%d)", n, runtime.GOMAXPROCS(0))
	}
}

func TestGetListReadWorkers_EnvOverrideClamped(t *testing.T) {
	t.Setenv(zqkenv.ListReadWorkers().Name(), "4")
	if n := getListReadWorkers(); n != 4 {
		t.Fatalf("got %d, want 4", n)
	}
	t.Setenv(zqkenv.ListReadWorkers().Name(), "999")
	if n := getListReadWorkers(); n != 16 {
		t.Fatalf("got %d, want clamp 16", n)
	}
}

func TestDefaultListCountMaxConcurrent(t *testing.T) {
	if defaultListCountMaxConcurrent != 4 {
		t.Fatalf("defaultListCountMaxConcurrent = %d, want 4 (OS-thread cap)", defaultListCountMaxConcurrent)
	}
}
