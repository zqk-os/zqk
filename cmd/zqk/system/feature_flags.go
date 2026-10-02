package system

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/featureflags"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/when"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// runFeatureFlags manages feature flags
func runFeatureFlags(cmd *cobra.Command, ctx *cli.Context, args []string) error {
	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)

	// Get feature flags
	flags := featureflags.GetGlobalFeatureFlags(projectRoot)

	// Check subcommand
	if len(args) == 0 {
		// List all flags
		return listFeatureFlags(cmd, flags)
	}

	subcommand := args[0]
	switch subcommand {
	case "list":
		return listFeatureFlags(cmd, flags)
	case "get":
		if len(args) < 2 {
			return errfmt.Errorf("usage: feature-flags get <flag-name>")
		}
		return getFeatureFlag(cmd, flags, args[1])
	case "enable":
		if len(args) < 2 {
			return errfmt.Errorf("usage: feature-flags enable <flag-name>")
		}
		return setFeatureFlag(cmd, flags, args[1], true)
	case "disable":
		if len(args) < 2 {
			return errfmt.Errorf("usage: feature-flags disable <flag-name>")
		}
		return setFeatureFlag(cmd, flags, args[1], false)
	default:
		return errfmt.Errorf("unknown subcommand: %s", subcommand)
	}
}

// listFeatureFlags lists all feature flags (POL-CODE-007: use command streams)
func listFeatureFlags(cmd *cobra.Command, flags *featureflags.FeatureFlags) error {
	allFlags := flags.GetAllFlags()
	var b strings.Builder
	fmt.Fprintf(&b, "Feature Flags:\n\n")
	for name, flag := range allFlags {
		status := "disabled"
		if flag.Enabled {
			status = "enabled"
		}
		fmt.Fprintf(&b, "  %s: %s", name, status)
		if !when.IsEmpty(flag.Description) {
			fmt.Fprintf(&b, " - %s", flag.Description)
		}
		fmt.Fprintf(&b, "\n")
	}
	return cli.WriteOutput(cmd, []byte(b.String()))
}

// getFeatureFlag gets a specific feature flag (POL-CODE-007: use command streams)
func getFeatureFlag(cmd *cobra.Command, flags *featureflags.FeatureFlags, name string) error {
	flag, exists := flags.GetFlag(name)
	if !exists {
		return errfmt.Errorf("feature flag not found: %s", name)
	}
	status := "disabled"
	if flag.Enabled {
		status = "enabled"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Feature Flag: %s\n", name)
	fmt.Fprintf(&b, "  Status: %s\n", status)
	if !when.IsEmpty(flag.Description) {
		fmt.Fprintf(&b, "  Description: %s\n", flag.Description)
	}
	return cli.WriteOutput(cmd, []byte(b.String()))
}

// setFeatureFlag sets a feature flag (requires elevated privileges)
func setFeatureFlag(cmd *cobra.Command, flags *featureflags.FeatureFlags, name string, enabled bool) error {
	// Get security context for current user (lightweight check - no I/O)
	secCtx := getSecurityContextForFeatureFlags()

	// Check if user has required permissions
	hasSystemWrite := hasPermission(secCtx, "write:system")
	hasConfigWrite := hasPermission(secCtx, "write:config")

	if !hasSystemWrite && !hasConfigWrite {
		return errfmt.Errorf("insufficient permissions: feature flag changes require write:system or write:config permission. "+paths.RewriteCanonicalCLIInvocations("Current account: %s. Use 'zqk system whoami' to check your permissions"), secCtx.AccountID)
	}

	// Get current value for audit (fast - in-memory)
	currentFlag, _ := flags.GetFlag(name)
	previousValue := false
	if currentFlag != nil {
		previousValue = currentFlag.Enabled
	}

	// Set the flag (fast operation - just writes JSON file)
	action := "disabled"
	if enabled {
		action = "enabled"
	}

	if err := flags.SetEnabled(name, enabled); err != nil {
		return errfmt.Newf("failed to set feature flag").Wrap(err)
	}

	// Log the change (non-blocking, lightweight)
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	logging.Fluent(logger).Info("Feature flag change").
		String("flag", name).
		String("action", action).
		Bool("previous", previousValue).
		String("account", secCtx.AccountID).
		Log()

	// Create audit event asynchronously (non-blocking, doesn't delay response)
	// Use goroutine with proper error handling to prevent hangs
	projectRoot := ProjectRootOrResolve("")
	bud := goroutinelabels.DefaultBudget()
	auditBuilder := goroutinelabels.NewGoroutine("feature_flag_audit_creator", fmt.Sprintf("creating audit event for feature flag %s", name))
	if bud != nil {
		auditBuilder = auditBuilder.WithBudget(bud)
	}
	auditBuilder.StartWithContext(pkgctx.NewSystemContext(), func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		if err := createFeatureFlagAuditEventAsync(ctx, projectRoot, secCtx, name, previousValue, enabled); err != nil {
			// Log error but don't fail the flag change
			logging.Fluent(logger).Warn("Failed to create audit event for feature flag change").
				WithError(err).
				Log()
		}
		return nil
	})

	return cli.WriteOutput(cmd, []byte(fmt.Sprintf("Feature flag '%s' %s\n", name, action)))
}

