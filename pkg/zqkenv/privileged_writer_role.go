package zqkenv

import (
	"os"
	"strings"
)

// Privileged-writer process identity. Env (IS_DAEMON) is belt-and-suspenders;
// argv `object daemon` is the source of truth that cannot lose a Setenv race.
const (
	PrivilegedWriterCLIGroup = "object"
	PrivilegedWriterCLIVerb  = "daemon"
)

// PrivilegedWriterDaemonCommandFragment is the CLI path fragment used by
// timeout hooks and argv matching (`object daemon`).
func PrivilegedWriterDaemonCommandFragment() string {
	return PrivilegedWriterCLIGroup + " " + PrivilegedWriterCLIVerb
}

// CommandArgs returns process argv. Tests swap os.Args and restore it.
func CommandArgs() []string {
	return os.Args
}

// PrivilegedWriterDaemonArgv reports whether args are the writer daemon
// (`… object daemon`), ignoring argv0 and flag tokens.
func PrivilegedWriterDaemonArgv(args []string) bool {
	tokens := commandTokensSkippingFlags(args)
	for i := 0; i < len(tokens)-1; i++ {
		if tokens[i] == PrivilegedWriterCLIGroup && tokens[i+1] == PrivilegedWriterCLIVerb {
			return true
		}
	}
	return false
}

func commandTokensSkippingFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for i, a := range args {
		if i == 0 || strings.HasPrefix(a, "-") {
			continue
		}
		out = append(out, a)
	}
	return out
}

// PrivilegedWriterDaemonRole is the pipeline contract: this process is the
// CAS membrane endpoint only if argv is `object daemon`.
// Note: IS_DAEMON alone does NOT grant privileged writer status, as other daemons
// (ambient, scheduler, steward) also set IS_DAEMON=1. Only the privileged writer
// daemon process itself has this role (F-SEC-002).
func PrivilegedWriterDaemonRole() bool {
	return PrivilegedWriterDaemonArgv(CommandArgs())
}

// PrivilegedWriterRequiredLaunchEnv is the env LaunchAgent / host install
// must inject so production does not depend on Go RunE Setenv order.
func PrivilegedWriterRequiredLaunchEnv() map[string]string {
	return map[string]string{
		IsDaemon().Name(): enabledFlagValue,
	}
}
