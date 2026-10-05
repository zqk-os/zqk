package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// listJobs lists all scheduler jobs
func listJobs(ctx *cli.Context, cmd *cobra.Command) error {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue && ctx != nil {
		projectRoot = ctx.ProjectRoot
	}
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	// Use global storage cache so we don't create a new storage instance (avoids blocking init and extra load)
	storageCtx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 30*time.Second)
	defer cancel()

	storageProvider, err := storagepkg.GetGlobalStorageProviderCache().GetOrCreate(storageCtx, projectRoot)
	if err != nil {
		return errfmt.Newf("failed to get storage").Wrap(err)
	}

	// Create security and storage contexts
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtxObj := ctx.GetStorageContext()

	// List all scheduler_job objects
	result, err := storageProvider.List(storageCtx, secCtx, storageCtxObj, storagepkg.ListFilter{
		Kind: schedulerKindJob,
	})
	if err != nil {
		return errfmt.Newf("failed to list scheduler jobs").Wrap(err)
	}

	// Load spec to get display_length constraints
	specLoader := objects.GetGlobalSpecLoader()
	spec, err := specLoader.LoadSpecWithInheritance(schedulerFileSchedulerJobYAML)
	if err != nil {
		// If spec loading fails, use defaults
		spec = nil
	}

	// Format output (--format on command chain)
	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, result)
	default:
		return outputTable(result, spec, cmd)
	}
}

// outputTable outputs scheduler jobs as a table
func outputTable(result *storagepkg.QueryResult, spec *objects.Spec, cmd *cobra.Command) error {
	if result == nil || len(result.Objects) == 0 {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(logger).Info("No scheduler jobs found").Log()
		return nil
	}

	var buf strings.Builder

	// Get column widths using GetColumnWidth (respects --columns flag, spec display_length, or defaults)
	colID := clipkg.GetColumnWidth("id", cmd, spec, 12)
	colJobType := clipkg.GetColumnWidth("job_type", cmd, spec, 28)
	colTrigger := clipkg.GetColumnWidth("trigger_type", cmd, spec, 12)
	colStatus := clipkg.GetColumnWidth("status", cmd, spec, 10)
	colEnabled := clipkg.GetColumnWidth("enabled", cmd, spec, 8) // boolean, fixed width
	colSchedule := clipkg.GetColumnWidth("schedule_expression", cmd, spec, 20)

	totalWidth := colID + colJobType + colTrigger + colStatus + colEnabled + colSchedule + 10 // +10 for spacing

	// Write header
	fmt.Fprintf(&buf, "%-*s %-*s %-*s %-*s %-*s %-*s\n",
		colID, "ID",
		colJobType, "Job Type",
		colTrigger, "Trigger",
		colStatus, "Status",
		colEnabled, "Enabled",
		colSchedule, "Schedule")
	buf.WriteString(strings.Repeat("-", totalWidth) + "\n")

	// Write each job
	for _, obj := range result.Objects {
		id := clipkg.TruncateString(getString(obj, "id"), colID)
		jobType := clipkg.TruncateString(getString(obj, "job_type"), colJobType)
		triggerType := clipkg.TruncateString(getString(obj, "trigger_type"), colTrigger)
		status := clipkg.TruncateString(getString(obj, "status"), colStatus)
		enabled := getBool(obj, "enabled")
		schedule := clipkg.TruncateString(getString(obj, "schedule_expression"), colSchedule)

		enabledStr := "No"
		if enabled {
			enabledStr = "Yes"
		}

		fmt.Fprintf(&buf, "%-*s %-*s %-*s %-*s %-*s %-*s\n",
			colID, id,
			colJobType, jobType,
			colTrigger, triggerType,
			colStatus, status,
			colEnabled, enabledStr,
			colSchedule, schedule)
	}

	fmt.Fprintf(&buf, "\nTotal: %d scheduler job(s)\n", len(result.Objects))
	return cli.WriteOutput(cmd, []byte(buf.String()))
}

// getString safely extracts a string value from a map
func getString(m map[string]any, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
		return fmt.Sprintf("%v", val)
	}
	return ""
}

// getBool safely extracts a boolean value from a map
//
//nolint:unparam // key parameter is kept for API consistency, even though it's always "enabled" in current usage
func getBool(m map[string]any, key string) bool {
	if val, ok := m[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}
