// Lock operation names for RunInLockWithLogger / RunInRLockWithLogger.
// CONSTANTS_AND_DRY_INVENTORY_PLAN Phase A.
package featureflags

const (
	LockNameFeatureFlagsGetAll     = "feature_flags_get_all"
	LockNameFeatureFlagsGetFlag    = "feature_flags_get_flag"
	LockNameFeatureFlagsIsEnabled  = "feature_flags_is_enabled"
	LockNameFeatureFlagsLoad       = "feature_flags_load"
	LockNameFeatureFlagsSave       = "feature_flags_save"
	LockNameFeatureFlagsSetEnabled = "feature_flags_set_enabled"
)
