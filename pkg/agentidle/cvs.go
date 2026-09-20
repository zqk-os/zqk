package agentidle

import (
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
)

// RecordCVSSnapshot writes the current total idle time to the convergence session's after_state_snapshot
func RecordCVSSnapshot(store *FileStore, cvsID string) error {
	records, err := store.GetAllRecords()
	if err != nil {
		return err
	}

	var total time.Duration
	for _, duration := range records {
		total += duration
	}

	// Call zqk object update via exec
	cmd := execwrap.Command(paths.CLIName(), "object", "update", cvsID, "--field", "after_state_snapshot.baseline_idle="+total.String(), "--field", "after_state_snapshot.captured_at="+time.Now().UTC().Format(time.RFC3339))
	return cmd.Run()
}
