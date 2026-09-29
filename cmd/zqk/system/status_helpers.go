package system

import (
	"github.com/zqk-os/zqk/pkg/execwrap"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"

	stdctx "context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/authcred"
	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/when"

	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	projectStatusIncomplete  = "incomplete"
	projectStatusInitialized = "initialized"
)

// StatusData holds structured status information
type StatusData struct {
	Project             string
	Root                string
	Status              string
	Message             string
	CurrentPriorityPlan map[string]any
	SystemHealth        map[string]any
	RecentActivity      []string
}

// getProjectRoot gets the project root from context or finds it
func getProjectRoot(ctx *cli.Context) (string, error) {
	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)
	if projectRoot == emptyValue {
		return "", errfmt.Errorf("not a ZQK project (no project root found)")
	}
	return projectRoot, nil
}

// getProjectName extracts project name from config or directory
func getProjectName(projectRoot string) string {
	projectName := filepath.Base(projectRoot)
	configPath := paths.FirstProjectYAMLConfig(projectRoot)
	if configData, err := fileutil.ReadFile(configPath); err == nil {
		if name := extractProjectName(string(configData)); name != emptyValue {
			projectName = name
		}
	}
	return projectName
}

// determineProjectStatus determines the project status based on directory structure
func determineProjectStatus(projectRoot string) (status string, message string) {
	processDir, err := resolveProcessDirForProject(projectRoot)
	if err != nil {
		processDir = datacell.ProcessPrimaryDir(projectRoot)
	}
	if _, err := fileutil.Stat(processDir); err != nil {
		relShow := paths.ProcessDir
		if r := paths.GetPathAlias(projectRoot, "process"); r != "" {
			relShow = r
		}
		return projectStatusIncomplete, fmt.Sprintf("Incomplete (%s directory not found)", relShow)
	}
	return projectStatusInitialized, "Initialized"
}

// getCurrentPriorityPlan retrieves the current active priority plan
func getCurrentPriorityPlan(cmd *cobra.Command, projectRoot string) map[string]any {
	resultChan := make(chan map[string]any, 1)
	goroutinelabels.NewGoroutine("refactor", "refactored").StartSimple(func() {
		func() {
			defer close(resultChan)

			factory, err := storage.NewStorageFactory(cmd.Context(), projectRoot)
			if err != nil {
				return
			}
			storageProvider := factory.GetStorage()
			if storageProvider != nil {
				defer func() { _ = storageProvider.Shutdown(stdctx.Background()) }()
			}

			secCtx := pkgctx.NewSystemSecurityContext()
			storageCtx := pkgctx.NewStorageContext()
			plans, err := storageProvider.List(cmd.Context(), secCtx, storageCtx, storage.DefaultQueryFactory.
				IdTitleStatusActive(objects.KindPriorityPlan).
				Sort("active_order", true).
				Limit(1).
				Build())
			if err != nil || len(plans.Objects) == 0 {
				return
			}

			currentPlan := plans.Objects[0]
			planID, okID := currentPlan[objects.FieldKeyID].(string)
			title, okTitle := currentPlan[objects.FieldKeyTitle].(string)
			if !okID || !okTitle {
				return
			}

			out := map[string]any{objects.FieldKeyID: planID, objects.FieldKeyTitle: title}
			// Backlog item count on this plan (non-terminal statuses only)
			blResult, err := storageProvider.List(cmd.Context(), secCtx, storageCtx, storage.DefaultQueryFactory.
				ForPlan(objects.KindBacklogItem, planID).
				StatusNotIn("complete", "archived", "rejected").
				IncludeFields(objects.FieldKeyID).
				Limit(1).
				Build())
			if err == nil && blResult.Meta != nil {
				if n, ok := blResult.Meta["total_count"].(int); ok {
					out["backlog_items"] = n
				}
			}
			resultChan <- out
		}()
	})

	select {
	case plan := <-resultChan:
		return plan
	case <-time.After(3 * time.Second):
		return map[string]any{
			objects.FieldKeyID:    "unknown",
			objects.FieldKeyTitle: "timed out fetching priority plan",
		}
	}
}

