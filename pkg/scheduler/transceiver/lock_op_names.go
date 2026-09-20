package transceiver

// Lock operation names for RunInLockWithLogger / RunInRLockWithLogger (CONSTANTS_AND_DRY_INVENTORY_PLAN Phase A).

// AsyncRouter lock operations.
const (
	LockNameAsyncRouterSetCallback             = "async_router_set_callback"
	LockNameAsyncRouterGetCallback             = "async_router_get_callback"
	LockNameAsyncRouterSetProjectRoot          = "async_router_set_project_root"
	LockNameAsyncRouterSetStorageProvider      = "async_router_set_storage_provider"
	LockNameAsyncRouterStart                   = "async_router_start"
	LockNameAsyncRouterStop                    = "async_router_stop"
	LockNameAsyncRouterRouteCheck              = "async_router_route_check"
	LockNameAsyncRouterWakeWorker              = "async_router_wake_worker"
	LockNameAsyncRouterWorkerStartEvent        = "async_router_worker_start_event"
	LockNameAsyncRouterWorkerShutdownEvent     = "async_router_worker_shutdown_event"
	LockNameAsyncRouterWorkerIdleShutdownEvent = "async_router_worker_idle_shutdown_event"
	LockNameAsyncRouterInitiateShutdown        = "async_router_initiate_shutdown"
)

// Router lock operations (router.go).
const (
	LockNameRouterRegisterAdapter   = "router_register_adapter"
	LockNameRouterUnregisterAdapter = "router_unregister_adapter"
	LockNameRouterLoadRules         = "router_load_rules"
	LockNameRouterGetRules          = "router_get_rules"
	LockNameRouterRouteGetRules     = "router_route_get_rules"
	LockNameRouterGetAdapter        = "router_get_adapter"
)

// Rule validation (rule_validation.go).
const (
	LockNameRouterValidateAdapter = "router_validate_adapter"
)

// Message verification lock operations (verification.go).
const (
	LockNameMessageVerificationRegister  = "message_verification_register"
	LockNameMessageVerificationVerify    = "message_verification_verify"
	LockNameMessageVerificationGetResult = "message_verification_get_result"
	LockNameMessageVerificationCleanup   = "message_verification_cleanup"
)

// Router metrics lock operations (router_metrics.go).
const (
	LockNameRouterMetricsRecordAdapter      = "router_metrics_record_adapter"
	LockNameRouterMetricsRecordRoutingTime  = "router_metrics_record_routing_time"
	LockNameRouterMetricsGetSnapshotMain    = "router_metrics_get_snapshot_main"
	LockNameRouterMetricsGetSnapshotAdapter = "router_metrics_get_snapshot_adapter"
)
