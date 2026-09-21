package storage

import (
	"os"

	"github.com/zqk-os/zqk/pkg/config"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"testing"
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
	if !config.TestingMetricsRecording().Safe() {
		_ = zqkenv.TestMetricsRecording().Set("1")
	}
	_ = os.Setenv(zqkenv.AdminBrandKey("TEST_INIT_API_KEY"), "mock_token")
	_ = zqkenv.APIKey().Set("ACC-SYSTEM")
	// TestMain has no *testing.T, so these are process-scoped; the key list is shared with the
	// t.Setenv call sites via zqkenv so no test hardcodes the socket path.
	zqkenv.ApplyIsolatedStorageEnv(zqkenv.OSEnvSetter)
	caspkg.SetListingIndexWriteQueueFactoryToPerProjectRoot()
	_ = zqkenv.SkipDeleteAudit().Set("1")
	code := m.Run()
	caspkg.SetListingIndexWriteQueueFactory(nil) // reset so other packages or benchmarks are unaffected
	os.Exit(code)
}
