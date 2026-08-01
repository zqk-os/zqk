package scheduler

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func stripSchedulerEnvKey(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// subprocessEnvForCVSOrchestrateRollupOnly returns a copy of os.Environ() with parent values for
// ZQK_PROJECT_ROOT and CVS_ORCH_SKIP_PERSIST stripped and replaced so the orchestrate shell script
// sees projectRoot consistently with exec.Cmd.Dir (avoids inheriting a stale IDE/shell root).
// When rollupOutOptional is non-empty, CVS_ORCH_ROLLUP_OUT is stripped from the parent env and set
// so the script writes rollup JSON to the same path ReadCVSRollupLatestSummary reads (job YAML env).
func subprocessEnvForCVSOrchestrateRollupOnly(projectRoot string, rollupOutOptional string) []string {
	env := os.Environ()
	env = stripSchedulerEnvKey(env, zqkenv.ProjectRoot())
	env = stripSchedulerEnvKey(env, EnvKeyCVSOrchestrateSkipPersist)
	env = stripSchedulerEnvKey(env, EnvKeyCVSOrchestrateRollupOut)
	env = append(env,
		EnvKeyCVSOrchestrateSkipPersist+"="+EnvValueCVSOrchestrateSkipPersist,
		zqkenv.ProjectRoot()+"="+projectRoot,
	)
	if strings.TrimSpace(rollupOutOptional) != "" {
		env = append(env, EnvKeyCVSOrchestrateRollupOut+"="+rollupOutOptional)
	}
	return env
}

func convergenceOrchestrateScriptPath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ScriptsDir, convergenceOrchestrateScriptFile)
}

func newCVSOrchestrateRollupOnlyCommand(ctx context.Context, projectRoot, sessionID, rollupOutFromJob string) *exec.Cmd {
	//nolint:gosec
	cmd := exec.CommandContext(ctx,
		convergenceOrchestrateScriptPath(projectRoot),
		sessionID,
		convergenceOrchestrateArgNoFailOnGates,
	)
	cmd.Dir = projectRoot
	cmd.Env = subprocessEnvForCVSOrchestrateRollupOnly(projectRoot, rollupOutFromJob)
	return cmd
}

// runCVSOrchestrateRollupCmd runs the rollup-only orchestrate subprocess (swap in tests).
var runCVSOrchestrateRollupCmd = func(ctx context.Context, projectRoot, sessionID, rollupOutFromJob string) ([]byte, error) {
	cmd := newCVSOrchestrateRollupOnlyCommand(ctx, projectRoot, sessionID, rollupOutFromJob)
	return cmd.CombinedOutput()
}
