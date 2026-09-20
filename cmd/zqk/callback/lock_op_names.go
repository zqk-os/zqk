// Lock operation names for WithLockTimeout / WithRLockTimeout.
// CONSTANTS_AND_DRY_INVENTORY_PLAN Phase A.
package callback

const (
	LockNameCallbackProcessorEnqueue       = "callback_processor_enqueue"
	LockNameCallbackProcessorGetQueueSize  = "callback_processor_get_queue_size"
	LockNameCallbackProcessorInitialize    = "callback_processor_initialize"
	LockNameCallbackProcessorShutdown      = "callback_processor_shutdown"
	LockNameCallbackProcessorProcessDirect = "callback_processor_process_direct"
	LockNameCallbackQueueDequeue           = "callback_queue_dequeue"
	LockNameCallbackQueueEnqueue           = "callback_queue_enqueue"
	LockNameCallbackQueueRecreateStopch    = "callback_queue_recreate_stopch"
	LockNameCallbackQueueSize              = "callback_queue_size"
	LockNameCallbackQueueStartProcessing   = "callback_queue_start_processing"
	LockNameCallbackQueueStopProcessing    = "callback_queue_stop_processing"
)
