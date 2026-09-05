package tde

import (
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/walutil"
)

const (
	tdeWALFileName      = "tde_staging.wal"
	tdeWALCheckpointExt = ".checkpoint"
	tdeWALMaxLineSize   = 2 * 1024 * 1024
)

// StagingWAL is a Write-Ahead Log for TDE envelopes.
type StagingWAL struct {
	inner *walutil.JSONLineWAL[Envelope]
}

// NewStagingWAL creates or opens the TDE staging WAL.
func NewStagingWAL(projectRoot string) (*StagingWAL, error) {
	inner, err := walutil.OpenJSONLineWAL[Envelope](
		projectRoot,
		tdeWALFileName,
		tdeWALCheckpointExt,
		tdeWALMaxLineSize,
	)
	if err != nil {
		return nil, err
	}
	return &StagingWAL{inner: inner}, nil
}

// Stage appends a new TDE envelope to the WAL with StatusStaged.
func (w *StagingWAL) Stage(env Envelope) error {
	env.Status = StatusStaged
	if env.CreatedAt.IsZero() {
		env.CreatedAt = time.Now().UTC()
	}
	return w.inner.Append(func(seq int64, ts time.Time) Envelope {
		env.Seq = seq
		return env
	}, tdeWALMaxLineSize)
}

// Revoke appends a status change to StatusCancelled for the given envelope ID.
func (w *StagingWAL) Revoke(id string) error {
	env := Envelope{
		ID:     id,
		Status: StatusCancelled,
	}
	return w.inner.Append(func(seq int64, ts time.Time) Envelope {
		env.Seq = seq
		return env
	}, tdeWALMaxLineSize)
}

// MarkCommitted appends a status change to StatusCommitted for the given envelope ID.
func (w *StagingWAL) MarkCommitted(id string) error {
	env := Envelope{
		ID:     id,
		Status: StatusCommitted,
	}
	return w.inner.Append(func(seq int64, ts time.Time) Envelope {
		env.Seq = seq
		return env
	}, tdeWALMaxLineSize)
}

// Sync flushes the buffer and syncs the WAL file to disk.
func (w *StagingWAL) Sync() error {
	return w.inner.Sync()
}

// Close closes the WAL file (flushes first).
func (w *StagingWAL) Close() error {
	return w.inner.Close()
}

// GetWALPath returns the path to the TDE WAL file for the given project root.
func GetWALPath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.WalDir, tdeWALFileName)
}

// LoadActive builds the current state of active (staged) envelopes by replaying the WAL.
func LoadActive(projectRoot string) (map[string]Envelope, error) {
	active := make(map[string]Envelope)

	err := walutil.ReplayFrom[Envelope](
		GetWALPath(projectRoot),
		tdeWALMaxLineSize,
		0, // start from the beginning
		func(b []byte) (*Envelope, error) {
			var e Envelope
			if err := json.Unmarshal(b, &e); err != nil {
				return nil, err
			}
			return &e, nil
		},
		func(e *Envelope) int64 { return e.Seq },
		func(e *Envelope) error {
			if e.Status == StatusStaged {
				// Initialize or update the active envelope
				if existing, ok := active[e.ID]; ok {
					// We might be updating it, but usually Stage is only called once per ID
					// In case of multiple Stage calls, we'll just overwrite
					_ = existing
				}
				active[e.ID] = *e
			} else {
				// StatusCancelled or StatusCommitted means it's no longer active
				delete(active, e.ID)
			}
			return nil
		},
	)

	return active, err
}
