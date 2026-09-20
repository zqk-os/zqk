package scheduler

import (
	"encoding/json"
	"path/filepath"
	"sync"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// CVSOrchestrateRunV1 is the JSON shape for one line in cvs_orchestrate_runs.jsonl
// (schema_version cvs_orchestrate_run_v1). Kept in sync with scripts/cvs_convergence_orchestrate.sh
// and docs/architecture/CONVERGENCE_ORCHESTRATION_AND_NESTED_CVS.md.
type CVSOrchestrateRunV1 struct {
	SchemaVersion  string `json:"schema_version"`
	EndedAtMS      int64  `json:"ended_at_ms"`
	DurationMS     int64  `json:"duration_ms"`
	CvsID          string `json:"cvs_id"`
	ExitCode       int    `json:"exit_code"`
	Stage          string `json:"stage"`
	PersistSkipped bool   `json:"persist_skipped"`
	PersistFailed  bool   `json:"persist_failed"`
	RollupStatus   string `json:"rollup_status,omitempty"`
	// ReadyForParentCompletion is nil when omitted (unknown); false/true when set.
	ReadyForParentCompletion *bool `json:"ready_for_parent_completion,omitempty"`
}

var cvsOrchestrateRunEventsMu sync.Mutex

// CVSOrchestrateRunSchemaVersion is the required schema_version for JSONL lines and stdin to record-cvs-orchestrate-run.
const CVSOrchestrateRunSchemaVersion = "cvs_orchestrate_run_v1"

// CVSOrchestrateRunsFilePath returns the default append-only JSONL path for orchestrate runs:
// .zqk/logs/scheduler/cvs_orchestrate_runs.jsonl
func CVSOrchestrateRunsFilePath(projectRoot string) string {
	return filepath.Join(JobLogsBaseDir(projectRoot), "cvs_orchestrate_runs.jsonl")
}

// AppendCVSOrchestrateRunV1 validates schema_version, marshals one compact JSON line, and appends.
// jsonlPath empty uses CVSOrchestrateRunsFilePath(projectRoot).
func AppendCVSOrchestrateRunV1(projectRoot, jsonlPath string, rec *CVSOrchestrateRunV1) error {
	if rec == nil {
		return errfmt.Errorf("cvs_orchestrate_run: nil record")
	}
	if projectRoot == emptyValue {
		return errfmt.Errorf("cvs_orchestrate_run: project root required")
	}
	if rec.SchemaVersion != CVSOrchestrateRunSchemaVersion {
		return errInvalidOrchestrateSchema(rec.SchemaVersion)
	}
	if jsonlPath == emptyValue {
		jsonlPath = CVSOrchestrateRunsFilePath(projectRoot)
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	cvsOrchestrateRunEventsMu.Lock()
	defer cvsOrchestrateRunEventsMu.Unlock()
	if err := fileutil.EnsureDir(filepath.Dir(jsonlPath)); err != nil {
		return err
	}
	f, err := fileutil.OpenFile(jsonlPath, fileutil.O_WRONLY|fileutil.O_CREATE|fileutil.O_APPEND, paths.FilePerm600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		if errClose := f.Close(); errClose != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			SLog(logger).Debug("Failed to close CVS orchestrate run log after failed write").WithError(errClose).Log()
		}
		return err
	}
	if _, err := f.WriteString("\n"); err != nil {
		if errClose := f.Close(); errClose != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			SLog(logger).Debug("Failed to close CVS orchestrate run log after failed string write").WithError(errClose).Log()
		}
		return err
	}
	return f.Close()
}

func errInvalidOrchestrateSchema(got string) error {
	return &invalidOrchestrateSchemaError{got: got}
}

type invalidOrchestrateSchemaError struct {
	got string
}

func (e *invalidOrchestrateSchemaError) Error() string {
	return "cvs_orchestrate_run: schema_version must be " + CVSOrchestrateRunSchemaVersion + ", got " + e.got
}
