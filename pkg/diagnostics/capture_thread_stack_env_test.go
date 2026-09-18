package diagnostics

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestThreadStackSkipThresholdFromEnv(t *testing.T) {
	key := zqkenv.DiagnosticsThreadStackSkipMinGoroutines()
	def := threadDumpSkipRuntimeStackMinGoroutines

	t.Run("unset uses default", func(t *testing.T) {
		t.Setenv(key.Name(), "")
		if got := threadStackSkipThresholdFromEnv(); got != def {
			t.Fatalf("got %d want %d", got, def)
		}
	})

	t.Run("invalid falls back to default", func(t *testing.T) {
		t.Setenv(key.Name(), "not-an-int")
		if got := threadStackSkipThresholdFromEnv(); got != def {
			t.Fatalf("got %d want %d", got, def)
		}
	})

	t.Run("zero disables threshold skip", func(t *testing.T) {
		t.Setenv(key.Name(), "0")
		if got := threadStackSkipThresholdFromEnv(); got != 0 {
			t.Fatalf("got %d want 0", got)
		}
	})

	t.Run("negative disables threshold skip", func(t *testing.T) {
		t.Setenv(key.Name(), "-5")
		if got := threadStackSkipThresholdFromEnv(); got != 0 {
			t.Fatalf("got %d want 0", got)
		}
	})

	t.Run("positive custom threshold", func(t *testing.T) {
		t.Setenv(key.Name(), "9000")
		if got := threadStackSkipThresholdFromEnv(); got != 9000 {
			t.Fatalf("got %d want 9000", got)
		}
	})

	t.Run("numeric value trimspace", func(t *testing.T) {
		t.Setenv(key.Name(), "  12000  ")
		if got := threadStackSkipThresholdFromEnv(); got != 12000 {
			t.Fatalf("got %d want 12000", got)
		}
	})
}
