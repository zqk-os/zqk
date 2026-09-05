package storage

import (
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	"os"
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

// TestMain sets up package-wide test configuration for maximum parallelization and stability.
// Per-project CAS index queues ensure each test's storage uses its own queue (no cross-test
// index updates), so tests can run in parallel without flakiness.
// ZQK_SKIP_DELETE_AUDIT=1 skips createDeleteAuditEvent in tests so cascade delete tests
// pass deterministically (delete-audit Create path can still affect index visibility; see PRI-212).
//
// Default-on ZQK_TEST_METRICS_RECORDING for this package only: most tests assert on CAS/file-lock
// counters and metric paths. Other packages default to metricsrecording.Enabled() == false unless
// they opt in (see pkg/metricsrecording). Unset or override the env to exercise the off path.
func TestMain(m *testing.M) {
	if os.Getenv(zqkenv.TestMetricsRecording()) == emptyValue {
		_ = os.Setenv(zqkenv.TestMetricsRecording(), "1")
	}
	_ = os.Setenv("ZQK_ADMIN_TEST_"+"INIT_API_KEY", "mock_token")
	_ = os.Setenv(zqkenv.APIKey(), "ACC-1785920548450214012-68b850c0")
	// TestMain has no *testing.T, so these are process-scoped; the key list is shared with the
	// t.Setenv call sites via zqkenv so no test hardcodes the socket path.
	zqkenv.ApplyIsolatedStorageEnv(zqkenv.OSEnvSetter)
	caspkg.SetListingIndexWriteQueueFactoryToPerProjectRoot()
	_ = os.Setenv(zqkenv.SkipDeleteAudit(), "1")
	code := m.Run()
	caspkg.SetListingIndexWriteQueueFactory(nil) // reset so other packages or benchmarks are unaffected
	os.Exit(code)
}
