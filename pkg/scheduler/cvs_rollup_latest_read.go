package scheduler

import (
	"bytes"
	"encoding/json"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

type rollupLatestFileV1 struct {
	SchemaVersion              string `json:"schema_version"`
	ParentConvergenceSessionID string `json:"parent_convergence_session_id"`
	RollupStatus               string `json:"rollup_status"`
	ReadyForParentCompletion   bool   `json:"ready_for_parent_completion"`
}

// ReadCVSRollupLatestSummary reads rollup JSON at rollupJSONPath when parent_convergence_session_id matches sessionID.
// rollupJSONPath is typically ResolveCVSRollupLatestJSONPath(projectRoot, envLookup(job, EnvKeyCVSOrchestrateRollupOut)).
// Returns matched=false when the file is missing, malformed, or describes a different session (stale file).
func ReadCVSRollupLatestSummary(sessionID, rollupJSONPath string) (rollupStatus string, readyForParent bool, matched bool, err error) {
	if sessionID == emptyValue || rollupJSONPath == emptyValue {
		return "", false, false, nil
	}
	data, err := fileutil.ReadFile(rollupJSONPath)
	if err != nil || len(data) == 0 {
		return "", false, false, nil
	}
	data = bytes.TrimSpace(data)
	var decoded rollupLatestFileV1
	if jsonErr := json.Unmarshal(data, &decoded); jsonErr != nil {
		return "", false, false, jsonErr
	}
	if decoded.ParentConvergenceSessionID != sessionID {
		return "", false, false, nil
	}
	return decoded.RollupStatus, decoded.ReadyForParentCompletion, true, nil
}
