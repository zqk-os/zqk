package interactionpolicy

// Event class names for agent↔kernel ping-pong.
// TRACK: REDACTED
const (
	EventGoTest           = "go_test"
	EventGitCommit        = "git_commit"
	EventGitWorktreeAdd   = "git_worktree_add"
	EventAgentOrchestrate = "agent_orchestrate"
	EventAgentExecute     = "agent_execute"
	EventIdle             = "idle"
	EventInboxUnacked     = "inbox_unacked"
	EventPushAhead        = "push_ahead"
	EventShovelReadyEmpty = "shovel_ready_empty"
	EventStratplanAhead   = "stratplan_ahead"
	EventCommsFail        = "comms_fail"
	EventLifecycleBlocked = "lifecycle_blocked"
)

// Known interaction policy IDs (kernel objects). Catalog text is the fast pong;
// CAS policy body remains SSOT for operators.
const (
	PolicyAdminMembrane   = "POL-AGENT-ADMIN-MEMBRANE-001"
	PolicyTPMGroomAhead   = "POL-AGENT-TPM-GROOM-AHEAD-001"
	PolicyTPMProcessAdmin = "POL-AGENT-TPM-PROCESS-ADMIN-001"
	PolicyCommsRemedy     = "POL-AGENT-COMMS-REMEDY-WAKE-001"
	PolicyInteractionMeta = "POL-AGENT-INTERACTION-POLICY-001"
)