// getSecurityContextForFeatureFlags gets security context for feature flag operations
// Lightweight - no I/O, just checks environment variables
func getSecurityContextForFeatureFlags() *pkgctx.SecurityContext {
	// Check for MCP account ID from environment (if running via MCP)
	if accountID := zqkenv.MCPAccountID().Get(); !when.IsEmpty(accountID) {
		// For MCP, check permissions from environment
		// If permissions include write:system or write:config, allow
		if perms := zqkenv.MCPPermissions().Get(); !when.IsEmpty(perms) {
			// Parse permissions (comma-separated)
			permissionList := strings.Split(perms, ",")
			for perm := range strings.SplitSeq(perms, ",") {
				perm = strings.TrimSpace(perm)
				if perm == "write:system" || perm == "write:config" || perm == "write:*" || perm == "*" {
					// Has required permission
					return &pkgctx.SecurityContext{
						AccountID:   accountID,
						Roles:       []string{"admin"}, // Assume admin if has write permissions
						Permissions: permissionList,
					}
				}
			}
		}
		// MCP account but no write permissions - deny
		return &pkgctx.SecurityContext{
			AccountID:   accountID,
			Roles:       []string{"viewer"},
			Permissions: []string{"read:*"},
		}
	}

	// For CLI operations, use system context (has all permissions)
	// This is safe because CLI runs locally and requires local file system access
	return pkgctx.NewSystemSecurityContext()
}

// createFeatureFlagAuditEventAsync creates an audit event asynchronously (non-blocking)
// Uses context with timeout to prevent hanging
func createFeatureFlagAuditEventAsync(
	ctx context.Context,
	projectRoot string,
	secCtx *pkgctx.SecurityContext,
	flagName string,
	previousValue, newValue bool,
) error {
	// Create storage factory with timeout context
	storageFactory, err := storage.NewStorageFactory(ctx, projectRoot)
	if err != nil {
		return errfmt.Newf("failed to create storage factory").Wrap(err)
	}
	storageProvider := storageFactory.GetStorage()

	now := time.Now().UTC()
	// Create audit event object
	auditEvent := map[string]any{
		objects.FieldKeyID:            fmt.Sprintf("AUDIT-FLAG-%d", now.UnixNano()),
		objects.FieldKeyKind:          objects.KindAuditEvent,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusCompleted,
		objects.FieldKeyEventType:     "feature_flag_change",
		objects.FieldKeyOperation:     "update",
		objects.FieldKeyObjectKind:    "feature_flag",
		"object_id":                   flagName,
		objects.FieldKeyAccountID:     secCtx.AccountID,
		"timestamp":                   now.Format(time.RFC3339),
		objects.FieldKeyMetadata: map[string]any{
			"previous_value":         previousValue,
			objects.FieldKeyNewValue: newValue,
			"flag_name":              flagName,
		},
		objects.FieldKeyCreatedAt: now.Format(time.RFC3339),
		objects.FieldKeyCreatedBy: secCtx.AccountID,
		objects.FieldKeyUpdatedAt: now.Format(time.RFC3339),
		objects.FieldKeyUpdatedBy: secCtx.AccountID,
	}

	// Create audit event (best effort - don't fail flag change if audit fails)
	if err := storageProvider.Create(ctx, secCtx, auditEvent); err != nil {
		return errfmt.Newf("failed to create audit event").Wrap(err)
	}

	return nil
}

// hasPermission checks if security context has a specific permission
func hasPermission(secCtx *pkgctx.SecurityContext, permission string) bool {
	// Check wildcard permissions
	for _, perm := range secCtx.Permissions {
		if perm == "write:*" || perm == "*" {
			return true
		}
		if perm == permission {
			return true
		}
	}
	return false
}

// NewFeatureFlagsCmd creates a command to manage feature flags
func NewFeatureFlagsCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Manage feature flags",
		"Manage feature flags that control system behavior.",
		"",
		"Feature flags allow you to toggle experimental features on/off:",
		"  - async_validation: Use async validator for system check (experimental)",
		"  - async_validation_parallel: Enable parallel validation in async mode",
		"  - deferred_hash_updates: Defer hash updates until all operations complete",
		"  - storage_orchestration: Use storage orchestrator for multi-backend coordination",
	).
		AddExample("List all feature flags", "%s system feature-flags list").
		AddExample("Get a specific flag", "%s system feature-flags get async_validation").
		AddExample("Enable async validation", "%s system feature-flags enable async_validation").
		AddExample("Disable async validation", "%s system feature-flags disable async_validation").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemFeatureFlagsCommandBuilder(), &cobra.Command{
		Use:  "feature-flags [command] [flag-name]",
		Args: cobra.MinimumNArgs(0),
	})
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		initCtx := &pkgctx.CliInitializationContext{
			ProjectRoot: ProjectRootOrResolve(""),
		}
		ctx, err := cli.GetContextFromCommand(cmd, initCtx)
		if err != nil {
			return err
		}
		return runFeatureFlags(cmd, ctx, args)
	})

	return cli.FinalizeCommand(cmd, helpBuilder)
}
