package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

// Annotation keys for command orchestration. Commands set these to declare what
// they need; root PreRunE/PostRunE use them so we don't start storage, session,
// or coordinator paths unnecessarily (avoids blocking/hangs and keeps orchestration explicit).
const (
	// AnnRequiresStorage: "true" (default for object/internal subtree) | "false".
	// When false, root does not pre-warm storage or create storage for session.
	AnnRequiresStorage = "zqk.requires_storage"
	// AnnRequiresSession: "true" (default) | "false".
	// When false, root does not start/reuse zqk_session (no GetObjectStorageForCommand for session).
	AnnRequiresSession = "zqk.requires_session"
	// AnnRequiresSchedulerCheck: "true" (default) | "false".
	// When false, root skips scheduler daemon status check (e.g. scheduler start/stop/status).
	AnnRequiresSchedulerCheck = "zqk.requires_scheduler_check"
	// AnnRequiresGovernorApproval: "true" | "false" (default).
	// When true, the command requires an APPROVED event edge in the graph before execution.
	AnnRequiresGovernorApproval = "zqk.requires_governor_approval"
)

// CommandRequirements holds the resolved orchestration requirements for the command
// that will run. Root PreRunE uses this to decide what to initialize (storage, session, etc.).
type CommandRequirements struct {
	// RequiresStorage: pre-warm storage and set in context (object/internal CRUD/list). If false, do not
	// start storage init or create storage for session.
	RequiresStorage bool
	// RequiresSession: start/reuse zqk_session. If false, do not call GetObjectStorageForCommand
	// for session (avoids creating storage and CAS index queue for commands that don't need it).
	RequiresSession bool
	// RequiresSchedulerCheck: check scheduler daemon status. If false, skip (e.g. scheduler status).
	RequiresSchedulerCheck bool
	// RequiresGovernorApproval: check for human signature on high-stakes actions.
	RequiresGovernorApproval bool
}

// GetCommandRequirements resolves the target command from root and args, then returns
// its orchestration requirements (from annotations with sensible defaults).
// Use this in root PersistentPreRunE so orchestration is driven by declarative command metadata.
func GetCommandRequirements(root *cobra.Command, args []string) CommandRequirements {
	req := CommandRequirements{
		RequiresStorage:          false, // only object/internal commands get pre-warm by default
		RequiresSession:          true,
		RequiresSchedulerCheck:   true,
		RequiresGovernorApproval: false,
	}
	targetCmd, remaining, err := root.Find(args)
	if err != nil || targetCmd == nil {
		return req
	}
	// Default storage = true when the resolved command is under "object" or "internal" (object-like CRUD/list).
	if isUnderCommand(targetCmd, "object") || isUnderCommand(targetCmd, "internal") {
		req.RequiresStorage = true
	}
	applyAnnotation(targetCmd, AnnRequiresStorage, &req.RequiresStorage)
	applyAnnotation(targetCmd, AnnRequiresSession, &req.RequiresSession)
	applyAnnotation(targetCmd, AnnRequiresSchedulerCheck, &req.RequiresSchedulerCheck)
	applyAnnotation(targetCmd, AnnRequiresGovernorApproval, &req.RequiresGovernorApproval)
	_ = remaining
	return req
}

func applyAnnotation(cmd *cobra.Command, key string, out *bool) {
	v := getAnnotation(cmd, key)
	if v == emptyValue {
		return
	}
	switch strings.ToLower(v) {
	case "true", "1", "yes":
		*out = true
	case "false", "0", "no":
		*out = false
	}
}

// getAnnotation returns the value for key from cmd or any parent (first wins from leaf to root).
func getAnnotation(cmd *cobra.Command, key string) string {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Annotations != nil {
			if v, ok := c.Annotations[key]; ok && v != emptyValue {
				return v
			}
		}
	}
	return ""
}

// isUnderCommand returns true if cmd is the given name or has it as an ancestor.
func isUnderCommand(cmd *cobra.Command, name string) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == name {
			return true
		}
	}
	return false
}

// RequireSession sets the command's session requirement. Call when building a command that
// must not trigger storage/session (e.g. system generate-builders). Idempotent.
func RequireSession(cmd *cobra.Command, require bool) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	if require {
		cmd.Annotations[AnnRequiresSession] = "true"
	} else {
		cmd.Annotations[AnnRequiresSession] = "false"
	}
}

// RequireStorage sets the command's storage requirement. Call when building a command that
// must not pre-warm or create storage. Idempotent.
func RequireStorage(cmd *cobra.Command, require bool) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	if require {
		cmd.Annotations[AnnRequiresStorage] = "true"
	} else {
		cmd.Annotations[AnnRequiresStorage] = "false"
	}
}

// RequireSchedulerCheck sets whether root should run the scheduler daemon status check.
func RequireSchedulerCheck(cmd *cobra.Command, require bool) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	if require {
		cmd.Annotations[AnnRequiresSchedulerCheck] = "true"
	} else {
		cmd.Annotations[AnnRequiresSchedulerCheck] = "false"
	}
}

// RequireGovernorApproval sets whether the command requires a human signature via the Governor pattern.
func RequireGovernorApproval(cmd *cobra.Command, require bool) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	if require {
		cmd.Annotations[AnnRequiresGovernorApproval] = "true"
	} else {
		cmd.Annotations[AnnRequiresGovernorApproval] = "false"
	}
}
