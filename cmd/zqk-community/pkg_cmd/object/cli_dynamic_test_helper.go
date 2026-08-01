package object

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/storage"
)

// WithCLIOperation marks a context as a CLI operation (re-export from storage)
var WithCLIOperation = storage.WithCLIOperation

var testIDCounter int64

// setupCLITestEnvironment is now an alias for SetupTestEnvironment for backward compatibility.
// It returns (tmpDir, cliBinary) to match the old signature.
func setupCLITestEnvironment(t *testing.T) (tmpDir, cliBinary string) {
	testEnv := SetupTestEnvironment(t)
	return testEnv.GetTestRoot(), testEnv.CLIBinary
}

// generateTestID generates a simple test ID based on kind.
func generateTestID(kind string) string {
	prefix := getPrefixForKind(kind)
	c := atomic.AddInt64(&testIDCounter, 1)
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().Unix(), c)
}

func getPrefixForKind(kind string) string {
	prefixes := map[string]string{
		pplanKindBacklogItem: "BLI",
		"goal":               "GOAL",
		"criteria":           "CRIT",
		"requirement":        "REQ",
	}
	prefix := prefixes[kind]
	if prefix == emptyValue {
		prefix = "TEST"
	}
	return prefix
}

func getInitialStatus(kind string) string {
	statuses := map[string]string{
		pplanKindBacklogItem: objectStatusExploring,
		"goal":               objectStatusActive,
		"criteria":           objectStatusNotStarted,
		"requirement":        "planned",
	}
	status := statuses[kind]
	if status == emptyValue {
		status = objectStatusActive
	}
	return status
}

// findProjectRootForTest finds the project root by looking for go.mod
func findProjectRootForTest(t *testing.T) string {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root (go.mod)")
		}
		dir = parent
	}
}
