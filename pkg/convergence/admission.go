package convergence

import "github.com/lanceman/zqk/pkg/objects"

// DraftToActivePreconditions is the fail-closed membrane contract for
// convergence_session draft→active. Keep this set small: do not require
// full C1–C6 history or optional fields such as predictions.
//
// YAML SSOT: docs/process/_internal/lifecycles/convergence_session_lifecycle.yaml
// Builder: pkg/specbuilder/bldr_lifecycle_v1/convergence_session_builder.go
// TRACK: REDACTED — remove this comment when admission is the default operator expectation.
func DraftToActivePreconditions() []string {
	return []string{
		objects.FieldKeyHypothesis + " is set",
		objects.FieldKeyDesiredEndState + " is set",
		objects.FieldKeyCurrentPhase + " is set",
	}
}
