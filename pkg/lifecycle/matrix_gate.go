package lifecycle

import "github.com/lanceman/zqk/pkg/objects"

// MatrixStatusMeetsTransitionGate returns true if the verification_matrix status
// satisfies the objective accountability transition gate requirements.
func MatrixStatusMeetsTransitionGate(status string) bool {
	switch status {
	case "active", "archived", objects.ObjectStatusCompleted:
		return true
	default:
		return false
	}
}
