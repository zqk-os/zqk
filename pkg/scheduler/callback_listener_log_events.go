package scheduler

// Log events for callback_listener handler (POL-CODE-007). Prefix matches [JobTypeCallbackListener].
const (
	LogEventCallbackListenerStarted                = JobTypeCallbackListener + "_started"
	LogEventCallbackListenerShuttingDownContext    = JobTypeCallbackListener + "_shutting_down_context"
	LogEventCallbackListenerServerError            = JobTypeCallbackListener + "_server_error"
	LogEventCallbackListenerRegisteringRoute       = JobTypeCallbackListener + "_registering_route"
	LogEventCallbackListenerAuthError              = JobTypeCallbackListener + "_auth_error"
	LogEventCallbackListenerUnauthenticatedRequest = JobTypeCallbackListener + "_unauthenticated_request"
	LogEventCallbackListenerAuthenticatedRequest   = JobTypeCallbackListener + "_authenticated_request"
	LogEventCallbackListenerParsePayloadFailed     = JobTypeCallbackListener + "_parse_payload_failed"
	LogEventCallbackListenerReceivedCallback       = JobTypeCallbackListener + "_received_callback"
	LogEventCallbackListenerUnknownHandlerType     = JobTypeCallbackListener + "_unknown_handler_type"
	LogEventCallbackListenerJobCompleteReceived    = JobTypeCallbackListener + "_job_complete_received"
	LogEventCallbackListenerJobErrorReceived       = JobTypeCallbackListener + "_job_error_received"
	LogEventCallbackListenerJobStatusReceived      = JobTypeCallbackListener + "_job_status_received"
	LogEventCallbackListenerTriggerReceived        = JobTypeCallbackListener + "_trigger_received"
	LogEventCallbackListenerTriggerJobFailed       = JobTypeCallbackListener + "_trigger_job_failed"
	LogEventCallbackListenerEmitEventReceived      = JobTypeCallbackListener + "_emit_event_received"
	LogEventCallbackListenerIdleShutdown           = JobTypeCallbackListener + "_idle_shutdown"
	LogEventCallbackListenerShutdownGraceful       = JobTypeCallbackListener + "_shutdown_graceful"
	LogEventCallbackListenerShutdownError          = JobTypeCallbackListener + "_shutdown_error"
)
