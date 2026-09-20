package migration

import (
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// HistoryFileName is the name of the migration history file under state dir.
const HistoryFileName = "migration_history.json"

// HistoryEntry records one completed migration run.
type HistoryEntry struct {
	MigrationID string    `json:"migration_id"`
	CompletedAt time.Time `json:"completed_at"`
}

// MigrationHistory is the persisted list of completed migrations.
type MigrationHistory struct {
	Entries []HistoryEntry `json:"entries"`
}

// historyFilePath returns the absolute path to the migration history file.
func historyFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, HistoryFileName)
}

// LoadHistory reads the migration history from the project state directory.
// If the file does not exist or is empty, returns a history with no entries.
func LoadHistory(projectRoot string) (*MigrationHistory, error) {
	p := historyFilePath(projectRoot)
	data, err := fileutil.ReadFile(p)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return &MigrationHistory{Entries: nil}, nil
		}
		return nil, errfmt.Newf("read migration history").Wrap(err)
	}
	var h MigrationHistory
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, errfmt.Newf("parse migration history").Wrap(err)
	}
	if h.Entries == nil {
		h.Entries = nil
	}
	return &h, nil
}

// HasRun returns whether the given migration ID has been recorded as completed.
func HasRun(projectRoot, migrationID string) (bool, error) {
	h, err := LoadHistory(projectRoot)
	if err != nil {
		return false, err
	}
	for _, e := range h.Entries {
		if e.MigrationID == migrationID {
			return true, nil
		}
	}
	return false, nil
}

// RecordSuccess appends a completed migration to history and persists the file.
// Creates the state directory if it does not exist.
func RecordSuccess(projectRoot, migrationID string) error {
	if projectRoot == emptyValue || migrationID == emptyValue {
		return errfmt.Errorf("project root and migration id are required")
	}
	stateDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.MkdirAll(stateDir, paths.DirPerm750); err != nil {
		return errfmt.Newf("create state dir").Wrap(err)
	}

	h, err := LoadHistory(projectRoot)
	if err != nil {
		return err
	}
	h.Entries = append(h.Entries, HistoryEntry{
		MigrationID: migrationID,
		CompletedAt: time.Now().UTC(),
	})
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return errfmt.Newf("encode migration history").Wrap(err)
	}
	p := historyFilePath(projectRoot)
	if err := fileutil.WriteSecureFile(p, data); err != nil {
		return errfmt.Newf("write migration history").Wrap(err)
	}
	return nil
}
