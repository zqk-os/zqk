package transceiver

// Log events for scheduler async router / transceiver (POLICY-CODE-007).
// Wire prefix distinguishes from parent scheduler runtime keys.
const schedulerTransceiverWirePrefix = "scheduler_transceiver"

const (
	LogEventSchedulerTransceiverVerificationRegistered       = schedulerTransceiverWirePrefix + "_verification_registered"
	LogEventSchedulerTransceiverVerificationDeliveryVerified = schedulerTransceiverWirePrefix + "_verification_delivery_verified"
	LogEventSchedulerTransceiverVerificationDeliveryFailed   = schedulerTransceiverWirePrefix + "_verification_delivery_failed"
	LogEventSchedulerTransceiverVerificationComplete         = schedulerTransceiverWirePrefix + "_verification_complete"
	LogEventSchedulerTransceiverVerificationExpired          = schedulerTransceiverWirePrefix + "_verification_expired"

	LogEventSchedulerTransceiverRuleConvertConfigFailed = schedulerTransceiverWirePrefix + "_rule_convert_config_failed"
	LogEventSchedulerTransceiverRuleLoaderNoDir         = schedulerTransceiverWirePrefix + "_rule_loader_no_directory_configured"
	LogEventSchedulerTransceiverRuleLoaderDirMissing    = schedulerTransceiverWirePrefix + "_rule_loader_directory_missing"
	LogEventSchedulerTransceiverRuleLoadFileFailed      = schedulerTransceiverWirePrefix + "_rule_load_file_failed"
	LogEventSchedulerTransceiverRuleLoadedFromFile      = schedulerTransceiverWirePrefix + "_rule_loaded_from_file"

	LogEventSchedulerTransceiverRouterRegisteredAdapter = schedulerTransceiverWirePrefix + "_router_registered_adapter"
	LogEventSchedulerTransceiverRouterLoadedRules       = schedulerTransceiverWirePrefix + "_router_loaded_rules"
	LogEventSchedulerTransceiverRouterRuleMatched       = schedulerTransceiverWirePrefix + "_router_rule_matched"
	LogEventSchedulerTransceiverRouterNoRulesMatched    = schedulerTransceiverWirePrefix + "_router_no_rules_matched"
	LogEventSchedulerTransceiverRouterActionFailed      = schedulerTransceiverWirePrefix + "_router_action_failed"
	LogEventSchedulerTransceiverRouterActionSucceeded   = schedulerTransceiverWirePrefix + "_router_action_succeeded"
	LogEventSchedulerTransceiverRouterUnknownOperator   = schedulerTransceiverWirePrefix + "_router_unknown_condition_operator"
	LogEventSchedulerTransceiverRouterRetryingAction    = schedulerTransceiverWirePrefix + "_router_retrying_action"

	LogEventSchedulerTransceiverProfileLoaded             = schedulerTransceiverWirePrefix + "_profile_loaded"
	LogEventSchedulerTransceiverProfileCreatedAsyncRouter = schedulerTransceiverWirePrefix + "_profile_created_async_router"
	LogEventSchedulerTransceiverProfileApplied            = schedulerTransceiverWirePrefix + "_profile_applied"

	LogEventSchedulerTransceiverAsyncRouterStarting           = schedulerTransceiverWirePrefix + "_async_router_starting"
	LogEventSchedulerTransceiverAsyncRouterStopping           = schedulerTransceiverWirePrefix + "_async_router_stopping"
	LogEventSchedulerTransceiverAsyncRouterAllWorkersStopped  = schedulerTransceiverWirePrefix + "_async_router_all_workers_stopped"
	LogEventSchedulerTransceiverAsyncRouterWorkerStopTimeout  = schedulerTransceiverWirePrefix + "_async_router_worker_stop_timeout"
	LogEventSchedulerTransceiverAsyncRouterStopped            = schedulerTransceiverWirePrefix + "_async_router_stopped"
	LogEventSchedulerTransceiverAsyncRouterMessageQueued      = schedulerTransceiverWirePrefix + "_async_router_message_queued"
	LogEventSchedulerTransceiverAsyncRouterQueueFailed        = schedulerTransceiverWirePrefix + "_async_router_queue_failed"
	LogEventSchedulerTransceiverAsyncRouterRoutingFailed      = schedulerTransceiverWirePrefix + "_async_router_routing_failed"
	LogEventSchedulerTransceiverAsyncRouterRoutingCompleted   = schedulerTransceiverWirePrefix + "_async_router_routing_completed"
	LogEventSchedulerTransceiverAsyncRouterWorkerIdleShutdown = schedulerTransceiverWirePrefix + "_async_router_worker_idle_shutdown"
)
