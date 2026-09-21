package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/adapters"
	"github.com/zqk-os/zqk/pkg/adapters/golang"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/swarm"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// seatWorkerMaxRepairs is how many extra turns the model gets to fix work
	// the gate rejected.
	seatWorkerMaxRepairs = 2
)

// seatWorkerCompletionGate is the Go toolchain adapter. Other languages
// implement adapters.CompletionGate; seating does not hard-code .go / go test.
var seatWorkerCompletionGate adapters.CompletionGate = golang.CompletionGate{}

// revertSeatWrites undoes the files a rejected run wrote so a seat cannot
// leave the tree broken for the next agent or human. Tracked files go back to
// HEAD; files the run created are removed.
func revertSeatWrites(ctx context.Context, root string, files []string) error {
	var failures []string
	for _, file := range files {
		// Kernel CAS instance data is never reverted through git.
		if strings.HasPrefix(file, paths.ProcessDir+"/") || strings.HasPrefix(file, paths.ProjectDataDir+"/") {
			continue
		}
		tracked := execwrap.CommandContext(ctx, "git", "ls-files", "--error-unmatch", file)
		tracked.Dir = root
		if err := tracked.Run(); err == nil {
			restore := execwrap.CommandContext(ctx, "git", "checkout", "--", file)
			restore.Dir = root
			if out, err := restore.CombinedOutput(); err != nil {
				failures = append(failures, fmt.Sprintf("%s: %s", file, strings.TrimSpace(string(out))))
			}
			continue
		}
		if err := fileutil.Remove(filepath.Join(root, file)); err != nil && !fileutil.IsNotExist(err) {
			failures = append(failures, fmt.Sprintf("%s: %v", file, err))
		}
	}
	if len(failures) > 0 {
		return errfmt.Errorf("revert seat writes: %s", strings.Join(failures, "; "))
	}
	return nil
}

// verifyWorkAsCompletion is the seat-worker completion gate: a coding ATK
// cannot complete on narrative. A successful write tool call is the minimum
// evidence; language adapters decide compile/test for the files they own.
func verifyWorkAsCompletion(ctx context.Context, root string, history []swarm.ToolCallRecord) (string, error) {
	if !swarm.HistoryHasMutationWrite(history) {
		return "No mutation tool evidence. Successfully invoke write_code or write_file before completing. Narrative is not execution evidence.", nil
	}
	return seatWorkerCompletionGate.Verify(ctx, root, history)
}