// getSystemHealthDataMCPFast returns minimal health for MCP (scheduler only, no system check subprocess).
// Used when --context mcp so system status returns in ~2–3s instead of 10+ seconds.
// resolveStatusCheckBinary picks a project-local CLI for system status's embedded
// fast check. PATH-only "zqk-stable" misses produced false check_failed.
func resolveStatusCheckBinary(projectRoot string) string {
	cands := append([]string{}, paths.StableBinaryCandidates(projectRoot)...)
	cands = append(cands,
		paths.RepoBinPath(projectRoot),
		filepath.Join(projectRoot, paths.CLICommandName),
	)
	for _, p := range cands {
		if p == "" {
			continue
		}
		if st, err := fileutil.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	if p, err := exec.LookPath(paths.CLICommandName); err == nil {
		return p
	}
	if p, err := exec.LookPath(brand.ZqkStableName); err == nil {
		return p
	}
	return paths.CLICommandName
}

func getSystemHealthDataMCPFast(projectRoot string) map[string]any {
	healthData := map[string]any{}
	schedulerRunning, _, err := scheduler.IsSchedulerRunning(projectRoot)
	if err != nil {
		schedulerRunning = false
	}
	healthData["scheduler"] = map[string]any{"running": schedulerRunning}
	healthData["mcp_fast"] = true

	checkSummary := loadCheckSummaryFromSystemHealth(projectRoot)
	if checkSummary != nil {
		healthData["blocking_issues"] = checkSummary.Blocking
		healthData["warnings"] = checkSummary.Warnings
		healthData["total_objects"] = checkSummary.TotalObjects
		healthData["source"] = checkSummary.Source
		switch {
		case checkSummary.Blocking > 0:
			healthData[objects.FieldKeyStatus] = "degraded"
		case checkSummary.Warnings > 0:
			healthData[objects.FieldKeyStatus] = "warning"
		default:
			healthData[objects.FieldKeyStatus] = "healthy"
		}
	} else {
		healthData[objects.FieldKeyStatus] = "unknown"
	}
	return healthData
}

// getSystemHealthData gets system health information by running system check
// Returns health summary including blocking issues, warnings, scheduler status, and overall status
func getSystemHealthData(projectRoot string) map[string]any {
	healthData := map[string]any{}

	if projectRoot == emptyValue || !paths.IsValidProjectRoot(projectRoot) {
		healthData[objects.FieldKeyStatus] = "uninitialized"
		healthData["error"] = paths.RewriteCanonicalCLIInvocations("project root not found or uninitialized; run 'zqk system init'")
		healthData["check_failed"] = true
		healthData["scheduler"] = map[string]any{"running": false}
		return healthData
	}

	// Check scheduler status
	schedulerRunning, _, err := scheduler.IsSchedulerRunning(projectRoot)
	if err != nil {
		// Log but don't fail - scheduler status check is best effort
		// Use system logger for this check
		systemLogger := logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
		systemLogger.Debug("Failed to check scheduler status", concurrency.LockField{Key: "error", Value: err.Error()})
		// Continue with schedulerRunning = false if error occurred
		schedulerRunning = false
	}
	healthData["scheduler"] = map[string]any{
		"running": schedulerRunning,
	}

	// Prefer workshop/repo stable then tip bin — never fail solely because
	// "zqk-stable" is absent from PATH (PATH miss was a false check_failed).
	zqkBin := resolveStatusCheckBinary(projectRoot)

	// Run system check with JSON output for parsing (quick check, no auto-fix)
	ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 10*time.Second)
	defer cancel()

	cmd := execwrap.CommandContext(ctx, zqkBin, "system", "check", "--format", "json", "--fast")
	zqkenv.WireExecForIsolatedProject(cmd, projectRoot)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == stdctx.DeadlineExceeded {
		healthData[objects.FieldKeyStatus] = "degraded"
		healthData["error"] = "system check timed out"
		healthData["check_failed"] = true
		return healthData
	}
	// Parse JSON output to extract health summary (from a PARTIAL --fast check).
	var checkResult struct {
		PartialCheck bool `json:"partial_check"`
		Summary      struct {
			BlockingIssues    int `json:"blocking_issues"`
			Warnings          int `json:"warnings"`
			PublicBlocking    int `json:"public_blocking"`
			InternalBlocking  int `json:"internal_blocking"`
			TotalObjects      int `json:"total_objects"`
			ObjectsWithIssues int `json:"objects_with_issues"`
		} `json:"summary"`
	}

	parsedJSON := false
	outputStr := string(output)
	jsonStart := strings.Index(outputStr, "{")
	if jsonStart >= 0 {
		jsonEnd := strings.LastIndex(outputStr, "}")
		if jsonEnd > jsonStart {
			jsonData := outputStr[jsonStart : jsonEnd+1]
			if err := json.Unmarshal([]byte(jsonData), &checkResult); err == nil {
				parsedJSON = true
				healthStatus := "partial_ok"
				if checkResult.Summary.BlockingIssues > 0 {
					healthStatus = "critical"
				} else if checkResult.Summary.Warnings > 0 {
					healthStatus = "warning"
				}

				healthData[objects.FieldKeyStatus] = healthStatus
				healthData["partial_check"] = true
				healthData["health_note"] = checkFastPartialVerdict
				healthData["blocking_issues"] = checkResult.Summary.BlockingIssues
				healthData["warnings"] = checkResult.Summary.Warnings
				healthData["public_blocking"] = checkResult.Summary.PublicBlocking
				healthData["internal_blocking"] = checkResult.Summary.InternalBlocking
				healthData["total_objects"] = checkResult.Summary.TotalObjects
				healthData["objects_with_issues"] = checkResult.Summary.ObjectsWithIssues
				if vis, msg, action := GetRetentionDriftReminder(stdctx.Background(), projectRoot); vis {
					healthData["retention_drift"] = true
					healthData["retention_drift_message"] = msg
					healthData["retention_drift_action"] = action
				}
				return healthData
			}
		}
	}

	if err != nil && !parsedJSON {
		// If command failed and no JSON could be parsed, return error status
		healthData[objects.FieldKeyStatus] = "error"
		healthData["error"] = err.Error()
		healthData["check_failed"] = true
		return healthData
	}

	// If parsing failed, return unknown status
	healthData[objects.FieldKeyStatus] = "unknown"
	healthData["parse_failed"] = true
	return healthData
}

