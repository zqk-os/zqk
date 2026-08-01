package monitors

import (
	"context"
	"fmt"
	"os"

	"github.com/lanceman/zqk/pkg/healthcheck"
	"github.com/lanceman/zqk/pkg/storage"
)

// walBacklogMonitor reports the backlog between object WAL last seq and applied seq.
// This is a coarse signal for write-behind health and catch-up behavior.
type walBacklogMonitor struct{}

func (m *walBacklogMonitor) ID() string   { return "wal_backlog" }
func (m *walBacklogMonitor) Name() string { return "Object WAL backlog" }

func (m *walBacklogMonitor) Run(ctx context.Context, projectRoot string) (*healthcheck.Result, error) {
	if projectRoot == emptyValue {
		return &healthcheck.Result{Status: statusOK, Summary: summaryNoProjectRoot}, nil
	}
	// ReadAppliedSeq runs migration (legacy object_wal -> object_wal.wal) if needed
	_, _ = storage.ReadAppliedSeq(projectRoot)
	walPath := storage.GetWALPath(projectRoot)
	if _, err := os.Stat(walPath); err != nil {
		if os.IsNotExist(err) {
			// No WAL yet; treat as ok.
			return &healthcheck.Result{Status: statusOK, Summary: "no object WAL"}, nil
		}
		return &healthcheck.Result{
			Status:  statusDegraded,
			Summary: "object WAL stat error",
			Details: map[string]any{detailsErrorKey: err.Error()},
		}, nil
	}

	appliedSeq, err := storage.ReadAppliedSeq(projectRoot)
	if err != nil {
		return &healthcheck.Result{
			Status:  statusDegraded,
			Summary: "failed to read applied WAL seq",
			Details: map[string]any{detailsErrorKey: err.Error()},
		}, nil
	}

	// Use ReplayWALChunk with limit <=0 to scan to the end and get lastSeq. This streams
	// the file and does not materialize all records in memory.
	_, lastSeq, err := storage.ReplayWALChunk(projectRoot, 0, 0, func(rec *storage.WALRecord) error {
		// We don't need to process the record; ReplayWALChunk will track lastSeq for us.
		return nil
	})
	if err != nil {
		return &healthcheck.Result{
			Status:  statusDegraded,
			Summary: "failed to scan WAL for last seq",
			Details: map[string]any{detailsErrorKey: err.Error()},
		}, nil
	}

	if lastSeq < appliedSeq {
		lastSeq = appliedSeq
	}
	backlog := lastSeq - appliedSeq

	// Coarse thresholds to start with; to be tuned based on reference hardware and
	// maintenance capacity:
	//   ok:       backlog <= 5_000
	//   degraded: 5_000 < backlog <= 25_000
	//   fail:     backlog > 25_000
	status := statusOK
	if backlog > 25000 {
		status = statusFail
	} else if backlog > 5000 {
		status = statusDegraded
	}

	summary := fmt.Sprintf("backlog=%d applied_seq=%d last_seq=%d", backlog, appliedSeq, lastSeq)
	details := map[string]any{
		"backlog":     backlog,
		"applied_seq": appliedSeq,
		"last_seq":    lastSeq,
	}
	return &healthcheck.Result{Status: status, Summary: summary, Details: details}, nil
}

func init() {
	healthcheck.DefaultRegistry.Register(&walBacklogMonitor{})
}
