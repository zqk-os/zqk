package app

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestMaxOSThreads_DefaultAndClamp(t *testing.T) {
	t.Setenv(zqkenv.MaxOSThreads().Name(), "")
	if n := maxOSThreads(); n != defaultMaxOSThreads {
		t.Fatalf("default=%d want %d", n, defaultMaxOSThreads)
	}
	t.Setenv(zqkenv.MaxOSThreads().Name(), "8")
	if n := maxOSThreads(); n != minMaxOSThreads {
		t.Fatalf("low clamp=%d want %d", n, minMaxOSThreads)
	}
	t.Setenv(zqkenv.MaxOSThreads().Name(), "99999")
	if n := maxOSThreads(); n != maxMaxOSThreads {
		t.Fatalf("high clamp=%d want %d", n, maxMaxOSThreads)
	}
	t.Setenv(zqkenv.MaxOSThreads().Name(), "256")
	if n := maxOSThreads(); n != 256 {
		t.Fatalf("got %d want 256", n)
	}
}

func TestApplyDefaultCLIGoroutineBudget_InstallsWhenMissing(t *testing.T) {
	prev := goroutinelabels.DefaultBudget()
	t.Cleanup(func() { goroutinelabels.SetDefaultBudget(prev) })
	goroutinelabels.SetDefaultBudget(nil)
	applyDefaultCLIGoroutineBudget()
	got := goroutinelabels.DefaultBudget()
	if got == nil {
		t.Fatal("expected DefaultBudget after apply")
	}
	if got.MaxTotal() != defaultCLIGoroutineBudget {
		t.Fatalf("MaxTotal=%d want %d", got.MaxTotal(), defaultCLIGoroutineBudget)
	}
	applyDefaultCLIGoroutineBudget()
	if goroutinelabels.DefaultBudget() != got {
		t.Fatal("second apply must not replace an existing budget")
	}
}
