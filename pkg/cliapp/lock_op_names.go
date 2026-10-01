// Lock operation names for WithLockTimeout / WithRLockTimeout.
// CONSTANTS_AND_DRY_INVENTORY_PLAN Phase A.
package cli

const (
	LockNameFormatHandlerGet                    = "format_handler_get"
	LockNameFormatHandlerPermissionsSet         = "format_handler_permissions_set"
	LockNameFormatHandlerPermissionsStream      = "format_handler_permissions_stream"
	LockNameFormatHandlerRegister               = "format_handler_register"
	LockNameMcpPermissionCheckerCheckDataAccess = "mcp_permission_checker_check_data_access"
	LockNameMcpPermissionCheckerCheckFormat     = "mcp_permission_checker_check_format"
	LockNameMcpPermissionCheckerCheckPermission = "mcp_permission_checker_check_permission"
	LockNameMcpPermissionCheckerGetSecCtx       = "mcp_permission_checker_get_sec_ctx"
	LockNameMcpPermissionCheckerSetSecCtx       = "mcp_permission_checker_set_sec_ctx"
)