// getRecentActivity gets recent activity for verbose mode
func getRecentActivity(projectRoot string) []string {
	processDir, err := resolveProcessDirForProject(projectRoot)
	if err != nil {
		processDir = datacell.ProcessPrimaryDir(projectRoot)
	}
	auditDir := filepath.Join(processDir, "audit", time.Now().Format("2006-01"))
	entries, err := fileutil.ReadDir(auditDir)
	if err != nil || len(entries) == 0 {
		return nil
	}

	count := len(entries)
	if count > 5 {
		count = 5
	}
	recent := make([]string, 0, count)
	for i := len(entries) - count; i < len(entries); i++ {
		recent = append(recent, entries[i].Name())
	}
	return recent
}

// buildStatusData builds the complete status data structure
func buildStatusData(cmd *cobra.Command, ctx *cli.Context, verbose bool) (map[string]any, error) {
	projectRoot, err := getProjectRoot(ctx)
	if err != nil {
		return nil, err
	}

	projectName := getProjectName(projectRoot)
	status, message := determineProjectStatus(projectRoot)

	statusData := map[string]any{
		"project":              projectName,
		"root":                 projectRoot,
		objects.FieldKeyStatus: status,
		"message":              message,
	}

	// Get current priority plan
	if plan := getCurrentPriorityPlan(cmd, projectRoot); plan != nil {
		statusData["current_priority_plan"] = plan
	}

	// Add system health information (always include, but more detailed in verbose)
	// Note: This may take a moment as it runs system check.
	// MCP fast path: skip full system check (10s+) when --context mcp so status returns in ~2–3s.
	profile := ""
	if ctx != nil {
		profile = ctx.Profile
	}
	var healthData map[string]any
	when.When(func() bool { return profile == systemProfileMCP }).Then(func() {
		healthData = getSystemHealthDataMCPFast(projectRoot)
	}).OrElse(func() {
		healthData = getSystemHealthData(projectRoot)
	}).Run()
	if len(healthData) > 0 {
		statusData["system_health"] = healthData
	}

	if ident := identityStatusForStatus(cmd, projectRoot); ident != nil {
		statusData["identity"] = ident
	}

	// Read CAP review result gate status from .zqk/state/cap_review_result.json
	capReviewPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, "cap_review_result.json")
	if raw, err := fileutil.ReadFile(capReviewPath); err == nil {
		var capReview map[string]any
		if json.Unmarshal(raw, &capReview) == nil {
			if info, stErr := fileutil.Stat(capReviewPath); stErr == nil {
				age := time.Since(info.ModTime())
				capReview["freshness"] = age.Truncate(time.Second).String()
				// 2h: stale "system_check:true" must not read as pristine.
				stale := age > 2*time.Hour
				capReview["stale"] = stale
				if stale {
					capReview["system_check_effective"] = false
					capReview["stale_note"] = "cap_review older than 2h — do not treat system_check as pristine"
				} else if sc, ok := capReview["system_check"].(bool); ok {
					capReview["system_check_effective"] = sc
				}
			}
			statusData["cap_review"] = capReview
		}
	}

	// Add verbose information
	if verbose {
		if recent := getRecentActivity(projectRoot); recent != nil {
			statusData["recent_activity"] = recent
		}
	}

	return statusData, nil
}

