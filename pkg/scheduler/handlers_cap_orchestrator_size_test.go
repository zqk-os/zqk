package scheduler

import (
	"bufio"
	"path/filepath"
	"runtime"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// CEF R2 BLI-CEF-R2-ARCH-GOD-SCHEDULER: keep the former god file below 2k lines.
const capOrchestratorMaxLines = 2000

func TestCapOrchestratorHandlerFileStaysUnder2k(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	path := filepath.Join(filepath.Dir(thisFile), "handlers_cap_orchestrator.go")
	f, err := fileutil.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	n := 0
	s := bufio.NewScanner(f)
	for s.Scan() {
		n++
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if n > capOrchestratorMaxLines {
		t.Errorf("handlers_cap_orchestrator.go has %d lines; want <= %d (extract more stages)", n, capOrchestratorMaxLines)
	}
}

// TestCapOrchestrator_BLI_CEF_R17_GOD_SCHEDULER_001 verifies that the CAP orchestrator god file
// is decomposed into bounded handler modules staying under the line budget (BLI-CEF-R17-GOD-SCHEDULER-001).
func TestCapOrchestrator_BLI_CEF_R17_GOD_SCHEDULER_001(t *testing.T) {
	TestCapOrchestratorHandlerFileStaysUnder2k(t)
}
