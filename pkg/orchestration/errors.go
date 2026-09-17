package orchestration

const (
	ErrMsgPolicyNegotiationFailed = "policy negotiation failed"
	ErrMsgPolicyRejectedIntent    = "policy rejected intent: %s"
	ErrMsgMemoryRetrievalFailed   = "memory retrieval failed"
	ErrMsgCommitmentFailed        = "commitment failed"
	ErrMsgSynthesisError          = "synthesis error"
	ErrMsgSynthesisReplayFailed   = "synthesis replay failed"
	ErrMsgWriteTickerLog          = "failed to write ticker log: %v"
	ErrMsgWriteAgentActivity      = "failed to write agent activity: %v"
	ErrMsgWriteNextUpdate         = "failed to write next update: %v"
	LogFmtAutonomousProgress      = "Autonomous progress: PlanID=%s, Impact=%s, Summary=%s"
	LogFmtSynthesized             = "Synthesized capability from %s"
	LogKeyHiveActivity            = "hive_activity.log"
	LogFmtHiveAgentsActive        = "Agents active: %d\n"
	LogFmtNextUpdate              = "Next update: %s\n"
	LogKeyIntentSubmission        = "intent_submission"
	LogKeySynthesisAgent          = "synthesis_agent"
	LogFmtOrchestratingIntent     = "orchestrating intent: "
	OpNameCapabilitySynthesis     = "capability_synthesis"
	LogMsgSynthesizingIntent      = "synthesizing intent: "
)