func identityStatusForStatus(cmd *cobra.Command, projectRoot string) map[string]any {
	if sec := pkgctx.GetSecurityContext(cmd.Context()); sec != nil && strings.TrimSpace(sec.AccountID) != "" {
		snap := authcred.SnapshotFromSecurityContext(sec, nil)
		authcred.WriteIdentityStatus(projectRoot, snap)
		return identityStatusMap(snap)
	}
	if snap, ok := authcred.ReadIdentityStatus(projectRoot); ok {
		return identityStatusMap(snap)
	}
	return nil
}

func identityStatusMap(snap authcred.IdentityStatus) map[string]any {
	return map[string]any{
		objects.FieldKeyAccountID:  snap.AccountID,
		objects.FieldKeyUsername:   snap.Username,
		objects.FieldKeyTitle:      snap.Title,
		objects.FieldKeyPersonaRef: snap.PersonaID,
		objects.FieldKeyRoles:      snap.Roles,
		"lane":                     snap.Lane,
		"write_star":               snap.WriteStar,
		objects.FieldKeyUpdatedAt:  snap.UpdatedAt,
	}
}

// buildStorageInfo builds storage information for verbose mode
func buildStorageInfo(cmd *cobra.Command, ctx *cli.Context) map[string]any {
	projectRoot, err := getProjectRoot(ctx)
	if err != nil {
		return nil
	}

	processDir, resolveErr := resolveProcessDirForProject(projectRoot)
	if resolveErr != nil {
		processDir = datacell.ProcessPrimaryDir(projectRoot)
	}
	factory, err := storage.NewStorageFactory(stdctx.Background(), projectRoot)
	if err != nil {
		return map[string]any{
			"backend":  "file-based",
			"data_dir": processDir,
		}
	}
	storageProvider := factory.GetStorage()
	if storageProvider != nil {
		defer func() { _ = storageProvider.Shutdown(stdctx.Background()) }()
	}

	storageInfo := map[string]any{
		"backend":       "file-based",
		"data_dir":      processDir,
		"object_counts": make(map[string]int),
	}

	// Count objects by kind
	secCtx := pkgctx.NewSystemSecurityContext()
	kinds := []string{
		objects.KindBacklogItem, objects.KindPolicy, objects.KindRequirement, objects.KindCriteria, objects.KindTestCase,
	}
	counts := make(map[string]int)
	for _, kind := range kinds {
		filter := storage.ListFilter{Kind: kind}
		count, err := storageProvider.Count(cmd.Context(), secCtx, filter)
		if err == nil {
			counts[kind] = count
		}
	}
	storageInfo["object_counts"] = counts

	return storageInfo
}

