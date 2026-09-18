package scheduler

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// scrubDaemonInheritEnv drops IDE/MCP-subprocess env that must not reach a long-lived
// scheduler daemon. ZQK_PARENT_PID arms the CLI parent-death watcher (os.Exit when
// Getppid no longer matches); ZQK_MCP_ACCOUNT_ID pollutes in-process CLI. Sets
// ZQK_IS_PARENT_ZQK so nested ExecuteContext skips the child idle watchdog.
// TRACK: BLI-1784969955962654000-dc689643 — same class as mcp daemon scrub.
func scrubDaemonInheritEnv(env []string) []string {
	drop := map[string]struct{}{
		zqkenv.ParentPID().Name():    {},
		zqkenv.MCPAccountID().Name(): {},
		zqkenv.IsParentZqk().Name():  {},
	}
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		key, _, ok := strings.Cut(e, "=")
		if ok {
			if _, skip := drop[key]; skip {
				continue
			}
		}
		out = append(out, e)
	}
	out = append(out, zqkenv.IsParentZqk().Name()+"=1")
	return out
}

// scrubSchedulerDaemonProcessEnv applies scrubDaemonInheritEnv to this process
// before the foreground daemon loop (agent shells often leak MCP child env).
func scrubSchedulerDaemonProcessEnv() {
	_ = zqkenv.ParentPID().Unset()
	_ = zqkenv.MCPAccountID().Unset()
	_ = zqkenv.IsParentZqk().Set("1")
}
