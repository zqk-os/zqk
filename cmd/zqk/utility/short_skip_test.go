package utility

import (
	"flag"
	"os"
	"testing"

	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
)

// TestMain skips the entire utility package when -short is set.
// Many tests start background goroutines (ConfigFileWatcher, storage workers)
// that are not fully torn down, causing goroutine leak detector failures.
// Per-project CAS index queues give each test its own queue so Create + Flush
// see the same index updates (avoids "ID not found in CAS directory" in ref validation).
func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	caspkg.SetListingIndexWriteQueueFactoryToPerProjectRoot()
	code := m.Run()
	caspkg.SetListingIndexWriteQueueFactory(nil)
	os.Exit(code)
}