// formatStatusTableHeader formats the status table header
func formatStatusTableHeader() string {
	return "ZQK Project Status\n===================\n\n"
}

// formatStatusTableProject formats project information for table output
func formatStatusTableProject(statusData map[string]any) string {
	var result string
	if project, ok := statusData["project"].(string); ok {
		result += fmt.Sprintf("Project: %s\n", project)
	}
	if root, ok := statusData["root"].(string); ok {
		result += fmt.Sprintf("Root: %s\n", root)
	}
	return result
}

// formatStatusTableStatus formats status information for table output
func formatStatusTableStatus(statusData map[string]any) string {
	status, ok := statusData[objects.FieldKeyStatus].(string)
	if !ok {
		return ""
	}
	if status == projectStatusIncomplete {
		if m, ok := statusData["message"].(string); ok && m != "" {
			return fmt.Sprintf("Status: ⚠️  %s\n", m)
		}
		return fmt.Sprintf("Status: ⚠️  Incomplete (%s directory not found)\n", paths.ProcessDir)
	}

	var statusMsg string
	if health, ok := statusData["system_health"].(map[string]any); ok {
		healthStatus, _ := health[objects.FieldKeyStatus].(string)
		schedRunning := true
		if sched, ok := health["scheduler"].(map[string]any); ok {
			if r, ok := sched["running"].(bool); ok {
				schedRunning = r
			}
		}

		if healthStatus == "critical" || healthStatus == "error" {
			errMsg := "blocking issues detected"
			if b, ok := health["blocking_issues"].(int); ok && b > 0 {
				errMsg = fmt.Sprintf("%d blocking issue(s) detected", b)
			} else if e, ok := health["error"].(string); ok && e != "" {
				errMsg = e
			}
			statusMsg = fmt.Sprintf("❌ Degraded / Critical (%s)", errMsg)
		} else if healthStatus == "degraded" {
			statusMsg = "⚠️  Degraded (health check timed out)"
		} else if !schedRunning {
			statusMsg = "⚠️  Initialized (Scheduler daemon stopped)"
		} else if healthStatus == "warning" {
			wCount := 0
			if w, ok := health["warnings"].(int); ok {
				wCount = w
			}
			if wCount > 0 {
				statusMsg = fmt.Sprintf("⚠️  Initialized (%d warnings)", wCount)
			} else {
				statusMsg = "⚠️  Initialized (warnings detected)"
			}
		} else {
			statusMsg = "✅ Initialized (Healthy)"
		}
	} else {
		statusMsg = "✅ Initialized"
	}

	return fmt.Sprintf("Status: %s\n", statusMsg)
}

// formatStatusTableHealth formats system health information for table output
func formatStatusTableHealth(statusData map[string]any) string {
	health, ok := statusData["system_health"].(map[string]any)
	if !ok || len(health) == 0 {
		return ""
	}
	var buf strings.Builder
	buf.WriteString("\nSystem Health:\n")
	if sched, ok := health["scheduler"].(map[string]any); ok {
		if r, ok := sched["running"].(bool); ok {
			schedStatus := "running"
			if !r {
				schedStatus = fmt.Sprintf("stopped (run '%s scheduler start' to start background daemon)", paths.CLICommandName)
			}
			buf.WriteString(fmt.Sprintf("  Scheduler: %s\n", schedStatus))
		}
	}
	if hs, ok := health[objects.FieldKeyStatus].(string); ok && hs != "" {
		buf.WriteString(fmt.Sprintf("  Health Status: %s\n", hs))
	}
	if b, ok := health["blocking_issues"].(int); ok && b > 0 {
		buf.WriteString(fmt.Sprintf("  Blocking Issues: %d\n", b))
	}
	if w, ok := health["warnings"].(int); ok && w > 0 {
		buf.WriteString(fmt.Sprintf("  Warnings: %d\n", w))
	}
	if errStr, ok := health["error"].(string); ok && errStr != "" {
		buf.WriteString(fmt.Sprintf("  Error: %s\n", errStr))
	}
	return buf.String()
}


