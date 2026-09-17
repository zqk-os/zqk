package adapters

// Adapter-only log keys (POL-CODE-007). Package adapters cannot import parent transceiver (import cycle).
// Wire strings MUST stay aligned with pkg/scheduler/transceiver/log_events.go naming.
const (
	LogEventSchedulerTransceiverHTTPWebhookSucceeded = "scheduler_transceiver_http_webhook_succeeded"
	LogEventSchedulerTransceiverHTTPUnknownAuth      = "scheduler_transceiver_http_unknown_auth_type"
	LogEventSchedulerTransceiverEventEmittedStub     = "scheduler_transceiver_event_emitted_stub"
	LogEventSchedulerTransceiverCommandExecuted      = "scheduler_transceiver_command_executed"
)
