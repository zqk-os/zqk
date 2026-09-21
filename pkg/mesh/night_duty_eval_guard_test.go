package mesh_test

import (
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestNightDutyEvalScriptGuard(t *testing.T) {
	// Path guard: prevent the hardened night-duty-eval.py from being overwritten
	// by a get-only copy (which lacked list-first logic and caused CAS stubs).

	scriptPath := filepath.Join("..", "..", "scripts", "mesh", "night-duty-eval.py")
	if !fileutil.Exists(scriptPath) {
		t.Skip("night-duty-eval.py not present in open-core distribution, skipping guard")
	}
	content, err := fileutil.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("Failed to read night-duty-eval.py: %v", err)
	}

	scriptStr := string(content)

	if !strings.Contains(scriptStr, "list-first") {
		t.Errorf("Path Guard Failed: scripts/mesh/night-duty-eval.py appears to be an old get-only copy! Missing 'list-first' logic.")
	}

	if !strings.Contains(scriptStr, "object_is_stub") {
		t.Errorf("Path Guard Failed: scripts/mesh/night-duty-eval.py appears to be an old get-only copy! Missing 'object_is_stub' check.")
	}

	if !strings.Contains(scriptStr, "\"object\", \"list\"") {
		t.Errorf("Path Guard Failed: scripts/mesh/night-duty-eval.py must perform an 'object list' before 'object get' to avoid CAS stubs overnight.")
	}
}

func TestNightDutyLatchHygieneGuard(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "scripts", "mesh", "night-duty-tick.sh")
	if !fileutil.Exists(scriptPath) {
		t.Skip("night-duty-tick.sh not present in open-core distribution, skipping guard")
	}
	content, err := fileutil.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("Failed to read night-duty-tick.sh: %v", err)
	}

	scriptStr := string(content)

	if !strings.Contains(scriptStr, "latch_mtime=") {
		t.Errorf("Path Guard Failed: scripts/mesh/night-duty-tick.sh missing latch modification time check.")
	}
	if !strings.Contains(scriptStr, "age=$((now - latch_mtime))") {
		t.Errorf("Path Guard Failed: scripts/mesh/night-duty-tick.sh missing age calculation for latch.")
	}
	if !strings.Contains(scriptStr, "rm -f \"$DONE_FILE\"") || !strings.Contains(scriptStr, "age > max_age") {
		t.Errorf("Path Guard Failed: scripts/mesh/night-duty-tick.sh missing auto-expire clear logic for DISABLED latch.")
	}
	if !strings.Contains(scriptStr, "feed emit-status") || !strings.Contains(scriptStr, "NIGHT-DUTY RED: DISABLED latch is stuck") {
		t.Errorf("Path Guard Failed: scripts/mesh/night-duty-tick.sh missing TPM page for stuck DISABLED latch.")
	}
}
