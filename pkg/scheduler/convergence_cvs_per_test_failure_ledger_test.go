package scheduler

import (
	"os"
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestCvsPerTestLedgerMaxFailuresFromEnv(t *testing.T) {
	key := zqkenv.CVSPerTestLedgerMaxFailures()
	t.Run("default", func(t *testing.T) {
		_ = os.Unsetenv(key)
		if got, want := cvsPerTestLedgerMaxFailuresFromEnv(), defaultCVSPerTestLedgerMaxFailures; got != want {
			t.Fatalf("got %d want %d", got, want)
		}
	})
	t.Run("zero_unlimited", func(t *testing.T) {
		t.Setenv(key, "0")
		if got := cvsPerTestLedgerMaxFailuresFromEnv(); got != 0 {
			t.Fatalf("got %d want 0", got)
		}
	})
	t.Run("explicit_cap", func(t *testing.T) {
		t.Setenv(key, "3")
		if got, want := cvsPerTestLedgerMaxFailuresFromEnv(), 3; got != want {
			t.Fatalf("got %d want %d", got, want)
		}
	})
	t.Run("invalid_falls_back", func(t *testing.T) {
		t.Setenv(key, "not-a-number")
		if got, want := cvsPerTestLedgerMaxFailuresFromEnv(), defaultCVSPerTestLedgerMaxFailures; got != want {
			t.Fatalf("got %d want %d", got, want)
		}
	})
	t.Run("negative_falls_back", func(t *testing.T) {
		t.Setenv(key, "-5")
		if got, want := cvsPerTestLedgerMaxFailuresFromEnv(), defaultCVSPerTestLedgerMaxFailures; got != want {
			t.Fatalf("got %d want %d", got, want)
		}
	})
}

func TestStringKeyedAnyMap_anyAny(t *testing.T) {
	t.Parallel()
	v := map[any]any{"a": float64(1)}
	m := stringKeyedAnyMap(v)
	if m == nil || m["a"] != float64(1) {
		t.Fatalf("got %#v", m)
	}
}
