package scheduler

// CAS recovery shared helper logs ([runCASRecoveryForKind]); not a schedulable JobType (same spirit as [noopHandlerWirePrefix]).
const casRecoveryKindWirePrefix = "cas_recovery_kind"

const (
	LogEventCASRecoveryKindFailed    = casRecoveryKindWirePrefix + "_failed"
	LogEventCASRecoveryKindCompleted = casRecoveryKindWirePrefix + "_completed"
)
