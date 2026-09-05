package objects

import "strings"

// TaskStepIsClosed reports whether an agent_task task_steps[].status value is
// done for the terminal security gate. Step statuses are NOT agent_task lifecycle
// values — do not call IsTerminal("agent_task", stepStatus) (that wrongly rejected
// step "completed" after agent_task dropped the completed stage).
func TaskStepIsClosed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case ObjectStatusCompleted, ObjectStatusComplete, ObjectStatusImplemented, "skipped", "done", "closed":
		return true
	default:
		return false
	}
}
