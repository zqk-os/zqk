package scheduler

const (
	LogEventLifecycleCheckStarted   = JobTypeLifecycleCheck + "_started"
	LogEventLifecycleCheckCompleted = JobTypeLifecycleCheck + "_completed"
)

// noopHandlerWirePrefix is not a schedulable job kind; POLICY-CODE-007 stable keys for [NoOpHandler].
const noopHandlerWirePrefix = "noop_handler"

const (
	LogEventNoOpHandlerExecuted = noopHandlerWirePrefix + "_executed"
)
