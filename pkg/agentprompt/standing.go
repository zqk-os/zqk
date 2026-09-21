package agentprompt

// Standing kernel policies that every task is bound by. Persist IDs, not bodies.
const (
	StandingPolicyProcessData       = "POL-ONBOARD-001"
	StandingPolicyTDD               = "POL-ONBOARD-002"
	StandingPolicyInstructionRubric = "POL-AGENT-INSTRUCTION-RUBRIC-001"
	StandingPolicyVDS               = "POL-WORKFLOW-VDS"
	StandingPolicyCASCommit         = "POL-CODE-CAS-COMMIT-001"
	StandingPolicyFlywheel          = "POL-CODE-FLYWHEEL-001"
)

// StandingPolicyRefs returns the always-on policy IDs that must be linked, not copied.
func StandingPolicyRefs() []string {
	return []string{
		StandingPolicyProcessData,
		StandingPolicyTDD,
		StandingPolicyInstructionRubric,
		StandingPolicyVDS,
		StandingPolicyCASCommit,
		StandingPolicyFlywheel,
	}
}
