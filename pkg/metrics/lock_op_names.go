// Lock operation names for RunInLockWithLogger / RunInRLockWithLogger.
// CONSTANTS_AND_DRY_INVENTORY_PLAN Phase A.
package metrics

const (
	LockNameSamplerBatchAddEvent            = "sampler_batch_add_event"
	LockNameSamplerBatchGetEventCount       = "sampler_batch_get_event_count"
	LockNameSamplerFlushAllCopyKeys         = "sampler_flush_all_copy_keys"
	LockNameSamplerFlushBatchCopy           = "sampler_flush_batch_copy"
	LockNameSamplerFlushBatchGet            = "sampler_flush_batch_get"
	LockNameSamplerGetBatchCount            = "sampler_get_batch_count"
	LockNameSamplerGetOrCreateBatch         = "sampler_get_or_create_batch"
	LockNameSamplerGetTotalEventsCopy       = "sampler_get_total_events_copy"
	LockNameSamplerRegistryFallbackCheck    = "sampler_registry_fallback_check"
	LockNameSamplerRegistryFallbackStore    = "sampler_registry_fallback_store"
	LockNameSamplerRegistryFlushAllCopy     = "sampler_registry_flush_all_copy"
	LockNameSamplerRegistryGet              = "sampler_registry_get"
	LockNameSamplerRegistryGetOrCreateCheck = "sampler_registry_get_or_create_check"
	LockNameSamplerRegistryGetOrCreateStore = "sampler_registry_get_or_create_store"
	LockNameSamplerRegistryGetStats         = "sampler_registry_get_stats"
	LockNameSamplerRegistryRegisterCheck    = "sampler_registry_register_check"
	LockNameSamplerRegistryRegisterStore    = "sampler_registry_register_store"
	LockNameSamplerRegistryStopAllCopy      = "sampler_registry_stop_all_copy"
	LockNameSamplerRegistryUnregisterCopy   = "sampler_registry_unregister_copy"
)