// formatStatusTablePlan formats priority plan information for table output
func formatStatusTablePlan(statusData map[string]any) string {
	plan, ok := statusData["current_priority_plan"].(map[string]any)
	if !ok {
		return ""
	}
	planID, okID := plan[objects.FieldKeyID].(string)
	title, okTitle := plan[objects.FieldKeyTitle].(string)
	if !okID || !okTitle {
		return ""
	}
	line := fmt.Sprintf("Current Priority Plan: %s - %s", planID, title)
	if n, ok := plan["backlog_items"].(int); ok && n >= 0 {
		line += fmt.Sprintf(" (%d backlog items)", n)
	}
	return line + "\n"
}

// formatStatusTableCapReview formats CAP stage review gate info for table output
func formatStatusTableCapReview(statusData map[string]any) string {
	capReview, ok := statusData["cap_review"].(map[string]any)
	if !ok {
		return ""
	}
	var buf strings.Builder
	buf.WriteString("\nCAP Review Gate Status:\n")
	if ts, ok := capReview["timestamp"].(string); ok && ts != "" {
		buf.WriteString(fmt.Sprintf("  Timestamp: %s\n", ts))
	}
	if b, ok := capReview["system_check"].(bool); ok {
		buf.WriteString(fmt.Sprintf("  System Check: %v\n", b))
	}
	if b, ok := capReview["system_check_effective"].(bool); ok {
		buf.WriteString(fmt.Sprintf("  System Check (effective): %v\n", b))
	}
	if fres, ok := capReview["freshness"].(string); ok && fres != "" {
		buf.WriteString(fmt.Sprintf("  Freshness: %s\n", fres))
	}
	if stale, ok := capReview["stale"].(bool); ok && stale {
		buf.WriteString("  Stale: true (do not treat as pristine)\n")
	}
	if b, ok := capReview["scheduler_health"].(bool); ok {
		buf.WriteString(fmt.Sprintf("  Scheduler Health: %v\n", b))
	}
	if b, ok := capReview["tests_passed"].(bool); ok {
		buf.WriteString(fmt.Sprintf("  Tests Passed: %v\n", b))
	}
	if errs, ok := capReview["errors"].([]any); ok && len(errs) > 0 {
		buf.WriteString("  Blocking Issues:\n")
		for _, e := range errs {
			buf.WriteString(fmt.Sprintf("    - %v\n", e))
		}
	}
	return buf.String()
}

// formatStatusTableStorage formats storage information for table output
func formatStatusTableStorage(statusData map[string]any) string {
	storageData, ok := statusData[objects.FieldKeyStorage].(map[string]any)
	if !ok {
		return ""
	}

	var result string
	result += "\nStorage:\n"
	if backend, ok := storageData["backend"].(string); ok {
		result += fmt.Sprintf("  Backend: %s\n", backend)
	}
	if dataDir, ok := storageData["data_dir"].(string); ok {
		result += fmt.Sprintf("  Data Directory: %s\n", dataDir)
	}
	if counts, ok := storageData["object_counts"].(map[string]int); ok {
		for kind, count := range counts {
			result += fmt.Sprintf("  %s: %d objects\n", kind, count)
		}
	}
	return result
}

// formatStatusTableActivity formats recent activity for table output
func formatStatusTableActivity(statusData map[string]any) string {
	activity, ok := statusData["recent_activity"].([]string)
	if !ok || len(activity) == 0 {
		return ""
	}

	var result string
	result += "\nRecent Activity:\n"
	for _, entry := range activity {
		result += fmt.Sprintf("  - %s\n", entry)
	}
	return result
}
