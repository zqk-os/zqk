package scheduler

import (
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/paths"
)

// ResolveCVSRollupLatestJSONPath returns the filesystem path to read rollup JSON after cvs_convergence_orchestrate.sh.
// envCVSORCHRollupOut is optional CVS_ORCH_ROLLUP_OUT (same as the shell script): absolute path, or repo-relative
// (e.g. ".zqk/logs/drift/cvs_rollup_latest.json"). Empty env uses the repo default layout under paths.ProjectDataDir/logs/drift/.
// Other teams or CI layouts can point rollup output elsewhere without forking Go code.
func ResolveCVSRollupLatestJSONPath(projectRoot, envCVSORCHRollupOut string) string {
	v := strings.TrimSpace(envCVSORCHRollupOut)
	if v != "" {
		if filepath.IsAbs(v) {
			return filepath.Clean(v)
		}
		v = strings.TrimPrefix(v, "./")
		return filepath.Join(projectRoot, filepath.FromSlash(v))
	}
	return filepath.Join(
		projectRoot,
		paths.ProjectDataDir,
		paths.LogsDir,
		paths.LogsDriftSubdir,
		convergenceOrchestrateRollupLatestFileName,
	)
}
