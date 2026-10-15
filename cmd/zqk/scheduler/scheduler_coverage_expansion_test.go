package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/agentdelivery"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/daemon/singleton"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/scheduler/hostservice"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestServiceCmd_HelpersAndTransitions(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// serviceRootOrCwd
	cwd, err := fileutil.Getwd()
	require.NoError(t, err)
	assert.NotEmpty(t, serviceRootOrCwd(""))
	assert.Equal(t, proj.Root, serviceRootOrCwd(proj.Root))
	assert.NotEmpty(t, serviceRootOrCwd(cwd))

	// requireSchedulerServiceControl
	cmd := NewServiceCmd()
	cmd.SetContext(context.Background())

	// Unauthorized
	t.Setenv(zqkenv.APIKey().Name(), "unauthorized")
	t.Setenv(zqkenv.TestBypassAuth().Name(), "")
	t.Setenv(zqkenv.TestRoot().Name(), "")
	err = requireSchedulerServiceControl(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scheduler service control requires")

	// Authorized via APIKey
	t.Setenv(zqkenv.APIKey().Name(), pkgctx.SystemAccountID)
	require.NoError(t, requireSchedulerServiceControl(cmd))

	// Authorized via TestBypassAuth
	t.Setenv(zqkenv.APIKey().Name(), "other")
	t.Setenv(zqkenv.TestBypassAuth().Name(), "1")
	require.NoError(t, requireSchedulerServiceControl(cmd))

	// Authorized via SecurityContext
	t.Setenv(zqkenv.TestBypassAuth().Name(), "")
	secCtx := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
	cmdWithSec := &cobra.Command{}
	cmdWithSec.SetContext(pkgctx.WithSecurityContext(context.Background(), secCtx))
	require.NoError(t, requireSchedulerServiceControl(cmdWithSec))

	adminSec := &pkgctx.SecurityContext{AccountID: "user-1", Roles: []string{"admin"}}
	cmdAdmin := &cobra.Command{}
	cmdAdmin.SetContext(pkgctx.WithSecurityContext(context.Background(), adminSec))
	require.NoError(t, requireSchedulerServiceControl(cmdAdmin))

	// parseServiceRoot
	t.Setenv(zqkenv.TestBypassAuth().Name(), "1")
	testCmd := &cobra.Command{}
	testCmd.Flags().String("root", "", "root")
	testCmd.Flags().Set("root", proj.Root)
	parsedRoot, err := parseServiceRoot(testCmd)
	require.NoError(t, err)
	assert.Equal(t, proj.Root, parsedRoot)

	// Service List
	listCmd := NewServiceCmd()
	outList, errList := executeSchedulerCmd(t, listCmd, []string{"list"}, proj.Root)
	_ = outList
	_ = errList

	// Service List --orphans
	outOrphans, errOrphans := executeSchedulerCmd(t, listCmd, []string{"list", "--orphans"}, proj.Root)
	_ = outOrphans
	_ = errOrphans
}

func TestConvergenceAgentPrompt_Helpers(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// normalizeAgentPromptAttentionMode
	m, err := normalizeAgentPromptAttentionMode("")
	require.NoError(t, err)
	assert.Equal(t, "", m)

	m, err = normalizeAgentPromptAttentionMode("none")
	require.NoError(t, err)
	assert.Equal(t, "", m)

	m, err = normalizeAgentPromptAttentionMode("off")
	require.NoError(t, err)
	assert.Equal(t, "", m)

	m, err = normalizeAgentPromptAttentionMode("test")
	require.NoError(t, err)
	assert.Equal(t, agentPromptAttentionAliasTestNonDirective, m)

	m, err = normalizeAgentPromptAttentionMode("non_directive")
	require.NoError(t, err)
	assert.Equal(t, agentPromptAttentionAliasTestNonDirective, m)

	m, err = normalizeAgentPromptAttentionMode("interrupt")
	require.NoError(t, err)
	assert.Equal(t, agentPromptAttentionAliasInterruptPlumbing, m)

	m, err = normalizeAgentPromptAttentionMode("critical_interrupt")
	require.NoError(t, err)
	assert.Equal(t, agentPromptAttentionAliasInterruptPlumbing, m)

	_, err = normalizeAgentPromptAttentionMode("invalid_mode_xyz")
	require.Error(t, err)

	// agentPromptIngestBlockedReasonCode
	assert.Equal(t, "unknown", agentPromptIngestBlockedReasonCode(errors.New("generic error")))

	// formatRollupBlockerLine
	assert.Equal(t, "`code` — detail", formatRollupBlockerLine(map[string]any{"code": "code", "detail": "detail"}))
	assert.Equal(t, "string_item", formatRollupBlockerLine("string_item"))
	assert.Equal(t, "123", formatRollupBlockerLine(123))

	// formatAgentPromptRollupSection
	emptyRollup := formatAgentPromptRollupSection(nil)
	assert.Contains(t, emptyRollup, "rollup_status_core missing")

	rollupWithData := map[string]any{
		"rollup_status":           "satisfied",
		"recommended_next_action": "proceed",
		objects.FieldKeyBlockers: []any{
			map[string]any{"code": "block_1", "detail": "detail_1"},
		},
		"field_key_literals_gate": map[string]any{
			"status":    "passed",
			"exit_code": 0,
		},
		"hardcoded_go_literals_scan": map[string]any{
			"status": "passed",
		},
		"rollup_gates_skipped": true,
	}
	formattedRollup := formatAgentPromptRollupSection(rollupWithData)
	assert.Contains(t, formattedRollup, "Next measured action (rollup)")
	assert.Contains(t, formattedRollup, "satisfied")
	assert.Contains(t, formattedRollup, "block_1")
	assert.Contains(t, formattedRollup, "Literal gates")

	// appendAgentPromptAttentionPreamble
	var b strings.Builder
	appendAgentPromptAttentionPreamble(&b, agentPromptAttentionAliasTestNonDirective)
	assert.Contains(t, b.String(), "Attention — test / non-directive")

	b.Reset()
	appendAgentPromptAttentionPreamble(&b, agentPromptAttentionAliasInterruptPlumbing)
	assert.Contains(t, b.String(), "Attention — critical interrupt / queue plumbing")

	// agentPromptPhaseRouterNotesContainUnknownVariant
	assert.False(t, agentPromptPhaseRouterNotesContainUnknownVariant(nil))
	assert.False(t, agentPromptPhaseRouterNotesContainUnknownVariant(map[string]any{
		objects.FieldKeyNotes: []any{"known variant note"},
	}))
	assert.True(t, agentPromptPhaseRouterNotesContainUnknownVariant(map[string]any{
		objects.FieldKeyNotes: []any{"Unknown flow_variant: custom_flow"},
	}))

	// appendAgentPromptCLIDiagnostic
	b.Reset()
	sug := map[string]any{
		objects.FieldKeyCurrentPhase: "execution",
	}
	routingMeta := map[string]any{
		agentPromptRoutingMetaFlowVariantSource:    "session_override",
		agentPromptRoutingMetaEffectiveFlowVariant: "variant_fast",
	}
	appendAgentPromptCLIDiagnostic(&b, sug, "variant_fast", routingMeta)
	assert.Contains(t, b.String(), "CLI diagnostic")
	assert.Contains(t, b.String(), "variant_fast")

	// convergenceFieldString
	testObj := map[string]any{
		"string_key": "val",
		"int_key":    42,
	}
	assert.Equal(t, "val", convergenceFieldString(testObj, "string_key"))
	assert.Equal(t, "", convergenceFieldString(testObj, "missing"))
	assert.Equal(t, "42", convergenceFieldString(testObj, "int_key"))

	// sessionPredictionsFromCVS
	assert.Nil(t, sessionPredictionsFromCVS(nil))
	preds := sessionPredictionsFromCVS(map[string]any{
		objects.FieldKeyPredictions: map[string]any{"p1": "v1"},
	})
	assert.Equal(t, "v1", preds["p1"])

	// Paths and JSONL logging
	keystrokePath := agentPromptIDEKeystrokeLogPath(proj.Root)
	assert.Contains(t, keystrokePath, agentPromptIDEKeystrokeLogFile)

	jsonlPath := agentPromptRunsJSONLPath(proj.Root)
	assert.Contains(t, jsonlPath, agentPromptRunsJSONLFile)

	writeAgentPromptRunJSONLLine(jsonlPath, []byte(`{"test":true}`))
	appendAgentPromptRunJSONL(proj.Root, map[string]any{"session": "CVS-1"})

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	appendAgentPromptIDEKeystrokeLog(logger, proj.Root, "test_event", "test_detail")
	detail := ideKeystrokeDetailPbcopyBegin(128, "/tmp/log")
	assert.Contains(t, detail, "markdown_bytes=128")

	// primaryAgentPromptDestination
	pDest, pathForAudit := primaryAgentPromptDestination("")
	assert.Equal(t, "stdout", pDest)
	assert.Equal(t, "", pathForAudit)

	pDest, pathForAudit = primaryAgentPromptDestination("/tmp/prompt.md")
	assert.Equal(t, "file", pDest)
	assert.Equal(t, "/tmp/prompt.md", pathForAudit)

	// effectiveAgentPromptHTTPDeliveryURL & effectiveAgentPromptHTTPBearer
	cmdPrompt := &cobra.Command{}
	cmdPrompt.Flags().String("post-url", "", "")
	assert.Equal(t, "", effectiveAgentPromptHTTPDeliveryURL(cmdPrompt))

	t.Setenv(zqkenv.AgentPromptDeliveryHTTPURL().Name(), "http://localhost:8080/hook")
	assert.Equal(t, "http://localhost:8080/hook", effectiveAgentPromptHTTPDeliveryURL(cmdPrompt))

	t.Setenv(zqkenv.AgentPromptDeliveryHTTPBearer().Name(), "secret-token")
	assert.Equal(t, "secret-token", effectiveAgentPromptHTTPBearer())

	// buildAppleScriptIDEPaste
	script, err := buildAppleScriptIDEPaste()
	require.NoError(t, err)
	assert.NotEmpty(t, script)
	assert.Contains(t, script, "tell application")

	// formatAgentMarkdown
	cvs := map[string]any{
		objects.FieldKeyTitle:           "Test CVS Session",
		objects.FieldKeyStatus:          "in_progress",
		objects.FieldKeyCurrentPhase:    "analysis",
		objects.FieldKeyDesiredEndState: "All tests pass",
		objects.FieldKeyHypothesis:      "Code passes when fixed",
		objects.FieldKeyNextAction:      "Run verification",
	}
	snap := &schedpkg.TestBundleConvergenceSnapshot{
		DeltaAssessment:           "advancing",
		ReadyForSessionCompletion: false,
		LinesInWindow:             10,
		FailingFingerprintsNow:    []string{"pkg.TestA"},
	}
	persistOut := &persistSessionOutcome{
		Requested: true,
		Applied:   true,
	}
	markdown := formatAgentMarkdown("CVS-TEST-1", cvs, sug, snap, "test_non_directive", persistOut, rollupWithData, "default", routingMeta, "Linked Backlog Text")
	assert.Contains(t, markdown, "CVS-TEST-1")
	assert.Contains(t, markdown, "Code passes when fixed")
	assert.Contains(t, markdown, "Next measured action (rollup)")
	assert.Contains(t, markdown, "Linked Backlog Text")
}

func TestConvergenceAgentPromptBacklog_Helpers(t *testing.T) {
	// truncateRunesForPrompt
	assert.Equal(t, "hello", truncateRunesForPrompt("hello", 10))
	assert.Equal(t, "hell…", truncateRunesForPrompt("hello world", 4))

	// isCriteriaRefToken
	assert.True(t, isCriteriaRefToken("CRIT-123"))
	assert.True(t, isCriteriaRefToken("CRIT-ABC-DEF"))
	assert.False(t, isCriteriaRefToken("BLI-123"))
	assert.False(t, isCriteriaRefToken("req"))

	// backlogItemRefsFromCVS
	assert.Nil(t, backlogItemRefsFromCVS(nil))
	assert.Nil(t, backlogItemRefsFromCVS(map[string]any{}))
	refs := backlogItemRefsFromCVS(map[string]any{
		objects.FieldKeyBacklogItemRefs: []any{"BLI-1", "  BLI-2  ", 123},
	})
	assert.Equal(t, []string{"BLI-1", "BLI-2"}, refs)

	// formatCriteriaAcceptanceBulletUnresolved
	bullet := formatCriteriaAcceptanceBulletUnresolved("CRIT-100", errors.New("not found"))
	assert.Contains(t, bullet, "CRIT-100")
	assert.Contains(t, bullet, "not found")

	// formatCriteriaAcceptanceBulletResolved
	hints := make(map[string]struct{})
	critObj := map[string]any{
		objects.FieldKeyID:               "CRIT-200",
		objects.FieldKeyTitle:            "Coverage elevated to >=80%",
		objects.FieldKeyStatus:           "in_progress",
		objects.FieldKeyValidationMethod: criteriaValidationMethodAutomatedTest,
		objects.FieldKeyContext:          "Run go test -cover ./cmd/zqk/scheduler/...",
	}
	resolved := formatCriteriaAcceptanceBulletResolved(critObj, hints)
	assert.Contains(t, resolved, "CRIT-200")
	assert.Contains(t, resolved, "Coverage elevated to >=80%")
	assert.Contains(t, resolved, "automated_test")
	assert.Contains(t, resolved, "Programmatic verification")
}

func TestShowActivityHelpers_Methods(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)

	// parseActivitySince
	t1, err := parseActivitySince("24h", now)
	require.NoError(t, err)
	assert.Equal(t, now.Add(-24*time.Hour), t1)

	t2, err := parseActivitySince("30m", now)
	require.NoError(t, err)
	assert.Equal(t, now.Add(-30*time.Minute), t2)

	rfc := now.Format(time.RFC3339)
	t3, err := parseActivitySince(rfc, now)
	require.NoError(t, err)
	assert.Equal(t, now, t3)

	_, err = parseActivitySince("", now)
	require.Error(t, err)

	_, err = parseActivitySince("invalid-since", now)
	require.Error(t, err)

	// parseEventMetadata
	evtWithMeta := map[string]any{
		"metadata": map[string]any{
			"duration_seconds": 1.5,
			"error":            "job failed",
			"success":          false,
		},
	}
	dur, errMsg, succ := parseEventMetadata(evtWithMeta)
	assert.Equal(t, "1.50s", dur)
	assert.Equal(t, "job failed", errMsg)
	assert.False(t, succ)

	// coerceStringKeyMap & coerceFloat64
	m, ok := coerceStringKeyMap(map[string]any{"k": "v"})
	assert.True(t, ok)
	assert.Equal(t, "v", m["k"])

	m2, ok2 := coerceStringKeyMap(map[any]any{"k2": "v2"})
	assert.True(t, ok2)
	assert.Equal(t, "v2", m2["k2"])

	_, ok3 := coerceStringKeyMap("not a map")
	assert.False(t, ok3)

	f, ok := coerceFloat64(42)
	assert.True(t, ok)
	assert.Equal(t, 42.0, f)

	f, ok = coerceFloat64(3.14)
	assert.True(t, ok)
	assert.Equal(t, 3.14, f)

	_, ok = coerceFloat64("not a float")
	assert.False(t, ok)

	// determineEventOutcome
	startedMap := make(map[string]jobActivityEvent)
	completedMap := make(map[string]bool)
	assert.Equal(t, "Success", determineEventOutcome("scheduler_job_completed", "JOB-1", rfc, startedMap, completedMap))
	assert.Equal(t, "Failed", determineEventOutcome("scheduler_job_failed", "JOB-1", rfc, startedMap, completedMap))
	assert.Equal(t, "Started", determineEventOutcome("scheduler_job_started", "JOB-1", rfc, startedMap, completedMap))

	// formatRelativeAge
	assert.Equal(t, "5m ago", formatRelativeAge(now.Add(-5*time.Minute), now))
	assert.Equal(t, "2h ago", formatRelativeAge(now.Add(-2*time.Hour), now))
	assert.Equal(t, "1d ago", formatRelativeAge(now.Add(-25*time.Hour), now))
	assert.Equal(t, "just now", formatRelativeAge(now.Add(5*time.Minute), now))

	// formatActivityCompactUTC
	assert.NotEmpty(t, formatActivityCompactUTC(now, now))
	assert.NotEmpty(t, formatActivityCompactUTC(time.Time{}, now))

	// formatOutcomeWithEmoji
	assert.Contains(t, formatOutcomeWithEmoji("scheduler_job_completed", "Success"), "Success")
	assert.Contains(t, formatOutcomeWithEmoji("scheduler_job_failed", "Failed"), "Failed")
	assert.Contains(t, formatOutcomeWithEmoji("scheduler_job_started", "Started"), "Started")

	// stuckJobAssessmentParts
	code, detail := stuckJobAssessmentParts(time.Minute, 2*time.Minute, 30*time.Minute)
	assert.Equal(t, "REC", code)
	assert.Contains(t, detail, "likely healthy in-flight")

	code, detail = stuckJobAssessmentParts(5*time.Minute, 2*time.Minute, 30*time.Minute)
	assert.Equal(t, "WCH", code)
	assert.Contains(t, detail, "still running")

	code, detail = stuckJobAssessmentParts(45*time.Minute, 2*time.Minute, 30*time.Minute)
	assert.Equal(t, "INV", code)
	assert.Contains(t, detail, "unusually long")

	// Buffer output helpers
	var buf strings.Builder
	outputDaemonDownWarningToBuffer(&buf, true)
	assert.Contains(t, buf.String(), "Scheduler daemon is not running")

	buf.Reset()
	outputBusynessToBuffer(&buf, SchedulerBusyness{
		Executing:        2,
		PendingInQueue:   1,
		CompletedSuccess: 5,
		CompletedFailed:  1,
	})
	assert.Contains(t, buf.String(), "Scheduler busyness")

	buf.Reset()
	outputCronLegendToBuffer(&buf)
	assert.Contains(t, buf.String(), "Cron schedule")

	buf.Reset()
	appendCronFieldsLine(&buf, "*/5 * * * *")
	assert.Contains(t, buf.String(), "min=*/5")

	buf.Reset()
	outputMissedTriggersLegendToBuffer(&buf)
	assert.Contains(t, buf.String(), "high-freq grace")

	buf.Reset()
	outputStuckJobsLegendToBuffer(&buf, 2*time.Minute, 30*time.Minute)
	assert.Contains(t, buf.String(), "REC within 2m")

	// Table columns
	assert.Greater(t, effTableWidth(), 0)
	assert.Greater(t, effColJobID(), 0)
	assert.Greater(t, effMissedColSchedule(), 0)

	// parseActivityEventTime
	tParsed, ok := parseActivityEventTime(now.Format(time.RFC3339))
	assert.True(t, ok)
	assert.Equal(t, now, tParsed)

	_, ok = parseActivityEventTime("not a valid time")
	assert.False(t, ok)
}

func TestShowHistoryHelpers_Methods(t *testing.T) {
	// parseTimeString
	tSince := parseTimeString("1h", true)
	require.NotNil(t, tSince)
	assert.WithinDuration(t, time.Now().UTC().Add(-time.Hour), *tSince, 5*time.Second)

	tUntil := parseTimeString("30m", false)
	require.NotNil(t, tUntil)
	assert.WithinDuration(t, time.Now().UTC().Add(30*time.Minute), *tUntil, 5*time.Second)

	tToday := parseTimeString("today", true)
	require.NotNil(t, tToday)

	assert.Nil(t, parseTimeString("", true))
	assert.Nil(t, parseTimeString("not-a-valid-time", false))

	// determineOutcomeFromEventType
	assert.Equal(t, "Success", determineOutcomeFromEventType("scheduler_job_completed"))
	assert.Equal(t, "Failed", determineOutcomeFromEventType("scheduler_job_failed"))
	assert.Equal(t, "scheduler_job_started", determineOutcomeFromEventType("scheduler_job_started"))
	assert.Equal(t, "unknown_event", determineOutcomeFromEventType("unknown_event"))

	// Job stats sorting (alphabetical ascending by JobID)
	stats1 := &jobStats{JobID: "JOB-A", TotalRuns: 5, LastRunAt: time.Now().Add(-time.Hour).Format(time.RFC3339)}
	stats2 := &jobStats{JobID: "JOB-B", TotalRuns: 10, LastRunAt: time.Now().Format(time.RFC3339)}
	statsList := []*jobStats{stats2, stats1}
	sortJobStatsList(statsList)
	assert.Equal(t, "JOB-A", statsList[0].JobID)
	assert.Equal(t, "JOB-B", statsList[1].JobID)
}

func TestTestFailures_ConvergenceRollupAndHelpers(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// gateExitForDisplay
	assert.Nil(t, gateExitForDisplay(0, true))
	assert.Equal(t, 0, gateExitForDisplay(0, false))
	assert.Equal(t, 1, gateExitForDisplay(1, false))

	// runLiteralRepoGates
	fk, ze := runLiteralRepoGates(proj.Root)
	assert.Equal(t, 0, fk)
	assert.Equal(t, 0, ze)

	// cvsIDsFromRelatedObjectRefs
	assert.Empty(t, cvsIDsFromRelatedObjectRefs(nil))
	assert.Empty(t, cvsIDsFromRelatedObjectRefs([]any{"not-cvs", 123}))
	cvsRefs := cvsIDsFromRelatedObjectRefs([]any{"CVS-1", "CVS-2", "REQ-1"})
	assert.Equal(t, []string{"CVS-1", "CVS-2"}, cvsRefs)

	// buildRollupStatusCore with skipSessionContext=true
	snap := &schedpkg.TestBundleConvergenceSnapshot{
		ReadyForSessionCompletion: true,
		DeltaAssessment:           "neutral",
	}
	cmd := &cobra.Command{}
	rollupCore, err := buildRollupStatusCore(cmd, proj.Root, true, "", snap, true)
	require.NoError(t, err)
	assert.NotNil(t, rollupCore)
	assert.Equal(t, "satisfied", rollupCore["rollup_status"])
	assert.True(t, rollupCore["rollup_gates_skipped"].(bool))

	// resolveDebriefNotesFromFlags
	notes, err := resolveDebriefNotesFromFlags("  direct notes  ", "")
	require.NoError(t, err)
	assert.Equal(t, "direct notes", notes)

	emptyNotes, err := resolveDebriefNotesFromFlags("", "")
	require.NoError(t, err)
	assert.Empty(t, emptyNotes)

	// From file
	tmpNotesFile := filepath.Join(proj.Root, "debrief.txt")
	require.NoError(t, fileutil.WriteFile(tmpNotesFile, []byte("file notes"), paths.FilePerm600))
	fileNotes, err := resolveDebriefNotesFromFlags("direct", tmpNotesFile)
	require.NoError(t, err)
	assert.Equal(t, "file notes", fileNotes)

	// Overflow validation
	hugeNotes := strings.Repeat("x", maxConvergenceDebriefNotesLen+1)
	_, err = resolveDebriefNotesFromFlags(hugeNotes, "")
	require.Error(t, err)

	hugeFile := filepath.Join(proj.Root, "huge_debrief.txt")
	require.NoError(t, fileutil.WriteFile(hugeFile, []byte(hugeNotes), paths.FilePerm600))
	_, err = resolveDebriefNotesFromFlags("", hugeFile)
	require.Error(t, err)

	// firstLine
	assert.Equal(t, "first line", firstLine("first line\nsecond line"))
	assert.Equal(t, "", firstLine(nil))
	assert.Equal(t, "single", firstLine("single"))

	// summarizeSuggestedSessionFields
	sugStr, err := summarizeSuggestedSessionFields(snap, "execution", "default", nil, nil, false, nil, false, "notes", nil)
	require.NoError(t, err)
	assert.Contains(t, sugStr, "current_phase")

	// buildConvergenceOutputMap
	outputMap, err := buildConvergenceOutputMap(snap, "CVS-1", "execution", "default", nil, nil, false, nil, false, "notes", nil, nil, rollupCore)
	require.NoError(t, err)
	assert.NotNil(t, outputMap)
	assert.Equal(t, "CVS-1", outputMap["convergence_session_id"])

	// resolveConvergenceRoutingForCLI with empty sessionID
	phase, flow, meta, before, preds, thresh, err := resolveConvergenceRoutingForCLI(cmd, "", "p1", "f1", true)
	require.NoError(t, err)
	assert.Equal(t, "p1", phase)
	assert.Equal(t, "f1", flow)
	assert.Nil(t, meta)
	assert.Nil(t, before)
	assert.Nil(t, preds)
	assert.Nil(t, thresh)
}

func TestTestFailures_HealthAndMerge(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// extractTestHealthMapString
	m := map[string]any{"key1": "val1", "key2": 123}
	assert.Equal(t, "val1", extractTestHealthMapString(m, "key1"))
	assert.Equal(t, "", extractTestHealthMapString(m, "missing"))
	assert.Equal(t, "", extractTestHealthMapString(m, "key2"))

	// resolveHealthSignalMessage
	assert.Contains(t, resolveHealthSignalMessage(0, 0, 0), "no data")
	assert.Contains(t, resolveHealthSignalMessage(10, 10, 0), "green")
	assert.Contains(t, resolveHealthSignalMessage(10, 6, 4), "mixed")
	assert.Contains(t, resolveHealthSignalMessage(10, 2, 8), "needs attention")

	// readTestBundleHealthTailForAgentPrompt (when missing)
	lines, err := readTestBundleHealthTailForAgentPrompt(proj.Root, 100)
	require.NoError(t, err)
	assert.Empty(t, lines)

	// write mock health.jsonl
	healthDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerDir, "cvs", "test-bundles")
	require.NoError(t, fileutil.MkdirAll(healthDir, paths.DirPerm750))
	healthFile := filepath.Join(healthDir, "health.jsonl")

	mockEvents := []map[string]any{
		{
			"timestamp":          time.Now().Format(time.RFC3339),
			"test_outcome":       "pass",
			"bundle_fingerprint": "fp1",
		},
		{
			"timestamp":          time.Now().Format(time.RFC3339),
			"test_outcome":       "fail",
			"bundle_fingerprint": "fp2",
			"event_type":         schedulerStateFailed,
			"test_failures":      []any{"github.com/zqk-os/zqk/pkg/storage.TestA"},
		},
	}
	var healthBuf strings.Builder
	for _, ev := range mockEvents {
		b, _ := json.Marshal(ev)
		healthBuf.Write(b)
		healthBuf.WriteString("\n")
	}
	require.NoError(t, fileutil.WriteFile(healthFile, []byte(healthBuf.String()), paths.FilePerm600))

	// readTestBundleHealthTailForAgentPrompt (when present)
	lines, err = readTestBundleHealthTailForAgentPrompt(proj.Root, 100)
	require.NoError(t, err)
	assert.Equal(t, 2, len(lines))

	// mergeTestBundleEventFailures
	failuresByPackage := make(map[string][]string)
	var totalFailures int
	err = mergeTestBundleEventFailures(proj.Root, time.Now().Add(-time.Hour), "", failuresByPackage, &totalFailures)
	require.NoError(t, err)
	assert.Equal(t, 1, totalFailures)
	assert.Contains(t, failuresByPackage, "./pkg/storage")

	// parseCallbackTestFailures
	cbFailures := make(map[string][]string)
	cbEntry := map[string]any{
		objects.FieldKeyEventType: schedulerStateFailed,
		"test_failures": []any{
			"github.com/zqk-os/zqk/pkg/mesh.TestMeshBroker",
			"github.com/zqk-os/zqk/cmd/zqk.TestRoot",
		},
	}
	parseCallbackTestFailures(cbEntry, "mesh", func(pkg, testName string) {
		cbFailures[pkg] = append(cbFailures[pkg], testName)
	})
	assert.Contains(t, cbFailures, "./pkg/mesh")
	assert.NotContains(t, cbFailures, "./cmd/zqk")

	// walkLogFileLines
	var walkedLines []string
	err = walkLogFileLines(healthDir, time.Now().Add(-time.Hour), []string{".jsonl"}, func(line string) {
		walkedLines = append(walkedLines, line)
	})
	require.NoError(t, err)
	assert.Equal(t, 2, len(walkedLines))
}

func TestCriteriaEvidence_Helpers(t *testing.T) {
	// gatherLatestCriteriaEvidence
	now := time.Now().Format(time.RFC3339)
	later := time.Now().Add(time.Minute).Format(time.RFC3339)

	rows := []map[string]any{
		{
			schedpkg.KeyEventType:                     schedpkg.KeyEventTypeCriteriaVerificationEvidence,
			objects.FieldKeyCriteriaRefs:              []any{"CRIT-101", "CRIT-102"},
			schedpkg.KeyTimestamp:                     now,
			schedpkg.KeyCriteriaVerificationSatisfied: false,
			schedpkg.KeyJobID:                         "JOB-OLD",
		},
		{
			schedpkg.KeyEventType:                     schedpkg.KeyEventTypeCriteriaVerificationEvidence,
			objects.FieldKeyCriteriaRefs:              []any{"CRIT-101"},
			schedpkg.KeyTimestamp:                     later,
			schedpkg.KeyCriteriaVerificationSatisfied: true,
			schedpkg.KeyJobID:                         "JOB-NEW",
		},
		{
			schedpkg.KeyEventType:        "other_event",
			objects.FieldKeyCriteriaRefs: []any{"CRIT-999"},
		},
	}

	evidence := gatherLatestCriteriaEvidence(rows)
	require.NotNil(t, evidence)
	assert.Len(t, evidence, 2)

	crit101 := evidence["CRIT-101"]
	require.NotNil(t, crit101)
	assert.True(t, crit101[schedpkg.KeyCriteriaVerificationSatisfied].(bool))
	assert.Equal(t, "JOB-NEW", crit101[schedpkg.KeyJobID])

	crit102 := evidence["CRIT-102"]
	require.NotNil(t, crit102)
	assert.False(t, crit102[schedpkg.KeyCriteriaVerificationSatisfied].(bool))
	assert.Equal(t, "JOB-OLD", crit102[schedpkg.KeyJobID])
}

func TestSchedulerBundleProgress_Helpers(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// Missing progress file returns default pending rows (empty in fresh temp project)
	rows, err := readBundleProgress(proj.Root)
	require.NoError(t, err)
	assert.Empty(t, rows)

	// Write mock progress events
	progressFile := schedpkg.TestBundlesProgressFilePath(proj.Root)
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(progressFile), paths.DirPerm750))

	startTime := time.Now().Add(-10 * time.Second).Format(time.RFC3339)
	endTime := time.Now().Format(time.RFC3339)

	mockEvents := []map[string]any{
		{"job_id": "JOB-1", objects.FieldKeyEventType: "started", "timestamp": startTime},
		{"job_id": "JOB-1", objects.FieldKeyEventType: "pass", "timestamp": endTime},
		{"job_id": "JOB-2", objects.FieldKeyEventType: "started", "timestamp": startTime},
		{"job_id": "JOB-2", objects.FieldKeyEventType: "fail", "timestamp": endTime},
		{"job_id": "JOB-3", objects.FieldKeyEventType: "started", "timestamp": startTime},
	}
	var buf strings.Builder
	for _, ev := range mockEvents {
		b, _ := json.Marshal(ev)
		buf.Write(b)
		buf.WriteString("\n")
	}
	require.NoError(t, fileutil.WriteFile(progressFile, []byte(buf.String()), paths.FilePerm600))

	rows, err = readBundleProgress(proj.Root)
	require.NoError(t, err)
	assert.Len(t, rows, 3)

	// Run command
	cmd := NewSchedulerBundleProgressCmd()
	out, err := executeSchedulerCmd(t, cmd, []string{}, proj.Root)
	require.NoError(t, err)
	assert.Contains(t, out, "Bundle Progress Matrix")
	assert.Contains(t, out, "JOB-1")
	assert.Contains(t, out, "pass")
	assert.Contains(t, out, "JOB-2")
	assert.Contains(t, out, "fail")
	assert.Contains(t, out, "JOB-3")
	assert.Contains(t, out, "running")
}

func TestSchedulerHealth_DaemonUnhealthyHelpers(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// daemonUnhealthyPath
	p := daemonUnhealthyPath(proj.Root)
	assert.Contains(t, p, paths.SchedulerDaemonUnhealthyFile)

	// writeDaemonUnhealthyFile
	writeDaemonUnhealthyFile(proj.Root, "test_reason", 3, "Test failure message")
	assert.True(t, fileutil.Exists(p))

	data, err := fileutil.ReadFile(p)
	require.NoError(t, err)
	var payload daemonUnhealthyPayload
	require.NoError(t, json.Unmarshal(data, &payload))
	assert.Equal(t, "test_reason", payload.Reason)
	assert.Equal(t, 3, payload.EnabledTimerJobs)
	assert.Equal(t, "Test failure message", payload.Message)

	// removeDaemonUnhealthyFile
	removeDaemonUnhealthyFile(proj.Root)
	assert.False(t, fileutil.Exists(p))

	// recordExternalHealthMetric
	storageProvider, err := cli.NewStorageProviderFromFactory(pkgctx.NewSystemContext(), proj.Root)
	require.NoError(t, err)
	secCtx := pkgctx.NewSystemSecurityContext()

	// Zero values do nothing
	recordExternalHealthMetric(storageProvider, secCtx, time.Now(), 0, 0, 0, 0, 0)

	// Non-zero values record metric
	recordExternalHealthMetric(storageProvider, secCtx, time.Now(), 1, 2, 0, 0, 150*time.Millisecond)
}

func TestTestFailuresAnalyze_PipelineStages(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	stageCtx := &pipeline.Context{
		Ctx:     pkgctx.NewSystemContext(),
		Outcome: make(map[string]any),
	}

	payload := &testFailureAnalysisPayload{
		projectRoot:          proj.Root,
		minFailures:          1,
		failureRateThreshold: 0.5,
		failureStats:         make(map[string]*TestFailureStats),
	}

	// Stage 2: Collect logs
	_, err := stageCollectLogs(stageCtx, payload)
	require.NoError(t, err)

	// Stage 3: Analyze failures with mock lines
	tFail := time.Now().Add(-5 * time.Minute)
	payload.logLines = []string{
		fmt.Sprintf(`{"event_type":"failed","timestamp":"%s","test_failures":["github.com/zqk-os/zqk/pkg/storage.TestA"]}`, tFail.Format(time.RFC3339)),
		fmt.Sprintf(`{"event_type":"failed","timestamp":"%s","test_failures":["github.com/zqk-os/zqk/pkg/storage.TestA"]}`, tFail.Add(time.Minute).Format(time.RFC3339)),
	}

	_, err = stageAnalyzeFailures(stageCtx, payload)
	require.NoError(t, err)
	assert.Equal(t, 1, len(payload.persistentFailures))
	assert.Equal(t, 2, payload.persistentFailures[0].FailureCount)

	// Stage 4: Create backlog items
	_, err = stageCreateBacklogItems(stageCtx, payload)
	require.NoError(t, err)
	assert.Contains(t, payload.outputBuf.String(), "Found 1 persistent test failure")

	// Stage 5: Finalize
	cmd := &cobra.Command{}
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	payload.cmd = cmd
	_, err = stageFinalizeOutput(stageCtx, payload)
	require.NoError(t, err)
}

func TestSchedulerSubmitCmd_Execution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// Execute submit command with flags
	cmd := NewSubmitCmd()
	out, err := executeSchedulerCmd(t, cmd, []string{
		"echo hello",
		"--title", "Test Submit Job",
		"--description", "Job description for test",
		"--max-runtime", "60",
		"--workdir", proj.Root,
		"--env", "TEST_KEY=TEST_VAL",
		"--retry", "2",
		"--retry-delay", "3",
	}, proj.Root)
	require.NoError(t, err)
	assert.Contains(t, out, "Job submitted:")
	assert.Contains(t, out, "SCH-")
}

func TestConvergenceNest_Commands(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	convCmd := NewSchedulerConvergenceCmd()

	// 1. Nest spawn without required flags fails
	_, err := executeSchedulerCmd(t, convCmd, []string{"nest-spawn", "--parent-session-id", "CVS-PARENT"}, proj.Root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires --title, --hypothesis, and --desired-end-state")

	// 2. Nest status on nonexistent session
	outStatus, errStatus := executeSchedulerCmd(t, convCmd, []string{"nest-status", "--parent-session-id", "CVS-NONEXISTENT"}, proj.Root)
	_ = outStatus
	_ = errStatus

	// 3. Convergence measure in JSON
	outJSON, errJSON := executeSchedulerCmd(t, convCmd, []string{"measure", "--format", "json"}, proj.Root)
	_ = outJSON
	_ = errJSON

	// 4. Convergence measure in agent-prompt with skip-session-context
	outPrompt, errPrompt := executeSchedulerCmd(t, convCmd, []string{"measure", "--format", "agent-prompt", "--skip-session-context"}, proj.Root)
	_ = outPrompt
	_ = errPrompt

	// 5. Convergence overseer missing coordinator
	_, errOverseer := executeSchedulerCmd(t, convCmd, []string{"overseer"}, proj.Root)
	require.Error(t, errOverseer)
	assert.Contains(t, errOverseer.Error(), "--coordinator-session-id is required")
}

func TestRecordCvsOrchestrateRun_Execution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := NewRecordCvsOrchestrateRunCmd()
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	// Empty stdin returns error
	err := runRecordCvsOrchestrateRun(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stdin is empty")
}

func TestSchedulerCoordination_ExtraEvents(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	ctx := context.Background()
	emitSchedulerHistoryDebugEventViaCoordinator(ctx, proj.Root, nil, "filter", 10, 5, "debug")
	emitSchedulerOutputErrorViaCoordinator(ctx, proj.Root, nil, "json", errors.New("mock formatting error"), "debug")
}

func TestServiceCmd_ExtraTransitions(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestBypassAuth().Name(), "1")

	// Service GC
	cmd := NewServiceCmd()
	outGC, errGC := executeSchedulerCmd(t, cmd, []string{"gc"}, proj.Root)
	require.NoError(t, errGC)
	assert.NotEmpty(t, outGC)

	// Service Rebind without required flags
	_, errRebind := executeSchedulerCmd(t, cmd, []string{"rebind"}, proj.Root)
	require.Error(t, errRebind)
	assert.Contains(t, errRebind.Error(), "required")

	// Service Uninstall on unregistered root returns error
	_, errUninstall := executeSchedulerCmd(t, cmd, []string{"uninstall", "--root", proj.Root}, proj.Root)
	require.Error(t, errUninstall)
	assert.Contains(t, errUninstall.Error(), "no host service registry entry")
}

func TestDaemonStdio_CopyGoroutine(t *testing.T) {
	var wg sync.WaitGroup
	var buf bytes.Buffer
	r := strings.NewReader("sample stdio copy content")
	startCopyGoroutine(&wg, "test_stdio_copy", "testing copy", &buf, r)
	wg.Wait()
	assert.Equal(t, "sample stdio copy content", buf.String())
}

func TestCEFDiamond_ConvergenceMeasureExecution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringP("format", "f", "json", "")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	// Measure when surface scorecard does not exist
	err := runCEFDiamondConvergenceMeasure(cliCtx, cmd, proj.Root, "CVS-CEF-001", false, "c1", "default", map[string]any{}, map[string]any{})
	_ = err

	// Switch format to table
	_ = cmd.Flags().Set("format", "table")
	err = runCEFDiamondConvergenceMeasure(cliCtx, cmd, proj.Root, "CVS-CEF-001", false, "c1", "default", map[string]any{}, map[string]any{})
	_ = err
}

func TestSchedulerDaemon_HelpersAndConfiguration(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. rejectHostServiceOwnedBackgroundStart when no host service
	err := rejectHostServiceOwnedBackgroundStart(proj.Root)
	assert.NoError(t, err)

	// 2. configureSchedulerDaemonExec
	execCmd := &exec.Cmd{
		Path: "/bin/echo",
		Args: []string{"echo", "test"},
	}
	cleanup, err := configureSchedulerDaemonExec(execCmd, proj.Root, "test-label")
	if err == nil && cleanup != nil {
		defer cleanup()
		assert.NotEmpty(t, execCmd.Dir)
	}
}

func TestSchedulerStop_InProcessAndStorageHelpers(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// getStorageProviderForCoordinator on project root
	sp := getStorageProviderForCoordinator(proj.Root)
	assert.NotNil(t, sp)

	// getStorageProviderForCoordinator on empty or invalid path
	spNil := getStorageProviderForCoordinator("")
	assert.Nil(t, spNil)

	// Idempotent stop when scheduler is not running
	cmd := NewStopCmd()
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)
	err := stopScheduler(cliCtx, cmd)
	require.NoError(t, err)
}

func TestCriteriaEvidence_GatherAndFormat(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// When events.jsonl does not exist, runCriteriaEvidence returns friendly message
	out, err := executeSchedulerCmd(t, NewTestFailuresCriteriaEvidenceCmd(), []string{}, proj.Root)
	require.NoError(t, err)
	assert.Contains(t, out, "No test-bundle events file yet")
}

func TestTestFailuresListAndRerun_EmptyLogs(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// When no test logs exist at all
	outList, err := executeSchedulerCmd(t, NewTestFailuresListCmd(), []string{}, proj.Root)
	require.NoError(t, err)
	assert.Contains(t, outList, "No test logs found")

	// Rerun when no callback logs or events exist
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cmdRerun := NewTestFailuresRerunCmd()
	cli.SetContext(cmdRerun, cliCtx)
	err = rerunTestFailures(cliCtx, cmdRerun)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no callback logs or test-bundle events found")
}

func TestConvergenceAgentPrompt_HelpersAndFormatting(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. Acceptance criteria line resolution
	cmdProc := &cobra.Command{}
	cmdCtx := pkgctx.WithSecurityContext(pkgctx.NewSystemContext(), pkgctx.NewSystemSecurityContext())
	cmdProc.SetContext(cmdCtx)
	cli.SetContext(cmdProc, cli.ContextForProjectRoot(proj.Root))

	proc, err := cli.NewProcessor(cmdProc)
	if err == nil && proc != nil {
		globalHints := make(map[string]struct{})
		_ = acceptanceCriteriaMarkdownLine(proc, "CRIT-UNRESOLVED-001", logging.GetLoggerFromProfile("system"), globalHints)
		_ = acceptanceCriteriaMarkdownLine(proc, 12345, logging.GetLoggerFromProfile("system"), globalHints)
	}

	// 2. HTTP delivery URLs and Bearer
	cmd := &cobra.Command{}
	cmd.Flags().String("agent-prompt-delivery-http-url", "https://example.com/prompt", "")
	url := effectiveAgentPromptHTTPDeliveryURL(cmd)
	assert.Equal(t, "https://example.com/prompt", url)
	assert.Empty(t, effectiveAgentPromptHTTPBearer())

	// 3. appendAgentPromptIDEKeystrokeLog
	logger := logging.GetLoggerFromProfile("system")
	appendAgentPromptIDEKeystrokeLog(logger, proj.Root, "test_event", "test_detail")
	logFile := agentPromptIDEKeystrokeLogPath(proj.Root)
	_, statErr := fileutil.Stat(logFile)
	assert.NoError(t, statErr)
}

func TestConvergenceAgentPrompt_DeliveryPipeline(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. primaryAgentPromptDestination
	dest1, path1 := primaryAgentPromptDestination("")
	assert.Equal(t, "stdout", dest1)
	assert.Empty(t, path1)

	dest2, path2 := primaryAgentPromptDestination("-")
	assert.Equal(t, "stdout", dest2)
	assert.Empty(t, path2)

	dest3, path3 := primaryAgentPromptDestination("/tmp/prompt.md")
	assert.Equal(t, "file", dest3)
	assert.Equal(t, "/tmp/prompt.md", path3)

	// 2. deliverPrimaryAgentPrompt
	cmd := &cobra.Command{Use: "prompt"}
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	// Deliver to stdout
	delivered, prim, auditPath, err := deliverPrimaryAgentPrompt(cmd, context.Background(), []byte("# Markdown Prompt"), "CVS-001", "-")
	require.NoError(t, err)
	assert.Equal(t, []string{"stdout"}, delivered)
	assert.Equal(t, "stdout", prim)
	assert.Empty(t, auditPath)

	// Deliver to file
	targetFile := filepath.Join(proj.Root, "prompt_out.md")
	deliveredFile, primFile, auditPathFile, errFile := deliverPrimaryAgentPrompt(cmd, context.Background(), []byte("# File Markdown"), "CVS-001", targetFile)
	require.NoError(t, errFile)
	assert.Equal(t, []string{"file:" + targetFile}, deliveredFile)
	assert.Equal(t, "file", primFile)
	assert.Equal(t, targetFile, auditPathFile)

	// 3. deliverAgentPromptMarkdownOutput
	errDeliv := deliverAgentPromptMarkdownOutput(cmd, proj.Root, "# Test Prompt", "\nFooter", "CVS-001", "none", "auto", "")
	assert.NoError(t, errDeliv)

	// 4. agentPromptStageCommit
	plNone := &agentPromptPipelinePayload{
		markdown:    "# MD",
		projectRoot: proj.Root,
	}
	pctx := &pipeline.Context{Ctx: context.Background(), Outcome: make(map[string]any)}
	resNone, errStage := agentPromptStageCommit(pctx, plNone)
	require.NoError(t, errStage)
	assert.NotNil(t, resNone)
	assert.Equal(t, agentPromptDeliverNone, pctx.Outcome[outcomeAgentPromptDeliverMode])

	// 5. agentPromptStageFinalize
	resFin, errFin := agentPromptStageFinalize(pctx, plNone)
	require.NoError(t, errFin)
	assert.NotNil(t, resFin)
	assert.True(t, pctx.Outcome[outcomeAgentPromptComplete].(bool))
}

func TestCriteriaEvidence_ApplyValidatedAndFormatting(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// cloneMapShallow
	orig := map[string]any{"key1": "val1", "key2": 42}
	cloned := cloneMapShallow(orig)
	assert.Equal(t, orig, cloned)

	// Processor for storage
	cmd := &cobra.Command{Use: "crit"}
	cmdCtx := pkgctx.WithSecurityContext(pkgctx.NewSystemContext(), pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cmdCtx)
	cli.SetContext(cmd, cli.ContextForProjectRoot(proj.Root))
	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	// 1. applyCriteriaValidated on non-existent criteria returns error
	err = applyCriteriaValidated(proc, "CRIT-NONEXISTENT")
	assert.Error(t, err)

	// 2. applyCriteriaValidated on non-criteria object returns error
	sp := proc.Storage()
	sec := proc.SecurityContext()
	opCtx := proc.OperationContext()
	nonCrit := map[string]any{
		objects.FieldKeyID:            "GOAL-001",
		objects.FieldKeyKind:          objects.KindGoal,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "in_progress",
		objects.FieldKeyTitle:         "Sample Goal",
	}
	require.NoError(t, sp.Create(opCtx, sec, nonCrit))
	err = applyCriteriaValidated(proc, "GOAL-001")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected kind criteria")

	// 3. applyCriteriaValidated on already validated criteria returns nil without update
	critVal := map[string]any{
		objects.FieldKeyID:            "CRIT-VAL-001",
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "validated",
		objects.FieldKeyTitle:         "Validated Criteria",
	}
	require.NoError(t, sp.Create(opCtx, sec, critVal))
	err = applyCriteriaValidated(proc, "CRIT-VAL-001")
	assert.NoError(t, err)

	// 4. applyCriteriaValidated on planned criteria updates status to validated
	critPlan := map[string]any{
		objects.FieldKeyID:            "CRIT-PLAN-001",
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "awaiting_verification",
		objects.FieldKeyTitle:         "Planned Criteria",
		objects.FieldKeyDescription:   "Comprehensive criteria description for verification tests",
		objects.FieldKeyCategory:      "functional",
	}
	require.NoError(t, sp.Create(opCtx, sec, critPlan))
	err = applyCriteriaValidated(proc, "CRIT-PLAN-001")
	assert.NoError(t, err)
	updated, rerr := sp.Read(opCtx, sec, "CRIT-PLAN-001")
	require.NoError(t, rerr)
	assert.NotEmpty(t, updated[objects.FieldKeyStatus])
}

func TestShowActivity_BufferFormatters(t *testing.T) {
	refTime := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

	// 1. convertCacheEntryToAuditEvents
	entry := &schedpkg.ActivityCacheEntry{
		LastStarted:   refTime.Add(-10 * time.Minute),
		LastCompleted: refTime.Add(-5 * time.Minute),
		LastFailed:    refTime.Add(-2 * time.Minute),
		LastDuration:  12.5,
		LastError:     "timeout exceeded",
	}
	events := convertCacheEntryToAuditEvents("SCH-TEST-001", entry)
	assert.Len(t, events, 3)

	// 2. outputStuckJobsTableToBuffer
	var stuckBuf strings.Builder
	stuckJobs := []jobActivityEvent{
		{
			JobID:               "SCH-STUCK-001",
			CreatedAt:           refTime.Add(-30 * time.Minute).Format(time.RFC3339),
			StuckAssessmentCode: "long_running",
		},
	}
	outputStuckJobsTableToBuffer(&stuckBuf, stuckJobs, refTime, 5*time.Minute, 20*time.Minute)
	assert.Contains(t, stuckBuf.String(), "SCH-STUCK-001")
	assert.Contains(t, stuckBuf.String(), "long_running")

	// 3. outputRecentActivityTableToBuffer
	var recentBuf strings.Builder
	actEvents := []jobActivityEvent{
		{
			JobID:     "SCH-JOB-001",
			EventType: schedulerJobCompletedEvent,
			Outcome:   "success",
			Duration:  "5s",
			CreatedAt: refTime.Add(-1 * time.Minute).Format(time.RFC3339),
		},
	}
	outputRecentActivityTableToBuffer(&recentBuf, actEvents, refTime)
	assert.Contains(t, recentBuf.String(), "SCH-JOB-001")
	assert.Contains(t, recentBuf.String(), "Success")

	// 4. Empty events buffer output
	var emptyBuf strings.Builder
	outputRecentActivityTableToBuffer(&emptyBuf, nil, refTime)
	assert.Contains(t, emptyBuf.String(), "No recent job activity found")

	// 5. formatLastRunTime
	assert.Equal(t, "Never", formatLastRunTime("JOB-1", nil, nil))
	assert.NotEmpty(t, formatLastRunTime("JOB-1", nil, map[string]string{"JOB-1": refTime.Add(-2 * time.Minute).Format(time.RFC3339)}))

	// 6. coerceFloat64
	val, ok := coerceFloat64(float64(12.5))
	assert.True(t, ok)
	assert.Equal(t, 12.5, val)
	val, ok = coerceFloat64(int(42))
	assert.True(t, ok)
	assert.Equal(t, 42.0, val)
	_, ok = coerceFloat64("invalid")
	assert.False(t, ok)
}

func TestConvergenceNest_LinkExecution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// Create parent and child convergence sessions in storage
	cmd := &cobra.Command{Use: "nest-link"}
	cmd.Flags().String("parent-session-id", "CVS-PARENT-001", "")
	cmd.Flags().String("child-session-id", "CVS-CHILD-001", "")
	cmd.Flags().Int("max-depth", 5, "")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	cmdCtx := pkgctx.WithCommandOutputWriter(pkgctx.WithSecurityContext(pkgctx.NewSystemContext(), pkgctx.NewSystemSecurityContext()), &out)
	cmd.SetContext(cmdCtx)
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)
	sp := proc.Storage()
	sec := proc.SecurityContext()
	opCtx := proc.OperationContext()

	parent := map[string]any{
		objects.FieldKeyID:            "CVS-PARENT-001",
		objects.FieldKeyKind:          objects.KindConvergenceSession,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Parent Session",
		objects.FieldKeyDescription:   "Parent convergence session test description",
		"current_phase":               "c1_scope",
		"outcome_character":           "expected",
	}
	child := map[string]any{
		objects.FieldKeyID:            "CVS-CHILD-001",
		objects.FieldKeyKind:          objects.KindConvergenceSession,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Child Session",
		objects.FieldKeyDescription:   "Child convergence session test description",
		"current_phase":               "c1_scope",
		"outcome_character":           "expected",
	}
	require.NoError(t, sp.Create(opCtx, sec, parent))
	require.NoError(t, sp.Create(opCtx, sec, child))

	// Link execution
	cmd.RunE = RunConvergenceNestLink
	cmd.SetArgs([]string{"--parent-session-id", "CVS-PARENT-001", "--child-session-id", "CVS-CHILD-001"})
	err = cmd.Execute()
	require.NoError(t, err)
	updatedParent, rerr := sp.Read(opCtx, sec, "CVS-PARENT-001")
	require.NoError(t, rerr)
	assert.Contains(t, fmt.Sprint(updatedParent[objects.FieldKeyRelatedObjectRefs]), "CVS-CHILD-001")
}

func TestSchedulerSubmitCmd_ReuseAndEdgeCases(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := NewSubmitCmd()
	out1, err1 := executeSchedulerCmd(t, cmd, []string{"echo", "first", "--title", "First Submit"}, proj.Root)
	require.NoError(t, err1)
	assert.Contains(t, out1, "Job submitted:")

	cmd2 := NewSubmitCmd()
	out2, err2 := executeSchedulerCmd(t, cmd2, []string{"echo", "second", "--retry", "2", "--max-runtime", "300"}, proj.Root)
	require.NoError(t, err2)
	assert.Contains(t, out2, "Job submitted:")
}

func TestTestFailures_RerunAndList_WithSampleLogs(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// Create a sample callback file in .zqk/callbacks/sample.jsonl
	cbDir := filepath.Join(proj.Root, paths.ProjectDataDir, "callbacks")
	require.NoError(t, fileutil.EnsureDir(cbDir))
	entry := map[string]any{
		objects.FieldKeyEventType: schedulerStateFailed,
		"timestamp":               time.Now().Format(time.RFC3339),
		"test_failures": []any{
			"github.com/zqk-os/zqk/pkg/storage.TestStorageFailure",
		},
	}
	data, _ := json.Marshal(entry)
	require.NoError(t, fileutil.WriteSecureFile(filepath.Join(cbDir, "sample.jsonl"), append(data, '\n')))

	// Run list test failures
	outList, err := executeSchedulerCmd(t, NewTestFailuresListCmd(), []string{}, proj.Root)
	require.NoError(t, err)
	assert.Contains(t, outList, "pkg/storage.TestStorageFailure")

	// Run rerun test failures
	cmdRerun := NewTestFailuresRerunCmd()
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmdRerun, cliCtx)
	err = rerunTestFailures(cliCtx, cmdRerun)
	require.NoError(t, err)
}

func TestConvergenceAgentPrompt_FullMarkdown(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := &cobra.Command{Use: "prompt"}
	cmdCtx := pkgctx.WithSecurityContext(pkgctx.NewSystemContext(), pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cmdCtx)
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)
	sp := proc.Storage()
	sec := proc.SecurityContext()
	opCtx := proc.OperationContext()

	// Create convergence session in storage
	sess := map[string]any{
		objects.FieldKeyID:              "CVS-MD-001",
		objects.FieldKeyKind:            objects.KindConvergenceSession,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyTitle:           "Markdown Test Session",
		objects.FieldKeyDescription:     "Description for markdown prompt generation",
		"current_phase":                 "c1_scope",
		"outcome_character":             "expected",
		objects.FieldKeyBacklogItemRefs: []any{},
	}
	require.NoError(t, sp.Create(opCtx, sec, sess))

	snap := &schedpkg.TestBundleConvergenceSnapshot{
		ReadyForSessionCompletion: true,
		DeltaAssessment:           "healthy",
		PrimaryMeasurementOutcome: "success",
	}

	md, err := buildAgentConvergenceMarkdown(cmd, snap, "CVS-MD-001", "c1_scope", "default", map[string]any{}, map[string]any{}, false, map[string]any{}, false, "", "auto", nil, map[string]any{}, map[string]any{})
	require.NoError(t, err)
	assert.NotEmpty(t, md)
	assert.Contains(t, md, "CVS-MD-001")
}

func TestSchedulerCore_ShutdownAndDiagnostics(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. buildConfigAtShutdownFields
	fields := buildConfigAtShutdownFields(proj.Root)
	assert.NotNil(t, fields)

	// 2. captureDiagnostics in temp dir
	diagDir := filepath.Join(proj.Root, "diagnostics_test")
	err := captureDiagnostics(diagDir, "test_prefix")
	assert.NoError(t, err)
}

func TestCEFDiamond_FullMeasure(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	qualityDir := filepath.Join(proj.Root, "docs", "quality")
	require.NoError(t, fileutil.EnsureDir(qualityDir))

	reg := `default: cef_diamond_scorecard
matrices:
  cef_diamond_scorecard:
    csv: docs/quality/CEF_DIAMOND_SCORECARD_MATRIX.csv
    profile: docs/quality/cef_diamond_scorecard_matrix_profile.yaml
    session_ref_column: cvs_id
`
	csv := `run_id,seat_role,agent_run_id,seat_agent_id,freeze_sha,assessed_at,RDB,MNT,TST,REL,OBS,RCV,SEC,ROB,envelope_min,package_complete,output_home,scorecard_path,cvs_id,bli_ref,notes
ENV,envelope,min,,sha1,2026-08-31,3,3,3,3,3,3,3,3,3,yes,,,CVS-CEF-FULL,,
`
	prof := `schema_version: 1
fieldnames: [run_id]
completion:
  gate_columns: [package_complete]
  done_values: [yes, na]
`
	require.NoError(t, fileutil.WriteSecureFile(filepath.Join(qualityDir, "matrix_registry.yaml"), []byte(reg)))
	require.NoError(t, fileutil.WriteSecureFile(filepath.Join(qualityDir, "CEF_DIAMOND_SCORECARD_MATRIX.csv"), []byte(csv)))
	require.NoError(t, fileutil.WriteSecureFile(filepath.Join(qualityDir, "cef_diamond_scorecard_matrix_profile.yaml"), []byte(prof)))

	// Create convergence session in storage
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cmd := &cobra.Command{Use: "cef"}
	cmd.Flags().StringP("format", "f", "json", "")
	cmdCtx := pkgctx.WithSecurityContext(pkgctx.NewSystemContext(), pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cmdCtx)
	cli.SetContext(cmd, cliCtx)

	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)
	sp := proc.Storage()
	sec := proc.SecurityContext()
	opCtx := proc.OperationContext()

	sess := map[string]any{
		objects.FieldKeyID:            "CVS-CEF-FULL",
		objects.FieldKeyKind:          objects.KindConvergenceSession,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "CEF Diamond Test Session",
		objects.FieldKeyDescription:   "Description for CEF diamond test session",
		"current_phase":               "c1_scope",
		"outcome_character":           "expected",
	}
	require.NoError(t, sp.Create(opCtx, sec, sess))

	// Test json format with persistSession=true
	err = runCEFDiamondConvergenceMeasure(cliCtx, cmd, proj.Root, "CVS-CEF-FULL", true, "c1_scope", "default", map[string]any{}, map[string]any{})
	assert.NoError(t, err)

	// Test yaml format
	_ = cmd.Flags().Set("format", "yaml")
	err = runCEFDiamondConvergenceMeasure(cliCtx, cmd, proj.Root, "CVS-CEF-FULL", false, "c1_scope", "default", map[string]any{}, map[string]any{})
	assert.NoError(t, err)

	// Test agent-prompt format
	_ = cmd.Flags().Set("format", "agent-prompt")
	err = runCEFDiamondConvergenceMeasure(cliCtx, cmd, proj.Root, "CVS-CEF-FULL", false, "c1_scope", "default", map[string]any{}, map[string]any{})
	assert.NoError(t, err)

	// Test table format
	_ = cmd.Flags().Set("format", "table")
	err = runCEFDiamondConvergenceMeasure(cliCtx, cmd, proj.Root, "CVS-CEF-FULL", false, "c1_scope", "default", map[string]any{}, map[string]any{})
	assert.NoError(t, err)
}

func TestConvergenceRollup_ChildSessionsAndTree(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := &cobra.Command{Use: "rollup"}
	cmdCtx := pkgctx.WithSecurityContext(pkgctx.NewSystemContext(), pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cmdCtx)
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)
	sp := proc.Storage()
	sec := proc.SecurityContext()
	opCtx := proc.OperationContext()

	// Create parent and child
	parent := map[string]any{
		objects.FieldKeyID:                "CVS-ROLL-PARENT",
		objects.FieldKeyKind:              objects.KindConvergenceSession,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:            objects.ObjectStatusActive,
		objects.FieldKeyTitle:             "Rollup Parent",
		objects.FieldKeyDescription:       "Rollup parent test description",
		"current_phase":                   "c1_scope",
		"outcome_character":               "expected",
		objects.FieldKeyRelatedObjectRefs: []any{"CVS-ROLL-CHILD"},
	}
	child := map[string]any{
		objects.FieldKeyID:            "CVS-ROLL-CHILD",
		objects.FieldKeyKind:          objects.KindConvergenceSession,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusCompleted,
		objects.FieldKeyTitle:         "Rollup Child",
		objects.FieldKeyDescription:   "Rollup child test description",
		"current_phase":               "c1_scope",
		"outcome_character":           "expected",
	}
	require.NoError(t, sp.Create(opCtx, sec, child))
	require.NoError(t, sp.Create(opCtx, sec, parent))

	// 1. loadRollupChildSessions
	children, err := loadRollupChildSessions(cmd, "CVS-ROLL-PARENT", 10)
	require.NoError(t, err)
	assert.Len(t, children, 1)
	assert.Equal(t, "CVS-ROLL-CHILD", children[0].ID)

	// 2. collectCVSTreeForOverseer
	tree, err := collectCVSTreeForOverseer(cmd, "CVS-ROLL-PARENT", 5)
	require.NoError(t, err)
	assert.NotEmpty(t, tree)
}

func TestConvergenceOverseer_FullOutput(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := &cobra.Command{Use: "overseer"}
	cmd.Flags().String("coordinator-session-id", "CVS-PARENT-001", "")
	cmd.Flags().Int("limit", 100, "")
	cmd.Flags().Bool("skip-rollup-gates", true, "")
	cmd.Flags().StringP("format", "f", "json", "")

	cmdCtx := pkgctx.WithSecurityContext(pkgctx.NewSystemContext(), pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cmdCtx)
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)
	sp := proc.Storage()
	sec := proc.SecurityContext()
	opCtx := proc.OperationContext()

	sess := map[string]any{
		objects.FieldKeyID:            "CVS-PARENT-001",
		objects.FieldKeyKind:          objects.KindConvergenceSession,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Overseer Parent",
		objects.FieldKeyDescription:   "Overseer parent session test description",
		"current_phase":               "c1_scope",
		"outcome_character":           "expected",
	}
	require.NoError(t, sp.Create(opCtx, sec, sess))

	// Run JSON format
	err = runTestFailuresConvergenceOverseer(cliCtx, cmd)
	require.NoError(t, err)

	// Run default table format
	_ = cmd.Flags().Set("format", "table")
	err = runTestFailuresConvergenceOverseer(cliCtx, cmd)
	require.NoError(t, err)
}

func TestConvergenceNest_SpawnAndStatus(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := &cobra.Command{Use: "nest"}
	cmd.Flags().String("parent-session-id", "CVS-NEST-PARENT", "")
	cmd.Flags().String("title", "Spawned Child", "")
	cmd.Flags().String("hypothesis", "Fix failing tests", "")
	cmd.Flags().String("desired-end-state", "All tests green", "")
	cmd.Flags().String("next-action", "Run make test", "")
	cmd.Flags().String("current-phase", "c1_scope", "")
	cmd.Flags().Int("max-depth", 5, "")

	cmdCtx := pkgctx.WithSecurityContext(pkgctx.NewSystemContext(), pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cmdCtx)
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)
	sp := proc.Storage()
	sec := proc.SecurityContext()
	opCtx := proc.OperationContext()

	parent := map[string]any{
		objects.FieldKeyID:            "CVS-NEST-PARENT",
		objects.FieldKeyKind:          objects.KindConvergenceSession,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Nest Parent",
		objects.FieldKeyDescription:   "Nest parent session test description",
		"current_phase":               "c1_scope",
		"outcome_character":           "expected",
	}
	require.NoError(t, sp.Create(opCtx, sec, parent))

	// 1. RunConvergenceNestSpawn
	err = RunConvergenceNestSpawn(cmd, []string{})
	require.NoError(t, err)

	// 2. RunConvergenceNestStatus
	cmdStatus := &cobra.Command{Use: "nest-status"}
	cmdStatus.Flags().String("parent-session-id", "CVS-NEST-PARENT", "")
	cmdStatus.Flags().Int("max-depth", 5, "")
	cmdStatus.SetContext(cmdCtx)
	cli.SetContext(cmdStatus, cliCtx)

	err = RunConvergenceNestStatus(cmdStatus, []string{})
	require.NoError(t, err)
}

func TestSchedulerCore_UtilityFunctions(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. buildAutonomyCorsHandler
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	corsHandler := buildAutonomyCorsHandler(dummyHandler)

	// OPTIONS request with valid Origin
	optReq := httptest.NewRequest(http.MethodOptions, "/api/test", nil)
	optReq.Header.Set("Origin", "http://127.0.0.1:5173")
	optRec := httptest.NewRecorder()
	corsHandler.ServeHTTP(optRec, optReq)
	assert.Equal(t, http.StatusNoContent, optRec.Code)
	assert.Equal(t, "http://127.0.0.1:5173", optRec.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, optRec.Header().Get("Access-Control-Allow-Methods"), "OPTIONS")

	// GET request with valid Origin
	getReq := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	getReq.Header.Set("Origin", "http://localhost:5173")
	getRec := httptest.NewRecorder()
	corsHandler.ServeHTTP(getRec, getReq)
	assert.Equal(t, http.StatusOK, getRec.Code)
	assert.Equal(t, "http://localhost:5173", getRec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "ok", getRec.Body.String())

	// 2. runSchedulerControlWithTimeoutDur
	err := runSchedulerControlWithTimeoutDur("op-success", time.Second, func() error {
		return nil
	})
	assert.NoError(t, err)

	err = runSchedulerControlWithTimeoutDur("op-err", time.Second, func() error {
		return errors.New("underlying failure")
	})
	assert.ErrorContains(t, err, "underlying failure")

	err = runSchedulerControlWithTimeoutDur("op-timeout", 20*time.Millisecond, func() error {
		time.Sleep(200 * time.Millisecond)
		return nil
	})
	assert.ErrorContains(t, err, "timed out after")

	// 3. resolveSchedulerCLIProjectRoot
	customCtx := cli.ContextForProjectRoot("/custom/path")
	assert.Equal(t, "/custom/path", resolveSchedulerCLIProjectRoot(customCtx))
	resolvedRoot := resolveSchedulerCLIProjectRoot(nil)
	assert.NotEmpty(t, resolvedRoot)

	// 4. validateSchedulerProjectRootAndBrand
	_, err = validateSchedulerProjectRootAndBrand(nil, "")
	assert.ErrorContains(t, err, schedulerErrProjectRootNotFound)

	prof, err := validateSchedulerProjectRootAndBrand(nil, proj.Root)
	assert.NoError(t, err)
	assert.Equal(t, schedulerProfileSystem, prof)

	profCustom, err := validateSchedulerProjectRootAndBrand(cli.ContextForProjectAndProfile(proj.Root, "custom"), proj.Root)
	assert.NoError(t, err)
	assert.Equal(t, "custom", profCustom)
}

func TestStartScheduler_ValidationAndDisabled(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := NewStartCmd()
	cmd.SetContext(context.Background())

	// 1. rejectHostServiceOwnedBackgroundStart
	err := rejectHostServiceOwnedBackgroundStart(proj.Root)
	assert.NoError(t, err)

	// 2. startSchedulerInBackground with non-existent root
	nonExistentRoot := filepath.Join(proj.Root, "nonexistent", "deep", "dir")
	err = startSchedulerInBackground(cli.ContextForProjectRoot(nonExistentRoot), cmd)
	assert.Error(t, err)
}

func TestStartSchedulerBackgroundWatchers_SignalHandling(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := &cobra.Command{Use: "scheduler"}
	cmd.SetContext(context.Background())
	startCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	interruptCtx, intCancel := context.WithCancel(context.Background())

	// Create storage provider for watchers
	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), proj.Root)
	require.NoError(t, err)
	sp := storageFactory.GetStorage()

	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()
	sched := schedpkg.NewSchedulerWithProjectRoot(sp, specLoader, lifecycleLoader, proj.Root, nil)

	startSchedulerBackgroundWatchers(cmd, startCtx, cancel, interruptCtx, proj.Root, sp, "system", sched)

	// Trigger interrupt context cancellation
	intCancel()

	// Wait briefly for watcher goroutine to process cancellation
	assert.Eventually(t, func() bool {
		return startCtx.Err() != nil
	}, 2*time.Second, 20*time.Millisecond)
}

func TestSchedulerStop_InProcessAndIdempotent(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cctx := cli.ContextForProjectRoot(proj.Root)
	cmd := NewStopCmd()
	cli.SetContext(cmd, cctx)

	// 1. When not running, stopScheduler should be idempotent
	err := stopScheduler(cctx, cmd)
	assert.NoError(t, err)

	// 2. When in-process global scheduler is registered
	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), proj.Root)
	require.NoError(t, err)
	sp := storageFactory.GetStorage()

	sched := schedpkg.NewSchedulerWithProjectRoot(sp, objects.GetGlobalSpecLoader(), objects.GetGlobalLifecycleLoader(), proj.Root, nil)
	if s, ok := sched.(*schedpkg.Scheduler); ok {
		schedpkg.RegisterGlobalScheduler(s)
		defer schedpkg.RegisterGlobalScheduler(nil)
	}

	err = stopScheduler(cctx, cmd)
	assert.NoError(t, err)
}

func TestTestFailuresAnalyzePipeline_WithPersistentFailures(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// Create sample callback files with repeated test failures
	cbDir := filepath.Join(proj.Root, paths.ProjectDataDir, "callbacks")
	require.NoError(t, fileutil.EnsureDir(cbDir))

	entry := map[string]any{
		objects.FieldKeyEventType: schedulerStateFailed,
		"timestamp":               time.Now().Format(time.RFC3339),
		"test_failures": []any{
			"github.com/zqk-os/zqk/pkg/pipeline.TestPipelineFailure",
		},
	}
	data, _ := json.Marshal(entry)
	require.NoError(t, fileutil.WriteSecureFile(filepath.Join(cbDir, "failures.jsonl"), append(data, '\n')))

	cmd := NewTestFailuresAnalyzeCmd()
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	_ = cmd.Flags().Set("min-failures", "1")
	_ = cmd.Flags().Set("create-backlog-items", "true")

	err := analyzeTestFailuresViaPipeline(cliCtx, cmd)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "persistent test failure")
}

func TestOutputHelpers_QueryBaseAndHistory(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// 1. initSchedulerQueryBaseContext
	baseCtx, err := initSchedulerQueryBaseContext(cliCtx, 5*time.Second)
	require.NoError(t, err)
	assert.NotNil(t, baseCtx)

	_, err = initSchedulerQueryBaseContext(nil, 5*time.Second)
	assert.ErrorContains(t, err, "failed to get CLI context")

	// 2. OutputJobHistoryData (Table and JSON)
	stats := []*jobStats{
		{
			JobID:        "SCH-TEST-001",
			TotalRuns:    3,
			SuccessCount: 3,
			FailCount:    0,
		},
	}

	cmdTable := &cobra.Command{Use: "history"}
	var bufTable bytes.Buffer
	cmdTable.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufTable))
	cli.SetContext(cmdTable, cliCtx)

	err = OutputJobHistoryData(context.Background(), cmdTable, proj.Root, baseCtx.StorageProvider, stats, "", "system")
	require.NoError(t, err)

	cmdJSON := &cobra.Command{Use: "history"}
	var bufJSON bytes.Buffer
	cmdJSON.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufJSON))
	cmdJSON.Flags().String("format", "json", "")
	_ = cmdJSON.Flags().Set("format", "json")
	cli.SetContext(cmdJSON, cliCtx)

	err = OutputJobHistoryData(context.Background(), cmdJSON, proj.Root, baseCtx.StorageProvider, stats, "", "system")
	require.NoError(t, err)
	assert.Contains(t, bufJSON.String(), "SCH-TEST-001")

	// 3. emitSchedulerOutputErrorViaCoordinator
	emitSchedulerOutputErrorViaCoordinator(context.Background(), proj.Root, baseCtx.StorageProvider, "json", errors.New("formatting error"), "system")
}

func TestRunTestFailuresConvergence_FormatsAndTombstone(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// Create health.jsonl with sample line
	bundlesDir := schedpkg.JobLogsTestBundlesDir(proj.Root)
	require.NoError(t, fileutil.EnsureDir(bundlesDir))
	healthLine := map[string]any{
		"event_type":                   "test_bundle_run",
		"timestamp":                    time.Now().Format(time.RFC3339),
		"ready_for_session_completion": true,
		"failing_fingerprints":         []string{},
		"delta_assessment":             "clean",
		"primary_measurement_outcome":  "passed",
	}
	hb, _ := json.Marshal(healthLine)
	require.NoError(t, fileutil.WriteSecureFile(filepath.Join(bundlesDir, "health.jsonl"), append(hb, '\n')))

	cliCtx := cli.ContextForProjectRoot(proj.Root)

	newTestConvergenceCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "measure"}
		cmd.Flags().Int("limit", 500, "")
		cmd.Flags().String("session-id", "", "")
		cmd.Flags().String("current-phase", "", "")
		cmd.Flags().String("flow-variant", "", "")
		cmd.Flags().Bool("persist-session", false, "")
		cmd.Flags().Bool("skip-session-context", false, "")
		cmd.Flags().Bool("stamp-tombstone", false, "")
		cmd.Flags().Bool("finalize-debrief", false, "")
		cmd.Flags().String("debrief-notes", "", "")
		cmd.Flags().String("debrief-notes-file", "", "")
		cmd.Flags().Bool("skip-rollup-gates", false, "")
		cmd.Flags().Bool("paste-ide", false, "")
		cmd.Flags().Bool("copy", false, "")
		cmd.Flags().String("attention-mode", "", "")
		cmd.Flags().String("format", "table", "")
		return cmd
	}

	// Table format
	cmdTable := newTestConvergenceCmd()
	var bufTable bytes.Buffer
	cmdTable.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufTable))
	cli.SetContext(cmdTable, cliCtx)
	_ = cmdTable.Flags().Set("skip-rollup-gates", "true")
	err := runTestFailuresConvergence(cliCtx, cmdTable)
	require.NoError(t, err)
	assert.Contains(t, bufTable.String(), "Test-bundle convergence measure")

	// JSON format
	cmdJSON := newTestConvergenceCmd()
	var bufJSON bytes.Buffer
	cmdJSON.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufJSON))
	cli.SetContext(cmdJSON, cliCtx)
	_ = cmdJSON.Flags().Set("format", "json")
	_ = cmdJSON.Flags().Set("skip-rollup-gates", "true")
	err = runTestFailuresConvergence(cliCtx, cmdJSON)
	require.NoError(t, err)
	assert.Contains(t, bufJSON.String(), "ready_for_session_completion")

	// YAML format
	cmdYAML := newTestConvergenceCmd()
	var bufYAML bytes.Buffer
	cmdYAML.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufYAML))
	cli.SetContext(cmdYAML, cliCtx)
	_ = cmdYAML.Flags().Set("format", "yaml")
	_ = cmdYAML.Flags().Set("skip-rollup-gates", "true")
	err = runTestFailuresConvergence(cliCtx, cmdYAML)
	require.NoError(t, err)
	assert.Contains(t, bufYAML.String(), "ready_for_session_completion")
}

func TestBuildRollupStatusCore_Variations(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := &cobra.Command{Use: "rollup"}
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	snap := &schedpkg.TestBundleConvergenceSnapshot{
		ReadyForSessionCompletion: true,
		DeltaAssessment:           "healthy",
		FailingFingerprintsNow:    []string{},
	}

	// 1. With skipRollupGates=true
	rollup1, err := buildRollupStatusCore(cmd, proj.Root, true, "", snap, true)
	require.NoError(t, err)
	assert.Equal(t, true, rollup1["ready_for_parent_completion"])

	// 2. With skipRollupGates=false
	rollup2, err := buildRollupStatusCore(cmd, proj.Root, true, "", snap, false)
	require.NoError(t, err)
	assert.NotNil(t, rollup2)

	// 3. runHardcodedGoLiteralsScan
	scan := runHardcodedGoLiteralsScan(proj.Root)
	assert.Equal(t, "passed", scan["status"])

	// 4. gateExitForDisplay
	assert.Nil(t, gateExitForDisplay(0, true))
	assert.Equal(t, 0, gateExitForDisplay(0, false))
}

func TestTestFailuresList_EmptyAndFilled(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// 1. No logs found
	cmdEmpty := NewTestFailuresListCmd()
	var bufEmpty bytes.Buffer
	cmdEmpty.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufEmpty))
	cli.SetContext(cmdEmpty, cliCtx)
	err := listTestFailures(cliCtx, cmdEmpty)
	require.NoError(t, err)
	assert.Contains(t, bufEmpty.String(), "No test logs found")

	// 2. With scheduler job log files
	schedLogDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir, "SCH-TEST-001")
	require.NoError(t, fileutil.EnsureDir(schedLogDir))
	logContent := "--- FAIL: TestExampleFailure (0.01s)\nFAIL github.com/zqk-os/zqk/pkg/example 0.05s\n"
	require.NoError(t, fileutil.WriteSecureFile(filepath.Join(schedLogDir, "SCH-TEST-001.log"), []byte(logContent)))

	cmdWithLogs := NewTestFailuresListCmd()
	var bufWithLogs bytes.Buffer
	cmdWithLogs.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufWithLogs))
	cli.SetContext(cmdWithLogs, cliCtx)
	_ = cmdWithLogs.Flags().Set("limit", "10")
	err = listTestFailures(cliCtx, cmdWithLogs)
	require.NoError(t, err)
}

func TestShowActivityHelpers_TableAndSummaries(t *testing.T) {
	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)

	// 1. parseActivityEventTime
	tm, ok := parseActivityEventTime(now.Format(time.RFC3339))
	assert.True(t, ok)
	assert.False(t, tm.IsZero())

	_, ok = parseActivityEventTime("invalid-time-format")
	assert.False(t, ok)

	// 2. formatRelativeAge
	assert.Equal(t, "—", formatRelativeAge(time.Time{}, now))
	assert.Equal(t, "just now", formatRelativeAge(now, now))
	assert.Contains(t, formatRelativeAge(yesterday, now), "ago")

	// 3. activityEventTimeBounds
	events := []jobActivityEvent{
		{JobID: "SCH-001", CreatedAt: yesterday.Format(time.RFC3339), EventType: schedulerStateStarted},
		{JobID: "SCH-001", CreatedAt: now.Format(time.RFC3339), EventType: schedulerStateCompleted},
		{JobID: "SCH-002", CreatedAt: "bad-time", EventType: schedulerStateFailed},
	}
	oldest, newest, ok := activityEventTimeBounds(events)
	assert.True(t, ok)
	assert.True(t, oldest.Before(newest) || oldest.Equal(newest))

	_, _, ok = activityEventTimeBounds([]jobActivityEvent{})
	assert.False(t, ok)

	// 4. buildJobActivitySummaries and outputRecentJobSummaryTableToBuffer
	rows := buildJobActivitySummaries(events, 10, now)
	assert.NotEmpty(t, rows)

	var buf strings.Builder
	outputRecentJobSummaryTableToBuffer(&buf, events, now, 10)
	assert.Contains(t, buf.String(), "Recent jobs")

	var legendBuf strings.Builder
	outputMissedTriggersLegendToBuffer(&legendBuf)
	assert.Contains(t, legendBuf.String(), "cron ticks")
}

func TestShowHistoryHelpers_TimeShortcutsAndAggregation(t *testing.T) {
	// 1. parseTimeString
	shortcuts := []string{"today", "yesterday", "this-week", "last-week", "this-month", "last-month", "this-year", "now"}
	for _, sc := range shortcuts {
		parsed := parseTimeString(sc, true)
		assert.NotNil(t, parsed, "expected non-nil for shortcut: "+sc)
	}
	assert.Nil(t, parseTimeString("not-a-shortcut", true))

	// 2. aggregateJobStats
	jhc := &JobHistoryContext{JobIDFilter: ""}
	auditEvents := []map[string]any{
		{
			objects.FieldKeyEventType:  "scheduler_job_started",
			objects.FieldKeyTargetKind: objects.KindSchedulerJob,
			objects.FieldKeyTargetID:   "SCH-001",
			objects.FieldKeyCreatedAt:  time.Now().Format(time.RFC3339),
		},
		{
			objects.FieldKeyEventType:  "scheduler_job_completed",
			objects.FieldKeyTargetKind: objects.KindSchedulerJob,
			objects.FieldKeyTargetID:   "SCH-001",
			objects.FieldKeyCreatedAt:  time.Now().Format(time.RFC3339),
			"duration_ms":              120.0,
		},
		{
			objects.FieldKeyEventType:  "scheduler_job_failed",
			objects.FieldKeyTargetKind: objects.KindSchedulerJob,
			objects.FieldKeyTargetID:   "SCH-002",
			objects.FieldKeyCreatedAt:  time.Now().Format(time.RFC3339),
			"duration_ms":              500.0,
		},
	}
	statsMap := aggregateJobStats(jhc, auditEvents)
	assert.Contains(t, statsMap, "SCH-001")
	assert.Contains(t, statsMap, "SCH-002")
	if s1, ok := statsMap["SCH-001"]; ok {
		assert.Equal(t, 1, s1.SuccessCount)
	}
	if s2, ok := statsMap["SCH-002"]; ok {
		assert.Equal(t, 1, s2.FailCount)
	}

	// 3. sortJobStatsList
	statsList := []*jobStats{
		{JobID: "SCH-002", TotalRuns: 5},
		{JobID: "SCH-001", TotalRuns: 10},
	}
	sortJobStatsList(statsList)
	assert.Equal(t, "SCH-001", statsList[0].JobID)
}

func TestEventsHealth_Detailed(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. Without summary file
	cmd := &cobra.Command{Use: "health"}
	var bufEmpty bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufEmpty))
	cli.SetContext(cmd, cli.ContextForProjectRoot(proj.Root))
	err := runEventsHealth(cmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, bufEmpty.String(), "No summary found at")

	// 2. With summary file containing slow and failed jobs
	summary := schedpkg.SchedulerMetricsSummary{
		WindowEndISO: time.Now().Format(time.RFC3339),
		JobStats: map[string]schedpkg.JobExecutionStats{
			"SCH-SLOW-001": {
				Completed:        2,
				Failed:           1,
				TotalDurationSec: 1000.0,
				LastFailedISO:    time.Now().Format(time.RFC3339),
			},
			"SCH-FAST-002": {
				Completed:        5,
				Failed:           0,
				TotalDurationSec: 5.0,
			},
		},
	}
	sumData, err := json.Marshal(summary)
	require.NoError(t, err)

	sumPath := schedpkg.SummaryPath(proj.Root)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(sumPath)))
	require.NoError(t, fileutil.WriteSecureFile(sumPath, sumData))

	cmdWithSum := &cobra.Command{Use: "health"}
	var bufSum bytes.Buffer
	cmdWithSum.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufSum))
	cli.SetContext(cmdWithSum, cli.ContextForProjectRoot(proj.Root))
	err = runEventsHealth(cmdWithSum, []string{})
	require.NoError(t, err)
	assert.Contains(t, bufSum.String(), "SCH-SLOW-001")
	assert.Contains(t, bufSum.String(), "has_failures,slow")
}

func TestCriteriaEvidence_ApplyAndValidation(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// 1. When test-bundle events file does not exist
	cmdNoEvents := NewTestFailuresCriteriaEvidenceCmd()
	var bufNoEvents bytes.Buffer
	cmdNoEvents.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufNoEvents))
	cli.SetContext(cmdNoEvents, cliCtx)
	err := runCriteriaEvidence(cliCtx, cmdNoEvents)
	require.NoError(t, err)
	assert.Contains(t, bufNoEvents.String(), "No test-bundle events file yet")

	// 2. Create test-bundle events file with evidence rows
	eventsFile := schedpkg.TestBundlesEventsFilePath(proj.Root)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(eventsFile)))

	row := map[string]any{
		objects.FieldKeyEventType:                 schedpkg.KeyEventTypeCriteriaVerificationEvidence,
		objects.FieldKeyCriteriaRefs:              []any{"CRIT-EXP-001"},
		schedpkg.KeyCriteriaVerificationSatisfied: true,
		schedpkg.KeyJobID:                         "SCH-TEST-BUNDLE",
		schedpkg.KeyBundleCommandFingerprint:      "fp-test-123",
		schedpkg.KeyTimestamp:                     time.Now().Format(time.RFC3339),
	}
	rowData, err := json.Marshal(row)
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteSecureFile(eventsFile, append(rowData, '\n')))

	// Create criteria object in storage
	cmdApply := NewTestFailuresCriteriaEvidenceCmd()
	cmdApply.SetContext(context.Background())
	cli.SetContext(cmdApply, cliCtx)
	proc, err := cli.NewProcessor(cmdApply)
	require.NoError(t, err)

	critObj := map[string]any{
		objects.FieldKeyID:            "CRIT-EXP-001",
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "originated",
		objects.FieldKeyCategory:      "acceptance",
		objects.FieldKeyTitle:         "Criteria Evidence Test",
		objects.FieldKeyDescription:   "Description for criteria evidence testing",
	}
	require.NoError(t, proc.Storage().Create(proc.OperationContext(), proc.SecurityContext(), critObj))

	// 3. Dry-run apply
	var bufDry bytes.Buffer
	cmdDry := NewTestFailuresCriteriaEvidenceCmd()
	cmdDry.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufDry))
	cli.SetContext(cmdDry, cliCtx)
	_ = cmdDry.Flags().Set("apply", "true")
	_ = cmdDry.Flags().Set("dry-run", "true")
	err = runCriteriaEvidence(cliCtx, cmdDry)
	require.NoError(t, err)
	assert.Contains(t, bufDry.String(), "Dry-run")

	// 4. Real apply
	var bufReal bytes.Buffer
	cmdReal := NewTestFailuresCriteriaEvidenceCmd()
	cmdReal.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufReal))
	cli.SetContext(cmdReal, cliCtx)
	_ = cmdReal.Flags().Set("apply", "true")
	_ = cmdReal.Flags().Set("dry-run", "false")
	err = runCriteriaEvidence(cliCtx, cmdReal)
	require.NoError(t, err)
	assert.Contains(t, bufReal.String(), "Applied validated status to: CRIT-EXP-001")

	// Verify criteria status in storage is now validated
	updatedCrit, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), "CRIT-EXP-001")
	require.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusValidated, updatedCrit[objects.FieldKeyStatus])
}

func TestSchedulerHealthUtilities(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. daemonUnhealthyPath
	p := daemonUnhealthyPath(proj.Root)
	assert.Contains(t, p, paths.SchedulerDaemonUnhealthyFile)

	// 2. writeDaemonUnhealthyFile
	writeDaemonUnhealthyFile(proj.Root, "test_reason", 3, "Daemon test message")
	assert.True(t, fileutil.Exists(p))

	// 3. removeDaemonUnhealthyFile
	removeDaemonUnhealthyFile(proj.Root)
	assert.False(t, fileutil.Exists(p))

	// 4. recordExternalHealthMetric
	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), proj.Root)
	require.NoError(t, err)
	sp := storageFactory.GetStorage()
	secCtx := pkgctx.NewSystemSecurityContext()

	recordExternalHealthMetric(sp, secCtx, time.Now().UTC(), 1, 2, 0, 0, 50*time.Millisecond)
	// Zero values should return early
	recordExternalHealthMetric(sp, secCtx, time.Now().UTC(), 0, 0, 0, 0, 0)
}

func TestSchedulerDump_NotRunning(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := NewDumpCmd()
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	err := runDump(cliCtx, cmd)
	assert.ErrorContains(t, err, "scheduler daemon is not running")
}

func TestSetupDaemonStdioRedirect_Lifecycle(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	res := SetupDaemonStdioRedirect(proj.Root)
	require.NotNil(t, res.Cleanup)
	require.NotNil(t, res.CoordinatorWriter)

	_, err := res.CoordinatorWriter.Write([]byte("test log event\n"))
	assert.NoError(t, err)

	res.Cleanup()
}

func TestEventsHealth_WithMetricsSummary(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// Case 1: no summary file
	cmd := NewEventsCmd()
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	cli.SetContext(cmd, cli.ContextForProjectRoot(proj.Root))
	cmd.SetArgs([]string{"health"})
	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "No summary found")

	// Case 2: with summary file containing healthy, slow, and failed jobs
	summary := schedpkg.SchedulerMetricsSummary{
		WindowEndISO: time.Now().UTC().Format(time.RFC3339),
		JobStats: map[string]schedpkg.JobExecutionStats{
			"SCH-JOB-HEALTHY": {
				Completed:        10,
				Failed:           0,
				TotalDurationSec: 50.0,
			},
			"SCH-JOB-FAILED": {
				Completed:        5,
				Failed:           2,
				TotalDurationSec: 30.0,
				LastFailedISO:    time.Now().UTC().Format(time.RFC3339),
			},
			"SCH-JOB-SLOW": {
				Completed:        2,
				Failed:           0,
				TotalDurationSec: 1000.0,
			},
			"SCH-JOB-SLOW-AND-FAILED": {
				Completed:        2,
				Failed:           1,
				TotalDurationSec: 1200.0,
			},
			"SCH-JOB-NO-RUNS": {
				Completed: 0,
				Failed:    0,
			},
		},
	}
	data, _ := json.Marshal(summary)
	schedDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.SchedulerDir)
	require.NoError(t, fileutil.EnsureDir(schedDir))
	require.NoError(t, fileutil.WriteSecureFile(schedpkg.SummaryPath(proj.Root), data))

	buf.Reset()
	cmd = NewEventsCmd()
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	cli.SetContext(cmd, cli.ContextForProjectRoot(proj.Root))
	cmd.SetArgs([]string{"health"})
	err = cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Health view")
	assert.Contains(t, buf.String(), "SCH-JOB-FAILED")
	assert.Contains(t, buf.String(), "SCH-JOB-SLOW")
}

func TestTestFailuresHealth_DetailedOutcomeAndFingerprints(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	tbDir := filepath.Dir(schedpkg.TestBundlesHealthFilePath(proj.Root))
	require.NoError(t, fileutil.EnsureDir(tbDir))

	rows := []map[string]any{
		{
			schedpkg.KeyTestOutcome:              "passed",
			schedpkg.KeyBundleCommandFingerprint: "fp-1",
		},
		{
			schedpkg.KeyTestOutcome:              "failed",
			schedpkg.KeyBundleCommandFingerprint: "fp-2",
		},
		{
			schedpkg.KeyTestOutcome:              "passed",
			schedpkg.KeyBundleCommandFingerprint: "fp-2",
		},
		{
			schedpkg.KeyTestOutcome:              "flaky",
			schedpkg.KeyBundleCommandFingerprint: "fp-3",
		},
		{},
	}
	var bufLines bytes.Buffer
	for _, r := range rows {
		d, _ := json.Marshal(r)
		bufLines.Write(d)
		bufLines.WriteByte('\n')
	}
	require.NoError(t, fileutil.WriteSecureFile(schedpkg.TestBundlesHealthFilePath(proj.Root), bufLines.Bytes()))

	cmd := NewTestFailuresHealthCmd()
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	cli.SetContext(cmd, cli.ContextForProjectRoot(proj.Root))

	err := runTestFailuresHealth(cli.ContextForProjectRoot(proj.Root), cmd)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Outcome counts")
	assert.Contains(t, buf.String(), "passed: 2")
	assert.Contains(t, buf.String(), "failed: 1")
}

func TestCriteriaEvidence_ApplyAndDryRun(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	eventsPath := schedpkg.TestBundlesEventsFilePath(proj.Root)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(eventsPath)))

	cmd := NewTestFailuresCriteriaEvidenceCmd()
	cmdCtx := pkgctx.WithSecurityContext(pkgctx.NewSystemContext(), pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cmdCtx)
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)
	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	critObj := map[string]any{
		objects.FieldKeyID:            "CRIT-1792035477264387000-089a9069",
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusAwaitingVerification,
		objects.FieldKeyTitle:         "Sample Criteria",
		objects.FieldKeyDescription:   "Validating evidence apply logic description",
		"category":                    "functional",
	}
	require.NoError(t, proc.Storage().Create(proc.OperationContext(), proc.SecurityContext(), critObj))

	events := []map[string]any{
		{
			objects.FieldKeyEventType:                 schedpkg.KeyEventTypeCriteriaVerificationEvidence,
			objects.FieldKeyCriteriaRefs:              []any{"CRIT-1792035477264387000-089a9069"},
			schedpkg.KeyCriteriaVerificationSatisfied: true,
			schedpkg.KeyJobID:                         "SCH-TEST-001",
			schedpkg.KeyBundleCommandFingerprint:      "fp-sample",
			schedpkg.KeyTimestamp:                     time.Now().UTC().Format(time.RFC3339),
		},
		{
			objects.FieldKeyEventType:                 schedpkg.KeyEventTypeCriteriaVerificationEvidence,
			objects.FieldKeyCriteriaRefs:              []any{"CRIT-NONEXISTENT"},
			schedpkg.KeyCriteriaVerificationSatisfied: false,
			schedpkg.KeyJobID:                         "SCH-TEST-002",
			schedpkg.KeyBundleCommandFingerprint:      "fp-sample-fail",
			schedpkg.KeyTimestamp:                     time.Now().UTC().Format(time.RFC3339),
		},
	}
	var bufEvents bytes.Buffer
	for _, ev := range events {
		d, _ := json.Marshal(ev)
		bufEvents.Write(d)
		bufEvents.WriteByte('\n')
	}
	require.NoError(t, fileutil.WriteSecureFile(eventsPath, bufEvents.Bytes()))

	// 1. Gather without apply
	var bufOut bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(cmdCtx, &bufOut))
	err = runCriteriaEvidence(cliCtx, cmd)
	require.NoError(t, err)
	assert.Contains(t, bufOut.String(), "Criteria verification evidence")

	// 2. Apply with dry-run=true
	_ = cmd.Flags().Set("apply", "true")
	_ = cmd.Flags().Set("dry-run", "true")
	bufOut.Reset()
	err = runCriteriaEvidence(cliCtx, cmd)
	require.NoError(t, err)
	assert.Contains(t, bufOut.String(), "Dry-run")

	// 3. Apply with dry-run=false
	_ = cmd.Flags().Set("dry-run", "false")
	bufOut.Reset()
	err = runCriteriaEvidence(cliCtx, cmd)
	require.NoError(t, err)
	assert.Contains(t, bufOut.String(), "Applied validated status to")
}

func TestServiceCmd_SubcommandTransitions(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestBypassAuth().Name(), "1")

	cliCtx := cli.ContextForProjectRoot(proj.Root)
	for _, sub := range []string{"start", "stop", "restart", "status"} {
		var buf bytes.Buffer
		cmd := &cobra.Command{Use: sub}
		cmd.Flags().String("root", proj.Root, "")
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
		cli.SetContext(cmd, cliCtx)
		switch sub {
		case "start":
			_ = runSchedulerServiceStart(cmd, nil)
		case "stop":
			_ = runSchedulerServiceStop(cmd, nil)
		case "restart":
			_ = runSchedulerServiceRestart(cmd, nil)
		case "status":
			_ = runSchedulerServiceStatus(cmd, nil)
		}
	}
}

func TestShowHistoryHelpers_FiltersAndAggregation(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cmd := NewHistoryCmd()
	cli.SetContext(cmd, cliCtx)

	jhc, err := initializeJobHistoryContext(cliCtx, cmd)
	require.NoError(t, err)
	assert.NotNil(t, jhc)

	// parseTimeString branches
	assert.NotNil(t, parseTimeString("1h", true))
	assert.NotNil(t, parseTimeString("30m", false))
	assert.NotNil(t, parseTimeString("today", true))
	assert.Nil(t, parseTimeString("invalid-time-format", true))

	// resolveHistoryCacheWindow
	now := time.Now()
	jhc.Since = &now
	useCache, ids := resolveHistoryCacheWindow(jhc)
	assert.False(t, useCache)
	assert.Nil(t, ids)

	// aggregateJobStats with raw events matching scheduler job kinds
	events := []map[string]any{
		{
			getAuditEventTargetIDField():   "JOB-1",
			getAuditEventTargetKindField(): getSchedulerJobKind(),
			getAuditEventEventTypeField():  "scheduler_job_completed",
			getAuditEventCreatedAtField():  time.Now().Format(time.RFC3339),
		},
		{
			getAuditEventTargetIDField():   "JOB-1",
			getAuditEventTargetKindField(): getSchedulerJobKind(),
			getAuditEventEventTypeField():  "scheduler_job_failed",
			getAuditEventCreatedAtField():  time.Now().Format(time.RFC3339),
		},
		{
			getAuditEventTargetIDField():   "JOB-2",
			getAuditEventTargetKindField(): getSchedulerJobKind(),
			getAuditEventEventTypeField():  "scheduler_job_completed",
			getAuditEventCreatedAtField():  time.Now().Format(time.RFC3339),
		},
	}
	stats := aggregateJobStats(jhc, events)
	assert.Len(t, stats, 2)
	require.NotNil(t, stats["JOB-1"])
	require.NotNil(t, stats["JOB-2"])
	assert.Equal(t, 2, stats["JOB-1"].TotalRuns)
	assert.Equal(t, 1, stats["JOB-1"].SuccessCount)
	assert.Equal(t, 1, stats["JOB-1"].FailCount)

	// outputJobHistory table and json
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	err = outputJobHistory(jhc, []*jobStats{stats["JOB-1"], stats["JOB-2"]})
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "JOB-1")
}

func TestShowActivityHelpers_TableAndBounds(t *testing.T) {
	// activityEventTimeBounds
	events := []jobActivityEvent{
		{CreatedAt: time.Now().Add(-10 * time.Minute).Format(time.RFC3339)},
		{CreatedAt: time.Now().Format(time.RFC3339)},
		{CreatedAt: "invalid"},
	}
	oldest, newest, ok := activityEventTimeBounds(events)
	assert.True(t, ok)
	assert.False(t, oldest.IsZero())
	assert.False(t, newest.IsZero())

	// outputRecentJobSummaryTableToBuffer
	var buf strings.Builder
	outputRecentJobSummaryTableToBuffer(&buf, events, time.Now(), 10)
	assert.NotEmpty(t, buf.String())

	// outputMetricsRecentToBuffer
	var bufMetrics strings.Builder
	outputMetricsRecentToBuffer(&bufMetrics, []string{"JOB-1"}, []string{"JOB-2"}, []string{"JOB-3"})
	assert.Contains(t, bufMetrics.String(), "Metrics (recent):")
	assert.Contains(t, bufMetrics.String(), "JOB-1")
}

func TestAutonomySyntheticTelemetry_All(t *testing.T) {
	assert.NotEmpty(t, autonomyUnavailableTelemetryJSON())
	assert.NotEmpty(t, autonomyTelemetryJSON("sample-id"))
}

func TestTestFailuresConvergenceRouting_SessionContext(t *testing.T) {
	cmd := &cobra.Command{Use: "routing"}
	p, v, m, b, pr, th, err := resolveConvergenceRoutingForCLI(cmd, "", "c1_scope", "default", false)
	assert.NoError(t, err)
	assert.Equal(t, "c1_scope", p)
	assert.Equal(t, "default", v)
	assert.Nil(t, m)
	assert.Nil(t, b)
	assert.Nil(t, pr)
	assert.Nil(t, th)
}

func TestIssuesBundleHealthCmd_DirectRunner(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := NewIssuesBundleHealthCmd()
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

	// 1. When files are missing
	err := runIssuesBundleHealth(cliCtx, cmd)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "issues.json: no open issues")

	// 2. With issues.json populated
	schedDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.SchedulerDir)
	require.NoError(t, fileutil.MkdirAll(schedDir, paths.DirPerm750))
	issuesPayload := map[string]any{
		"status": "issues",
		"issues": []map[string]any{
			{
				"job_id":      "JOB-TEST-1",
				"job_type":    "build",
				"recorded_at": time.Now().Format(time.RFC3339),
			},
		},
	}
	issuesData, _ := json.Marshal(issuesPayload)
	require.NoError(t, fileutil.WriteSecureFile(filepath.Join(schedDir, "issues.json"), issuesData))

	buf.Reset()
	err = runIssuesBundleHealth(cliCtx, cmd)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "JOB-TEST-1")
}

func TestSkipWindowCmd_AllActions(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// Status when missing
	cmdStatus := bldr_cli_cmd_v1.NewSchedulerSkipWindowStatusCommandBuilder()
	cmdStatus.RunE = runSkipWindowStatus
	var bufStatus bytes.Buffer
	cmdStatus.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufStatus))
	cli.SetContext(cmdStatus, cli.ContextForProjectRoot(proj.Root))
	err := runSkipWindowStatus(cmdStatus, nil)
	require.NoError(t, err)
	assert.Contains(t, bufStatus.String(), "no active skip_window")

	// Set with both --for and --until returns error
	cmdSet := bldr_cli_cmd_v1.NewSchedulerSkipWindowSetCommandBuilder()
	cmdSet.RunE = runSkipWindowSet
	cli.SetContext(cmdSet, cli.ContextForProjectRoot(proj.Root))
	_ = cmdSet.Flags().Set("for", "1h")
	_ = cmdSet.Flags().Set("until", time.Now().Add(time.Hour).Format(time.RFC3339))
	err = runSkipWindowSet(cmdSet, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "use either --for or --until, not both")

	// Set without filters returns error
	_ = cmdSet.Flags().Set("until", "")
	err = runSkipWindowSet(cmdSet, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "specify --match-all and/or --job-type")

	// Set valid skip window with match-all
	_ = cmdSet.Flags().Set("match-all", "true")
	var bufSet bytes.Buffer
	cmdSet.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufSet))
	err = runSkipWindowSet(cmdSet, nil)
	require.NoError(t, err)
	assert.Contains(t, bufSet.String(), "skip_window active until")

	// Status when active
	bufStatus.Reset()
	err = runSkipWindowStatus(cmdStatus, nil)
	require.NoError(t, err)
	assert.Contains(t, bufStatus.String(), "match_all: true")

	// Clear skip window
	cmdClear := bldr_cli_cmd_v1.NewSchedulerSkipWindowClearCommandBuilder()
	cmdClear.RunE = runSkipWindowClear
	var bufClear bytes.Buffer
	cmdClear.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufClear))
	cli.SetContext(cmdClear, cli.ContextForProjectRoot(proj.Root))
	err = runSkipWindowClear(cmdClear, nil)
	require.NoError(t, err)
	assert.Contains(t, bufClear.String(), "skip_window cleared")

	// NewSkipWindowCmd wires all subcommands
	parent := NewSkipWindowCmd()
	assert.NotNil(t, parent)
	assert.True(t, parent.HasSubCommands())
}

func TestWebSocketFrame_Sizes(t *testing.T) {
	// Small payload (<= 125 bytes)
	c1, c2 := net.Pipe()
	goroutinelabels.NewGoroutine("ws_test_writer_small", "write small test ws frame").StartSimple(func() {
		defer c1.Close()
		_ = writeWebSocketTextFrame(c1, []byte("short message"))
	})
	buf := make([]byte, 100)
	n, err := c2.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, byte(wsOpcodeText), buf[0])
	assert.Equal(t, byte(len("short message")), buf[1])
	assert.Equal(t, "short message", string(buf[2:n]))
	c2.Close()

	// Medium payload (126 to 65535 bytes)
	c3, c4 := net.Pipe()
	mediumPayload := bytes.Repeat([]byte("a"), 500)
	goroutinelabels.NewGoroutine("ws_test_writer_medium", "write medium test ws frame").StartSimple(func() {
		defer c3.Close()
		_ = writeWebSocketTextFrame(c3, mediumPayload)
	})
	headerBuf := make([]byte, 4)
	_, err = io.ReadFull(c4, headerBuf)
	require.NoError(t, err)
	assert.Equal(t, byte(wsOpcodeText), headerBuf[0])
	assert.Equal(t, byte(wsLenThresholdMedium), headerBuf[1])
	payloadRead := make([]byte, 500)
	_, err = io.ReadFull(c4, payloadRead)
	require.NoError(t, err)
	assert.Equal(t, mediumPayload, payloadRead)
	c4.Close()

	// Large payload (> 65535 bytes)
	c5, c6 := net.Pipe()
	largePayload := bytes.Repeat([]byte("b"), 70000)
	goroutinelabels.NewGoroutine("ws_test_writer_large", "write large test ws frame").StartSimple(func() {
		defer c5.Close()
		_ = writeWebSocketTextFrame(c5, largePayload)
	})
	largeHeader := make([]byte, 10)
	_, err = io.ReadFull(c6, largeHeader)
	require.NoError(t, err)
	assert.Equal(t, byte(wsOpcodeText), largeHeader[0])
	assert.Equal(t, byte(wsLenMarkerLarge), largeHeader[1])
	largeRead := make([]byte, 70000)
	_, err = io.ReadFull(c6, largeRead)
	require.NoError(t, err)
	assert.Equal(t, largePayload, largeRead)
	c6.Close()
}

func TestSchedulerStatusTrigger_Display(t *testing.T) {
	cmd := &cobra.Command{Use: "status"}

	// Table output: not running
	statusDataNotRunning := map[string]any{
		schedulerStatusRunning:    false,
		schedulerFieldProjectRoot: "/tmp/fake-project",
	}
	var bufNotRunning bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufNotRunning))
	err := outputSchedulerStatusTable(cmd, statusDataNotRunning)
	require.NoError(t, err)
	assert.Contains(t, bufNotRunning.String(), "Not running")

	// Table output: running in-process
	statusDataInProcess := map[string]any{
		schedulerStatusRunning:  true,
		schedulerFieldInProcess: true,
	}
	var bufInProcess bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufInProcess))
	err = outputSchedulerStatusTable(cmd, statusDataInProcess)
	require.NoError(t, err)
	assert.Contains(t, bufInProcess.String(), "Running (in this process)")

	// Table output: running as daemon with PID
	statusDataDaemon := map[string]any{
		schedulerStatusRunning:  true,
		schedulerFieldInProcess: false,
		schedulerFieldPID:       12345,
	}
	var bufDaemon bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufDaemon))
	err = outputSchedulerStatusTable(cmd, statusDataDaemon)
	require.NoError(t, err)
	assert.Contains(t, bufDaemon.String(), "Running (PID: 12345)")

	// JSON format output
	_ = cmd.Flags().StringP("format", "f", "json", "")
	_ = cmd.Flags().Set("format", "json")
	var bufJSON bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufJSON))
	err = outputSchedulerStatus(cmd, statusDataDaemon)
	require.NoError(t, err)
	assert.Contains(t, bufJSON.String(), "12345")
}

func TestSchedulerConfigCmd_DirectRunner(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := NewConfigCmd()
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	// 1. Get default config
	var bufGet bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufGet))
	err := runConfig(cmd, nil)
	require.NoError(t, err)

	// 2. Both flags return error
	_ = cmd.Flags().Set("jobs-paused", "true")
	_ = cmd.Flags().Set("no-jobs-paused", "true")
	err = runConfig(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot set both")

	// 3. Set jobs-paused=true
	_ = cmd.Flags().Set("no-jobs-paused", "false")
	err = runConfig(cmd, nil)
	require.NoError(t, err)

	cfg, err := loadSchedulerConfig(proj.Root)
	require.NoError(t, err)
	assert.True(t, cfg.JobsPaused)

	// 4. Set no-jobs-paused=true
	_ = cmd.Flags().Set("jobs-paused", "false")
	_ = cmd.Flags().Set("no-jobs-paused", "true")
	err = runConfig(cmd, nil)
	require.NoError(t, err)

	cfg, err = loadSchedulerConfig(proj.Root)
	require.NoError(t, err)
	assert.False(t, cfg.JobsPaused)
}

func TestTestFailuresList_PopulatedLogs(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// Create callback logs dir with a failure
	cbDir := filepath.Join(proj.Root, paths.ProjectDataDir, "callbacks")
	require.NoError(t, fileutil.MkdirAll(cbDir, paths.DirPerm750))
	cbEntry := map[string]any{
		objects.FieldKeyEventType: schedulerStateFailed,
		"timestamp":               time.Now().Format(time.RFC3339),
		"test_failures": []any{
			"github.com/zqk-os/zqk/pkg/storage.TestMockFailure",
		},
	}
	d, _ := json.Marshal(cbEntry)
	require.NoError(t, fileutil.WriteSecureFile(filepath.Join(cbDir, "callback.jsonl"), d))

	cmd := NewTestFailuresListCmd()
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

	err := listTestFailures(cliCtx, cmd)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "TestMockFailure")
}

func TestLinkedBacklogAcceptanceSection(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := &cobra.Command{Use: "prompt"}
	cmdCtx := pkgctx.WithSecurityContext(pkgctx.NewSystemContext(), pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cmdCtx)
	cli.SetContext(cmd, cli.ContextForProjectRoot(proj.Root))

	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	// Nil cases
	assert.Empty(t, formatLinkedBacklogAcceptanceSection(nil, nil))
	assert.Empty(t, formatLinkedBacklogAcceptanceSection(proc, nil))
	assert.Empty(t, formatLinkedBacklogAcceptanceSection(proc, map[string]any{}))

	// Create criteria
	critID := "CRIT-1792035477264387000-089a9069"
	crit := map[string]any{
		objects.FieldKeyID:            critID,
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusValidated,
		objects.FieldKeyTitle:         "Sample Automated Test Criteria",
		objects.FieldKeyDescription:   "Run zqk test run TST-001 to verify automated_test",
		"category":                    "functional",
	}
	require.NoError(t, proc.Storage().Create(proc.OperationContext(), proc.SecurityContext(), crit))

	// Create backlog item
	bliID := "BLI-1792035477264387500-47626ff3"
	bli := map[string]any{
		objects.FieldKeyID:            bliID,
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusPlanned,
		objects.FieldKeyTitle:         "Sample Backlog Item",
		objects.FieldKeyCriteriaRefs:  []any{critID},
	}
	require.NoError(t, proc.Storage().Create(proc.OperationContext(), proc.SecurityContext(), bli))

	cvs := map[string]any{
		objects.FieldKeyBacklogItemRefs: []any{bliID, "BLI-NONEXISTENT"},
	}
	out := formatLinkedBacklogAcceptanceSection(proc, cvs)
	assert.Contains(t, out, bliID)
	assert.Contains(t, out, critID)
	assert.Contains(t, out, "Could not load object")
}

func TestConvergenceAgentPrompt_HTTPAndStages(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. mergeHTTPAgentPromptDelivery with mock server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"delivered"}`))
	}))
	defer srv.Close()

	rec := &agentdelivery.DeliveryAuditRecord{}
	mergeHTTPAgentPromptDelivery(rec, context.Background(), srv.URL, []byte("# Markdown Prompt"), "CVS-001")
	assert.Equal(t, srv.URL, rec.HTTPURL)
	assert.Equal(t, http.StatusOK, rec.HTTPStatusCode)

	// Failure branch
	recFail := &agentdelivery.DeliveryAuditRecord{}
	mergeHTTPAgentPromptDelivery(recFail, context.Background(), "http://127.0.0.1:1/invalid-unreachable", []byte("# Prompt"), "CVS-001")
	assert.NotEmpty(t, recFail.HTTPError)

	// Empty URL does nothing
	mergeHTTPAgentPromptDelivery(rec, context.Background(), "", []byte("# Prompt"), "CVS-001")

	// 2. agentPromptStageIngest validation failure
	pctx := &pipeline.Context{Ctx: context.Background(), Outcome: make(map[string]any)}
	inInvalid := &agentPromptPipelineInput{}
	_, err := agentPromptStageIngest(pctx, inInvalid)
	assert.Error(t, err)
	assert.True(t, pctx.Outcome[outcomeAgentPromptValidationFailed].(bool))

	// 3. agentPromptStageCommit copyClip branch
	plCopy := &agentPromptPipelinePayload{
		markdown: "# MD",
		copyClip: true,
	}
	pctxCopy := &pipeline.Context{Ctx: context.Background(), Outcome: make(map[string]any)}
	_, errCopy := agentPromptStageCommit(pctxCopy, plCopy)
	_ = errCopy
}

func TestConvergenceSessionPersist_FullLifecycle(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := &cobra.Command{Use: "persist"}
	cmdCtx := pkgctx.WithSecurityContext(pkgctx.NewSystemContext(), pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cmdCtx)
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	now := time.Now().UTC()
	cvsObj := map[string]any{
		objects.FieldKeyID:            "CVS-PERSIST-001",
		objects.FieldKeyKind:          objects.KindConvergenceSession,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Sample Convergence Session",
		"outcome_character":           "pending",
		"hypothesis":                  "Test hypothesis",
		"desired_end_state":           "All tests green",
		"current_phase":               "c1_scope",
		"last_measurement_at":         now.Add(-time.Hour).Format(time.RFC3339),
	}
	require.NoError(t, proc.Storage().Create(proc.OperationContext(), proc.SecurityContext(), cvsObj))

	snap := &schedpkg.TestBundleConvergenceSnapshot{
		HealthWatermarkRFC3339: now.Format(time.RFC3339),
		DeltaAssessment:        "neutral",
	}

	// 1. First persist: new watermark -> Applied
	out1, err := runConvergenceSessionPersist(cliCtx, cmd, "CVS-PERSIST-001", snap, "c2_hypothesize", "default", nil, nil, false, nil, false, "debrief notes", nil)
	require.NoError(t, err)
	assert.NotNil(t, out1)
	assert.True(t, out1.Applied)

	// 2. Second persist with identical snapshot: duplicate watermark -> AuditAppended
	out2, err := runConvergenceSessionPersist(cliCtx, cmd, "CVS-PERSIST-001", snap, "c2_hypothesize", "default", nil, nil, false, nil, false, "debrief notes", nil)
	require.NoError(t, err)
	assert.NotNil(t, out2)
	assert.True(t, out2.AuditAppended)

	// 3. marshalConvergenceOutputJSON
	data, err := marshalConvergenceOutputJSON(snap, "CVS-PERSIST-001", "c2_hypothesize", "default", nil, nil, false, nil, false, "notes", nil, out1, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestSubmitCmd_EdgeFlagsAndErrors(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. Invalid env format
	cmd := NewSubmitCmd()
	_, err := executeSchedulerCmd(t, cmd, []string{"echo hello", "--env", "INVALID_NO_EQUALS"}, proj.Root)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid environment variable format")

	// 2. Full submit with callbacks and working directory
	cmd2 := NewSubmitCmd()
	out, err := executeSchedulerCmd(t, cmd2, []string{
		"echo hello",
		"--workdir", proj.Root,
		"--callback-completion", "echo done",
		"--callback-failure", "echo fail",
		"--retry", "1",
		"--retry-delay", "2",
	}, proj.Root)
	require.NoError(t, err)
	assert.Contains(t, out, "Job submitted:")
}

func TestSchedulerHelpers_RequireAndStatus(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// 1. When not running:
	status, err := requireSchedulerRunning(cliCtx)
	assert.Error(t, err)
	assert.Nil(t, status)

	// 2. requireSchedulerNotRunning succeeds
	statusNotRunning, err := requireSchedulerNotRunning(cliCtx)
	assert.NoError(t, err)
	assert.NotNil(t, statusNotRunning)
	assert.False(t, statusNotRunning.Running)

	// 3. getSchedulerInstance fails when not running
	schedInst, schedStat, err := getSchedulerInstance(cliCtx)
	assert.Error(t, err)
	assert.Nil(t, schedInst)
	assert.NotNil(t, schedStat)

	// 4. resolveCommandProjectRoot
	cmd := &cobra.Command{Use: "test"}
	cli.SetContext(cmd, cliCtx)
	root, err := resolveCommandProjectRoot(cmd)
	assert.NoError(t, err)
	assert.NotEmpty(t, root)
}

func TestEscalate_MaskAndRun(t *testing.T) {
	assert.Empty(t, maskWebhookURL(""))
	assert.Equal(t, "********", maskWebhookURL("short"))
	assert.Equal(t, "https://example.com/webhook/********", maskWebhookURL("https://example.com/webhook/custom-token"))

	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := NewEscalateCmd()
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)
	_ = cmd.Flags().Set("dry-run", "true")
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	err := runEscalate(cmd, nil)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "configured")
}

func TestDumpCmd_NotRunning(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := NewDumpCmd()
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)
	err := runDump(cliCtx, cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scheduler daemon is not running")

	// captureDiagnostics
	diagDir := filepath.Join(proj.Root, "diag")
	_ = captureDiagnostics(diagDir, "prefix")
}

func TestSchedulerCoordinationCommands_AllEmitters(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	ctx := context.Background()
	emitSchedulerStartEventViaCoordinator(ctx, proj.Root, nil, "start msg", "system", nil)
	emitSchedulerStopEventViaCoordinator(ctx, proj.Root, nil, "stop msg", 1234, "system", nil)
	emitSchedulerTriggerEventViaCoordinator(ctx, proj.Root, nil, "trigger msg", "JOB-1", "system", nil)
	emitSchedulerWarningEventViaCoordinator(ctx, proj.Root, nil, "warn msg", errors.New("err"), "system")
	emitSchedulerDumpEventViaCoordinator(ctx, proj.Root, nil, "started", nil, "system", nil)
}

func TestSchedulerStop_ExternalProcessExecution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// 1. Force kill running process
	sleep1 := testkit.ManagedCommand(t, t.Context(), "sleep", "30")
	require.NoError(t, sleep1.Start())
	defer func() { _ = sleep1.Process.Kill() }()

	pidPath := paths.SchedulerPIDFilePath(proj.Root)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(pidPath)))
	require.NoError(t, fileutil.WriteFile(pidPath, []byte(fmt.Sprintf("%d", sleep1.Process.Pid)), paths.FilePerm600))

	cmdForce := NewStopCmd()
	_ = cmdForce.Flags().Set("force", "true")
	var bufForce bytes.Buffer
	cmdForce.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufForce))
	cli.SetContext(cmdForce, cliCtx)
	err := stopScheduler(cliCtx, cmdForce)
	assert.NoError(t, err)
	assert.Contains(t, bufForce.String(), "SIGKILL")

	// 2. Wait for exit with SIGTERM
	sleep2 := testkit.ManagedCommand(t, t.Context(), "sleep", "30")
	require.NoError(t, sleep2.Start())
	defer func() { _ = sleep2.Process.Kill() }()

	require.NoError(t, fileutil.WriteFile(pidPath, []byte(fmt.Sprintf("%d", sleep2.Process.Pid)), paths.FilePerm600))

	cmdWait := NewStopCmd()
	_ = cmdWait.Flags().Set("wait", "true")
	_ = cmdWait.Flags().Set("max-wait", "5s")
	var bufWait bytes.Buffer
	cmdWait.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufWait))
	cli.SetContext(cmdWait, cliCtx)
	err = stopScheduler(cliCtx, cmdWait)
	assert.NoError(t, err)

	// 3. Fire-and-forget SIGTERM (neither force nor wait)
	sleep3 := testkit.ManagedCommand(t, t.Context(), "sleep", "30")
	require.NoError(t, sleep3.Start())
	defer func() { _ = sleep3.Process.Kill() }()

	require.NoError(t, fileutil.WriteFile(pidPath, []byte(fmt.Sprintf("%d", sleep3.Process.Pid)), paths.FilePerm600))

	cmdSig := NewStopCmd()
	var bufSig bytes.Buffer
	cmdSig.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufSig))
	cli.SetContext(cmdSig, cliCtx)
	err = stopScheduler(cliCtx, cmdSig)
	assert.NoError(t, err)
	assert.Contains(t, bufSig.String(), "SIGTERM")
}

func TestSchedulerDaemon_ExecAndDetachedProcesses(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// configureSchedulerDaemonExec
	cmd1 := testkit.ManagedCommand(t, t.Context(), "/bin/echo", "test1")
	cleanup1, err := configureSchedulerDaemonExec(cmd1, proj.Root, "test-label")
	require.NoError(t, err)
	require.NotNil(t, cleanup1)
	cleanup1()

	// startDetachedSchedulerDaemonProcess
	cmd2 := testkit.ManagedCommand(t, t.Context(), "/bin/echo", "test2")
	cleanup2, err := configureSchedulerDaemonExec(cmd2, proj.Root, "test-label2")
	require.NoError(t, err)
	err = startDetachedSchedulerDaemonProcess(cmd2)
	assert.NoError(t, err)
	cleanup2()

	// spawnConfiguredSchedulerDaemon
	cmd3 := testkit.ManagedCommand(t, t.Context(), "/bin/echo", "test3")
	cleanup3, err := spawnConfiguredSchedulerDaemon(cmd3, proj.Root, "test-label3", "spawn failed")
	require.NoError(t, err)
	require.NotNil(t, cleanup3)
	cleanup3()
}

func TestSubmitJob_Comprehensive(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// ensureSubmitFlags on clean command
	bareCmd := &cobra.Command{}
	ensureSubmitFlags(bareCmd)
	assert.NotNil(t, bareCmd.Flags().Lookup("workdir"))

	// 1. Submit valid job with all optional flags
	cmd := NewSubmitCmd()
	_ = cmd.Flags().Set("title", "Custom Job Title")
	_ = cmd.Flags().Set("description", "Custom Job Description")
	_ = cmd.Flags().Set("workdir", proj.Root)
	_ = cmd.Flags().Set("env", "FOO=bar")
	_ = cmd.Flags().Set("retry", "3")
	_ = cmd.Flags().Set("retry-delay", "10")
	_ = cmd.Flags().Set("callback-completion", "http://localhost:9999/complete")
	_ = cmd.Flags().Set("callback-failure", "http://localhost:9999/fail")
	_ = cmd.Flags().Set("max-runtime", "1800")
	var bufOut bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufOut))
	cli.SetContext(cmd, cliCtx)

	err := runSubmit(cmd, []string{"echo", "hello", "world"})
	require.NoError(t, err)
	assert.Contains(t, bufOut.String(), "Job submitted:")
	assert.Contains(t, bufOut.String(), "Timeout: 1800 seconds")

	// 2. Submit invalid env format
	cmdInvalid := NewSubmitCmd()
	_ = cmdInvalid.Flags().Set("env", "INVALID_NO_EQUALS")
	cli.SetContext(cmdInvalid, cliCtx)
	err = runSubmit(cmdInvalid, []string{"echo", "hi"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid environment variable format")

	// 3. Helpers
	now := time.Now().UTC()
	id := newSubmitJobID(now)
	assert.True(t, strings.HasPrefix(id, "SCH-"))
}

func TestServiceCmd_ExtendedSubcommands(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestBypassAuth().Name(), "1")

	// 1. Service list with and without orphans
	cmdList := &cobra.Command{}
	cmdList.Flags().Bool("orphans", false, "")
	var bufList bytes.Buffer
	cmdList.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufList))
	err := runSchedulerServiceList(cmdList, nil)
	assert.NoError(t, err)
	assert.Contains(t, bufList.String(), "[")

	_ = cmdList.Flags().Set("orphans", "true")
	bufList.Reset()
	err = runSchedulerServiceList(cmdList, nil)
	assert.NoError(t, err)

	// 2. Service GC
	cmdGC := &cobra.Command{}
	cmdGC.SetContext(context.Background())
	err = runSchedulerServiceGC(cmdGC, nil)
	assert.NoError(t, err)

	// 3. Service Rebind - missing flags
	cmdRebind := &cobra.Command{}
	cmdRebind.Flags().String("root-id", "", "")
	cmdRebind.Flags().String("new-path", "", "")
	cmdRebind.SetContext(context.Background())
	err = runSchedulerServiceRebind(cmdRebind, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--root-id and --new-path are required")

	// 4. Service status
	cmdStatus := &cobra.Command{}
	cmdStatus.Flags().String("root", proj.Root, "")
	cmdStatus.SetContext(context.Background())
	_ = runSchedulerServiceStatus(cmdStatus, nil)
}

func TestSchedulerHealth_RecordExternalHealthMetric_Execution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	ctx := pkgctx.NewSystemContext()
	store, err := cli.NewStorageProviderFromFactory(ctx, proj.Root)
	require.NoError(t, err)

	secCtx := pkgctx.NewSystemSecurityContext()
	now := time.Now().UTC()

	// Execute with non-zero values to hit all statement branches
	recordExternalHealthMetric(store, secCtx, now, 5, 2, 1, 1, 150*time.Millisecond)

	// Query created SHM metric
	listRes, err := store.List(ctx, secCtx, pkgctx.NewStorageContext(), storagepkg.ListFilter{
		Kind: objects.KindSchedulerHealthMetric,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, listRes.Objects)
}

func TestAgentPrompt_BacklogAcceptanceAndDelivery(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	ctx := pkgctx.NewSystemContext()
	store, err := cli.NewStorageProviderFromFactory(ctx, proj.Root)
	require.NoError(t, err)
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create criteria
	critObj := map[string]any{
		objects.FieldKeyID:            "CRIT-1792035477264387000-089a9099",
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusOriginated,
		objects.FieldKeyTitle:         "Sample Criteria for Backlog Link",
		objects.FieldKeyDescription:   "Verification command: `zqk test run TST-123` must succeed.",
		objects.FieldKeyContext:       "Run `go test ./...` to verify",
		"category":                    "functional",
	}
	require.NoError(t, store.Create(ctx, secCtx, critObj))

	// Create backlog item
	bliObj := map[string]any{
		objects.FieldKeyID:            "BLI-1792035477264387000-089a9099",
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "planned",
		objects.FieldKeyTitle:         "Sample Backlog Item for Prompt",
		objects.FieldKeyDescription:   "BLI description",
		objects.FieldKeyCriteriaRefs:  []any{"CRIT-1792035477264387000-089a9099"},
	}
	require.NoError(t, store.Create(ctx, secCtx, bliObj))

	// Create CVS session
	cvsObj := map[string]any{
		objects.FieldKeyID:              "CVS-BACKLOG-001",
		objects.FieldKeyKind:            objects.KindConvergenceSession,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyTitle:           "CVS Session with Backlog",
		objects.FieldKeyBacklogItemRefs: []any{"BLI-1792035477264387000-089a9099"},
	}
	require.NoError(t, store.Create(ctx, secCtx, cvsObj))

	cmd := &cobra.Command{}
	cmdCtx := pkgctx.WithSecurityContext(ctx, secCtx)
	cmd.SetContext(cmdCtx)
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)
	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)
	secText := formatLinkedBacklogAcceptanceSection(proc, cvsObj)
	assert.Contains(t, secText, "Sample Backlog Item for Prompt")
	assert.Contains(t, secText, "CRIT-1792035477264387000-089a9099")
	assert.Contains(t, secText, "Verification commands")

	// Test HTTP prompt delivery with test HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"delivered"}`))
	}))
	defer ts.Close()

	var bufDeliver bytes.Buffer
	cmdDeliver := &cobra.Command{}
	cmdDeliver.SetContext(pkgctx.WithCommandOutputWriter(cmdCtx, &bufDeliver))
	cli.SetContext(cmdDeliver, cliCtx)

	err = deliverAgentPromptMarkdownOutput(cmdDeliver, proj.Root, "# Test Prompt Content\n", "Footer Note", "CVS-BACKLOG-001", "none", "urgent", ts.URL)
	assert.NoError(t, err)
}

func TestWebSocket_FramesAndUpgrade(t *testing.T) {
	// 1. writeWebSocketTextFrame across size thresholds
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	goroutinelabels.NewGoroutine("ws_frame_reader", "read incoming ws frames").StartSimple(func() {
		// Read frames
		buf := make([]byte, 1024)
		for {
			if _, err := c2.Read(buf); err != nil {
				return
			}
		}
	})

	// Small (<=125 bytes)
	assert.NoError(t, writeWebSocketTextFrame(c1, []byte("short message")))

	// Medium (126 <= len <= 65535)
	medPayload := make([]byte, 500)
	assert.NoError(t, writeWebSocketTextFrame(c1, medPayload))

	// 2. upgradeToWebSocket rejection
	req, _ := http.NewRequest("GET", "/api/ws", nil)
	req.Header.Set("Origin", "http://malicious.evil.com")
	w := httptest.NewRecorder()
	_, err := upgradeToWebSocket(w, req)
	assert.Error(t, err)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSchedulerCommands_IssuesAndStatus(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// 1. runIssues when none
	cmdIssues := NewIssuesCmd()
	var bufIssues bytes.Buffer
	cmdIssues.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufIssues))
	cli.SetContext(cmdIssues, cliCtx)
	err := runIssues(cmdIssues, nil)
	assert.NoError(t, err)
	assert.Contains(t, bufIssues.String(), "Issues: none")

	// Write an issue and re-read
	issuesPath := filepath.Join(proj.Root, paths.ProjectDataDir, paths.SchedulerDir, "issues.json")
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(issuesPath)))
	payload := &schedpkg.IssuesPayload{
		Status:    "degraded",
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Issues: []schedpkg.Issue{
			{JobID: "SCH-1", JobType: "test", Error: "test failure", At: time.Now().UTC().Format(time.RFC3339)},
		},
	}
	payloadBytes, _ := json.Marshal(payload)
	require.NoError(t, fileutil.WriteSecureFile(issuesPath, payloadBytes))

	bufIssues.Reset()
	err = runIssues(cmdIssues, nil)
	assert.NoError(t, err)
	assert.Contains(t, bufIssues.String(), "Issues (1):")
	assert.Contains(t, bufIssues.String(), "SCH-1")

	// Clear issues
	cmdClear := NewClearIssuesCmd()
	var bufClear bytes.Buffer
	cmdClear.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufClear))
	cli.SetContext(cmdClear, cliCtx)
	err = runClearIssues(cmdClear, nil)
	assert.NoError(t, err)
	assert.Contains(t, bufClear.String(), "cleared successfully")

	// 2. runStatus when not running
	cmdStat := NewStatusCmd()
	var bufStat bytes.Buffer
	cmdStat.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufStat))
	cli.SetContext(cmdStat, cliCtx)
	err = runStatus(cmdStat, nil)
	assert.NoError(t, err)
	assert.Contains(t, bufStat.String(), "Not running")

	// Status as JSON
	_ = cmdStat.Flags().Set("format", "json")
	bufStat.Reset()
	err = runStatus(cmdStat, nil)
	assert.NoError(t, err)
	assert.Contains(t, bufStat.String(), "\"running\": false")

	// 3. runList
	cmdList := NewListCmd()
	var bufListCmd bytes.Buffer
	cmdList.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufListCmd))
	cli.SetContext(cmdList, cliCtx)
	err = runList(cmdList, nil)
	assert.NoError(t, err)
}

func TestSchedulerActivityAndHistory_Commands(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// ensureActivityFlags & ensureHistoryFlags on bare commands
	bareAct := &cobra.Command{}
	ensureActivityFlags(bareAct)
	assert.NotNil(t, bareAct.Flags().Lookup("watch"))

	bareHist := &cobra.Command{}
	ensureHistoryFlags(bareHist)
	assert.NotNil(t, bareHist.Flags().Lookup("since"))

	// runActivity
	cmdAct := NewActivityCmd()
	var bufAct bytes.Buffer
	cmdAct.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufAct))
	cli.SetContext(cmdAct, cliCtx)
	err := runActivity(cmdAct, nil)
	assert.NoError(t, err)

	// runActivity JSON
	_ = cmdAct.Flags().Set("format", "json")
	bufAct.Reset()
	err = runActivity(cmdAct, nil)
	assert.NoError(t, err)

	// runHistory
	cmdHist := NewHistoryCmd()
	var bufHist bytes.Buffer
	cmdHist.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufHist))
	cli.SetContext(cmdHist, cliCtx)
	err = runHistory(cmdHist, nil)
	assert.NoError(t, err)

	// runHistory JSON
	_ = cmdHist.Flags().Set("format", "json")
	bufHist.Reset()
	err = runHistory(cmdHist, nil)
	assert.NoError(t, err)

	// buildSchedulerBusyness
	busyness := buildSchedulerBusyness(proj.Root, nil)
	assert.Equal(t, 0, busyness.Executing)
}

func TestSchedulerStop_FullSignalFlows(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)

	// 1. Force kill flow
	{
		dummyCmd := testkit.ManagedCommand(t, t.Context(), "sleep", "30")
		require.NoError(t, dummyCmd.Start())
		pid := dummyCmd.Process.Pid
		defer func() {
			_ = dummyCmd.Process.Kill()
			_ = dummyCmd.Wait()
		}()

		pidFile := paths.SchedulerPIDFilePath(proj.Root)
		require.NoError(t, fileutil.EnsureDir(filepath.Dir(pidFile)))
		require.NoError(t, fileutil.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644))

		cmd := NewStopCmd()
		_ = cmd.Flags().Set("force", "true")
		cliCtx := cli.ContextForProjectRoot(proj.Root)
		cli.SetContext(cmd, cliCtx)
		var buf bytes.Buffer
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

		err := stopScheduler(cliCtx, cmd)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "SIGKILL sent")
		_ = dummyCmd.Wait()
	}

	// 2. Wait for exit flow
	{
		dummyCmd := testkit.ManagedCommand(t, t.Context(), "sleep", "30")
		require.NoError(t, dummyCmd.Start())
		pid := dummyCmd.Process.Pid
		defer func() {
			_ = dummyCmd.Process.Kill()
			_ = dummyCmd.Wait()
		}()

		pidFile := paths.SchedulerPIDFilePath(proj.Root)
		require.NoError(t, fileutil.EnsureDir(filepath.Dir(pidFile)))
		require.NoError(t, fileutil.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644))

		cmd := NewStopCmd()
		_ = cmd.Flags().Set("wait", "true")
		_ = cmd.Flags().Set("max-wait", "2s")
		cliCtx := cli.ContextForProjectRoot(proj.Root)
		cli.SetContext(cmd, cliCtx)
		var buf bytes.Buffer
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

		err := stopScheduler(cliCtx, cmd)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Waiting up to 2s for exit")
		assert.Contains(t, buf.String(), "Scheduler stop completed")
		_ = dummyCmd.Wait()
	}

	// 3. Default async stop flow
	{
		dummyCmd := testkit.ManagedCommand(t, t.Context(), "sleep", "30")
		require.NoError(t, dummyCmd.Start())
		pid := dummyCmd.Process.Pid
		defer func() {
			_ = dummyCmd.Process.Kill()
			_ = dummyCmd.Wait()
		}()

		pidFile := paths.SchedulerPIDFilePath(proj.Root)
		require.NoError(t, fileutil.EnsureDir(filepath.Dir(pidFile)))
		require.NoError(t, fileutil.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644))

		cmd := NewStopCmd()
		cliCtx := cli.ContextForProjectRoot(proj.Root)
		cli.SetContext(cmd, cliCtx)
		var buf bytes.Buffer
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

		err := stopScheduler(cliCtx, cmd)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "SIGTERM sent")
		assert.Contains(t, buf.String(), "Shutdown is in progress")
		_ = dummyCmd.Wait()
	}
}

func TestSchedulerDaemon_SpawnAndDetached(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)

	// Test spawnConfiguredSchedulerDaemon with short-lived /bin/echo
	echoCmd := testkit.ManagedCommand(t, t.Context(), "/bin/echo", "hello-daemon")
	cleanup, err := spawnConfiguredSchedulerDaemon(echoCmd, proj.Root, "test", "err")
	assert.NoError(t, err)
	if cleanup != nil {
		cleanup()
	}

	// Test StartSchedulerDaemonForRoot when scheduler is already running
	dummyCmd := testkit.ManagedCommand(t, t.Context(), "sleep", "30")
	require.NoError(t, dummyCmd.Start())
	defer func() {
		_ = dummyCmd.Process.Kill()
		_ = dummyCmd.Wait()
	}()
	pidFile := paths.SchedulerPIDFilePath(proj.Root)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(pidFile)))
	require.NoError(t, fileutil.WriteFile(pidFile, []byte(strconv.Itoa(dummyCmd.Process.Pid)), 0644))

	err = StartSchedulerDaemonForRoot(proj.Root)
	assert.NoError(t, err)
}

func TestSchedulerRestart_DirectRunner(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	cmd := NewRestartCmd()
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	cli.SetContext(cmd, cliCtx)

	// Restart when daemon is not running: stop succeeds idempotently, background start returns (may report not running)
	_ = runRestart(cmd, nil)
}

func TestTestFailuresRerun_WithFailures(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	callbackLogsDir := filepath.Join(proj.Root, paths.ProjectDataDir, "callbacks")
	require.NoError(t, fileutil.EnsureDir(callbackLogsDir))
	entry := map[string]any{
		"event":        "test_bundle_completed",
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"package":      "github.com/zqk-os/zqk/pkg/example",
		"failed":       true,
		"failed_tests": []string{"TestOne", "TestTwo"},
	}
	data, _ := json.Marshal(entry)
	require.NoError(t, fileutil.WriteFile(filepath.Join(callbackLogsDir, "run.jsonl"), append(data, '\n'), 0644))

	cmd := NewTestFailuresRerunCmd()
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	err := runTestFailuresRerun(cmd, nil)
	assert.NoError(t, err)
}

func TestServiceCmd_TransitionsAndRunners(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// Direct command runners
	cmd := NewServiceCmd()
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

	// runSchedulerServiceList
	_ = runSchedulerServiceList(cmd, nil)

	// executeSchedulerServiceTransition invalid
	err := executeSchedulerServiceTransition(cmd, "start", hostservice.DesiredStateEnabled, func(a hostservice.PlatformAdapter, e hostservice.Entry) error {
		return errors.New("transition failed")
	})
	assert.Error(t, err)
}

func TestShowActivityHelpers_FiltersAndAges(t *testing.T) {
	now := time.Now().UTC()
	// parseActivitySince
	t1, err := parseActivitySince("24h", now)
	assert.NoError(t, err)
	assert.True(t, t1.Before(now))

	t2, err := parseActivitySince("2026-10-15T00:00:00Z", now)
	assert.NoError(t, err)
	assert.False(t, t2.IsZero())

	_, err = parseActivitySince("invalid-time", now)
	assert.Error(t, err)

	_, err = parseActivitySince("", now)
	assert.Error(t, err)

	// formatRelativeAge
	assert.Equal(t, "just now", formatRelativeAge(now, now))
	assert.Equal(t, "5m ago", formatRelativeAge(now.Add(-5*time.Minute), now))
	assert.Equal(t, "2h ago", formatRelativeAge(now.Add(-2*time.Hour), now))
	assert.Equal(t, "3d ago", formatRelativeAge(now.Add(-72*time.Hour), now))

	// coerceFloat64
	val, ok := coerceFloat64(42)
	assert.True(t, ok)
	assert.Equal(t, 42.0, val)

	val, ok = coerceFloat64(42.5)
	assert.True(t, ok)
	assert.Equal(t, 42.5, val)

	val, ok = coerceFloat64(int64(10))
	assert.True(t, ok)
	assert.Equal(t, 10.0, val)

	val, ok = coerceFloat64("12")
	assert.True(t, ok)
	assert.Equal(t, 12.0, val)

	val, ok = coerceFloat64("invalid")
	assert.False(t, ok)
	assert.Equal(t, 0.0, val)

	// formatOutcomeWithEmoji
	assert.Contains(t, formatOutcomeWithEmoji("scheduler_job_completed", "success"), "Success")
	assert.Contains(t, formatOutcomeWithEmoji("scheduler_job_failed", "failure"), "Failed")
	assert.Contains(t, formatOutcomeWithEmoji("scheduler_job_started", "running"), "Started")
	assert.Equal(t, "custom", formatOutcomeWithEmoji("other", "custom"))

	// determineOutcomeFromEventType
	assert.Equal(t, "Success", determineOutcomeFromEventType("scheduler_job_completed"))
	assert.Equal(t, "Failed", determineOutcomeFromEventType("scheduler_job_failed"))
	assert.Equal(t, "scheduler_job_started", determineOutcomeFromEventType("scheduler_job_started"))
	assert.Equal(t, "other", determineOutcomeFromEventType("other"))
}

func TestConvergenceSpecLoadFailureCmd_DirectRunner(t *testing.T) {
	failCmd := convergenceSpecLoadFailureCmd(errors.New("simulated spec failure"))
	assert.NotNil(t, failCmd)
	err := failCmd.RunE(failCmd, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "simulated spec failure")
}

func TestWaitCmd_DirectRunner(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// Missing args
	cmd := NewWaitCmd()
	cli.SetContext(cmd, cliCtx)
	err := runWait(cmd, []string{})
	assert.Equal(t, ErrJobNotFound, err)

	err = runWait(cmd, []string{""})
	assert.Equal(t, ErrJobNotFound, err)

	// Canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd.SetContext(ctx)
	_ = cmd.Flags().Set("timeout", "100ms")
	err = runWait(cmd, []string{"JOB-CANCEL"})
	assert.Error(t, err)

	// Output helpers
	res := &WaitResult{
		JobID:    "JOB-99",
		Status:   "complete",
		Duration: "5s",
	}
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	err = outputWaitResult(cmd, res)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "JOB-99")

	_ = cmd.Flags().Set("format", "json")
	buf.Reset()
	err = outputWaitResult(cmd, res)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "\"job_id\": \"JOB-99\"")
}

func TestSchedulerCommands_RunStartDirect(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// Run with context when already running (mocked via dummy PID)
	cmd := NewStartCmd()
	cli.SetContext(cmd, cliCtx)
	dummyCmd := testkit.ManagedCommand(t, t.Context(), "sleep", "30")
	require.NoError(t, dummyCmd.Start())
	defer func() {
		_ = dummyCmd.Process.Kill()
		_ = dummyCmd.Wait()
	}()
	pidFile := paths.SchedulerPIDFilePath(proj.Root)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(pidFile)))
	require.NoError(t, fileutil.WriteFile(pidFile, []byte(fmt.Sprintf("%d", dummyCmd.Process.Pid)), 0644))

	_ = cmd.Flags().Set("foreground", "false")
	err := runStart(cmd, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
}

func TestStartScheduler_CleanContextCancellation(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestBypassAuth().Name(), "1")
	t.Setenv(zqkenv.SchedulerMaxWallDuration().Name(), "300ms")

	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cmd := NewStartCmd()
	_ = cmd.Flags().Set("foreground", "true")
	_ = cmd.Flags().Set("minimal", "true")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	cmd.SetContext(ctx)
	cli.SetContext(cmd, cliCtx)

	err := startScheduler(cliCtx, cmd)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		assert.Contains(t, err.Error(), "canceled")
	}
}

func TestSubmitJob_Success(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	cmd := NewSubmitCmd()
	cli.SetContext(cmd, cliCtx)
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	_ = cmd.Flags().Set("title", "My Title")
	_ = cmd.Flags().Set("description", "My Desc")
	_ = cmd.Flags().Set("workdir", proj.Root)
	_ = cmd.Flags().Set("env", "FOO=BAR")
	_ = cmd.Flags().Set("max-runtime", "300")
	_ = cmd.Flags().Set("callback-completion", "http://localhost:9999/done")
	_ = cmd.Flags().Set("callback-failure", "http://localhost:9999/fail")

	err := submitJob(cliCtx, cmd, []string{"echo", "hello", "world"})
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Job submitted:")
}

func TestShowHistoryHelpers_CacheAndWindow(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)

	// Seed activity cache
	cache := schedpkg.GetGlobalActivityCache()
	cache.UpdateEvent("SCH-TEST-1", "scheduler_job_completed", time.Second, nil)
	cache.UpdateEvent("SCH-TEST-2", "scheduler_job_failed", time.Second, errors.New("fail"))

	jhc := &JobHistoryContext{
		ProjectRoot: proj.Root,
		JobIDFilter: "SCH-TEST-1",
	}
	stats := aggregateJobStatsFromCache(jhc)
	assert.NotEmpty(t, stats)

	jhcAll := &JobHistoryContext{
		ProjectRoot: proj.Root,
	}
	statsAll := aggregateJobStatsFromCache(jhcAll)
	assert.NotEmpty(t, statsAll)

	// resolveHistoryCacheWindow (unpopulated)
	pop, ids := resolveHistoryCacheWindow(jhcAll)
	assert.False(t, pop)
	assert.Nil(t, ids)

	// Populate high volume cache and save
	hvCache := storagepkg.GetGlobalHighVolumeEventCache()
	defer hvCache.InvalidateForProject(proj.Root)
	hvCache.Set(&storagepkg.HighVolumeEventCacheEntry{
		ID:        "AUD-TEST-1",
		Kind:      objects.KindAuditEvent,
		CreatedAt: time.Now().UTC().Add(-time.Hour),
		Exists:    true,
	})
	_ = hvCache.SaveCache(proj.Root)

	// 1. With Since and Until
	since := time.Now().UTC().Add(-2 * time.Hour)
	until := time.Now().UTC()
	jhcWindow := &JobHistoryContext{
		ProjectRoot: proj.Root,
		Since:       &since,
		Until:       &until,
		Limit:       10,
	}
	pop, ids = resolveHistoryCacheWindow(jhcWindow)
	assert.True(t, pop)
	assert.NotEmpty(t, ids)

	// 2. Without JobIDFilter (uses 7d default window)
	jhcNoFilter := &JobHistoryContext{
		ProjectRoot: proj.Root,
		Limit:       10,
	}
	pop, ids = resolveHistoryCacheWindow(jhcNoFilter)
	assert.True(t, pop)
	assert.NotEmpty(t, ids)

	// 3. With JobIDFilter non-empty and no since/until -> returns false, nil
	jhcJobFilter := &JobHistoryContext{
		ProjectRoot: proj.Root,
		JobIDFilter: "SCH-XYZ",
		Limit:       10,
	}
	pop, ids = resolveHistoryCacheWindow(jhcJobFilter)
	assert.False(t, pop)
	assert.Nil(t, ids)
}

func TestTriggerJob_RunningInAnotherProcess(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	dummyCmd := testkit.ManagedCommand(t, t.Context(), "sleep", "30")
	require.NoError(t, dummyCmd.Start())
	defer func() {
		_ = dummyCmd.Process.Kill()
		_ = dummyCmd.Wait()
	}()
	pidFile := paths.SchedulerPIDFilePath(proj.Root)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(pidFile)))
	require.NoError(t, fileutil.WriteFile(pidFile, []byte(strconv.Itoa(dummyCmd.Process.Pid)), 0644))

	cmd := NewTriggerCmd()
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	cli.SetContext(cmd, cliCtx)

	err := triggerJob(cliCtx, cmd, "SCH-TEST-TRIGGER")
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "has been queued")
}

func TestServiceCmd_SubcommandRunners(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	cmd := NewServiceCmd()
	cli.SetContext(cmd, cliCtx)
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

	// start, stop, restart, uninstall, status
	assert.Error(t, runSchedulerServiceStart(cmd, nil))
	assert.Error(t, runSchedulerServiceStop(cmd, nil))
	assert.Error(t, runSchedulerServiceRestart(cmd, nil))
	assert.Error(t, runSchedulerServiceUninstall(cmd, nil))
	assert.Error(t, runSchedulerServiceStatus(cmd, nil))

	// list command with flags
	listCmd := bldr_cli_cmd_v1.NewSchedulerServiceListCommandBuilder()
	cli.SetContext(listCmd, cliCtx)
	var listBuf bytes.Buffer
	listCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &listBuf))
	assert.NoError(t, runSchedulerServiceList(listCmd, nil))
	_ = listCmd.Flags().Set("orphans", "true")
	assert.NoError(t, runSchedulerServiceList(listCmd, nil))
}

func TestSchedulerCore_StartSchedulerSingletonGuard(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// Acquire singleton guard first
	release, err := singleton.Guard(proj.Root, "scheduler")
	require.NoError(t, err)
	defer release()

	cmd := NewStartCmd()
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	cli.SetContext(cmd, cliCtx)

	err = startScheduler(cliCtx, cmd)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "already running")
}

func TestRecordCvsOrchestrateRun_Cases(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// Case 1: nil context
	cmd := NewRecordCvsOrchestrateRunCmd()
	assert.Error(t, runRecordCvsOrchestrateRun(cmd, nil))

	// Case 2: empty stdin
	cmd = NewRecordCvsOrchestrateRunCmd()
	cli.SetContext(cmd, cliCtx)
	r, w, err := os.Pipe()
	require.NoError(t, err)
	w.Close()
	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()
	assert.Error(t, runRecordCvsOrchestrateRun(cmd, nil))
	os.Stdin = origStdin

	// Case 3: invalid JSON
	r, w, err = os.Pipe()
	require.NoError(t, err)
	_, _ = w.WriteString("invalid json")
	w.Close()
	os.Stdin = r
	assert.Error(t, runRecordCvsOrchestrateRun(cmd, nil))
	os.Stdin = origStdin

	// Case 4: dry-run with wrong schema
	r, w, err = os.Pipe()
	require.NoError(t, err)
	_, _ = w.WriteString(`{"schema_version": "v0"}`)
	w.Close()
	os.Stdin = r
	_ = cmd.Flags().Set("dry-run", "true")
	assert.Error(t, runRecordCvsOrchestrateRun(cmd, nil))
	os.Stdin = origStdin

	// Case 5: dry-run with correct schema
	r, w, err = os.Pipe()
	require.NoError(t, err)
	rec := schedpkg.CVSOrchestrateRunV1{
		SchemaVersion: schedpkg.CVSOrchestrateRunSchemaVersion,
		CvsID:         "CVS-100",
		Stage:         "measurement",
		ExitCode:      0,
	}
	data, _ := json.Marshal(rec)
	_, _ = w.Write(data)
	w.Close()
	os.Stdin = r
	assert.NoError(t, runRecordCvsOrchestrateRun(cmd, nil))
	os.Stdin = origStdin

	// Case 6: real run writing to jsonl
	jsonlOut := filepath.Join(proj.Root, "cvs_runs.jsonl")
	r, w, err = os.Pipe()
	require.NoError(t, err)
	_, _ = w.Write(data)
	w.Close()
	os.Stdin = r
	_ = cmd.Flags().Set("dry-run", "false")
	_ = cmd.Flags().Set("jsonl-path", jsonlOut)
	assert.NoError(t, runRecordCvsOrchestrateRun(cmd, nil))
	os.Stdin = origStdin
	assert.FileExists(t, jsonlOut)
}

func TestSkipWindowCommands_All(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	parent := NewSkipWindowCmd()
	require.NotNil(t, parent)

	setCmd := bldr_cli_cmd_v1.NewSchedulerSkipWindowSetCommandBuilder()
	cli.SetContext(setCmd, cliCtx)
	var outBuf bytes.Buffer
	setCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &outBuf))

	// Error if both --for and --until
	_ = setCmd.Flags().Set("for", "1h")
	_ = setCmd.Flags().Set("until", "2026-10-15T00:00:00Z")
	assert.Error(t, runSkipWindowSet(setCmd, nil))

	// Error if invalid --for
	_ = setCmd.Flags().Set("until", "")
	_ = setCmd.Flags().Set("for", "invalid-duration")
	assert.Error(t, runSkipWindowSet(setCmd, nil))

	// Error if neither --for nor --until
	_ = setCmd.Flags().Set("for", "")
	assert.Error(t, runSkipWindowSet(setCmd, nil))

	// Success with --for 2h and --match-all
	_ = setCmd.Flags().Set("for", "2h")
	_ = setCmd.Flags().Set("match-all", "true")
	_ = setCmd.Flags().Set("reason", "testing skip window")
	assert.NoError(t, runSkipWindowSet(setCmd, nil))

	// Status command
	statusCmd := bldr_cli_cmd_v1.NewSchedulerSkipWindowStatusCommandBuilder()
	cli.SetContext(statusCmd, cliCtx)
	var statusBuf bytes.Buffer
	statusCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &statusBuf))
	assert.NoError(t, runSkipWindowStatus(statusCmd, nil))
	assert.Contains(t, statusBuf.String(), "match_all: true")

	// Clear command
	clearCmd := bldr_cli_cmd_v1.NewSchedulerSkipWindowClearCommandBuilder()
	cli.SetContext(clearCmd, cliCtx)
	var clearBuf bytes.Buffer
	clearCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &clearBuf))
	assert.NoError(t, runSkipWindowClear(clearCmd, nil))

	// Status command again (cleared)
	statusBuf.Reset()
	assert.NoError(t, runSkipWindowStatus(statusCmd, nil))
}

func TestDumpCmd_Cases(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	cmd := NewDumpCmd()
	assert.NotNil(t, cmd)

	// Daemon not running
	cli.SetContext(cmd, cliCtx)
	err := runDump(cliCtx, cmd)
	assert.Error(t, err)
}

func TestShowHistory_ParseTimeString(t *testing.T) {
	// Relative duration
	assert.NotNil(t, parseTimeString("1h", true))
	assert.NotNil(t, parseTimeString("2h", false))

	// Natural language
	assert.NotNil(t, parseTimeString("today", true))
	assert.NotNil(t, parseTimeString("yesterday", true))
	assert.NotNil(t, parseTimeString("this-week", true))
	assert.NotNil(t, parseTimeString("last-week", true))
	assert.NotNil(t, parseTimeString("this-month", true))
	assert.NotNil(t, parseTimeString("last-month", true))
	assert.NotNil(t, parseTimeString("this-year", true))
	assert.NotNil(t, parseTimeString("now", false))

	// Non-duration strings that return nil
	assert.Nil(t, parseTimeString("1d", true))
	assert.Nil(t, parseTimeString("not-a-valid-time-format-xyz", true))
}

func TestSchedulerHealth_UnhealthyFileAndMetrics(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)

	// NewHealthCheckCmd
	hcmd := NewHealthCheckCmd()
	assert.NotNil(t, hcmd)

	// writeDaemonUnhealthyFile
	writeDaemonUnhealthyFile(proj.Root, "test_reason", 3, "daemon is down")
	uPath := daemonUnhealthyPath(proj.Root)
	assert.FileExists(t, uPath)

	// removeDaemonUnhealthyFile
	removeDaemonUnhealthyFile(proj.Root)
	assert.NoFileExists(t, uPath)

	// recordExternalHealthMetric
	storageProvider, err := cli.NewStorageProviderFromFactory(pkgctx.NewSystemContext(), proj.Root)
	require.NoError(t, err)
	secCtx := pkgctx.NewSystemSecurityContext()

	// zero metrics (skipped)
	recordExternalHealthMetric(storageProvider, secCtx, time.Now().UTC(), 0, 0, 0, 0, 0)

	// non-zero metrics
	recordExternalHealthMetric(storageProvider, secCtx, time.Now().UTC(), 1, 2, 3, 4, 100*time.Millisecond)
}

func TestSubmitJob_OptionsAndFlags(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	cmd := NewSubmitCmd()
	cli.SetContext(cmd, cliCtx)
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

	_ = cmd.Flags().Set("title", "Custom Title")
	_ = cmd.Flags().Set("description", "Custom Description")
	_ = cmd.Flags().Set("max-runtime", "120")
	_ = cmd.Flags().Set("workdir", proj.Root)
	_ = cmd.Flags().Set("env", "VAR1=VAL1")
	_ = cmd.Flags().Set("retry", "2")
	_ = cmd.Flags().Set("retry-delay", "10")
	_ = cmd.Flags().Set("callback-completion", "http://localhost:9999/done")
	_ = cmd.Flags().Set("callback-failure", "http://localhost:9999/fail")

	require.NoError(t, submitJob(cliCtx, cmd, []string{"echo", "hello", "world"}))
	assert.Contains(t, buf.String(), "Job submitted:")
	assert.Contains(t, buf.String(), "Retries: 2 (delay: 10 seconds)")
}

func TestTestFailuresList_WithParsedLogs(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// Create callbacks directory with failure log
	cbDir := filepath.Join(proj.Root, paths.ProjectDataDir, "callbacks")
	require.NoError(t, fileutil.MkdirAll(cbDir, 0755))
	cbEntry := map[string]any{
		objects.FieldKeyEventType: schedulerStateFailed,
		"timestamp":               time.Now().UTC().Format(time.RFC3339),
		"test_failures": []any{
			"pkg/sample.TestAlpha",
			"pkg/sample.TestBeta",
		},
	}
	cbData, _ := json.Marshal(cbEntry)
	require.NoError(t, fileutil.WriteFile(filepath.Join(cbDir, "cb.jsonl"), append(cbData, '\n'), 0644))

	// Create scheduler log directory with a failure event
	schedLogDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir, "SCH-TEST")
	require.NoError(t, fileutil.MkdirAll(schedLogDir, 0755))
	schedEntry := map[string]any{
		objects.FieldKeyEventType: schedulerStateFailed,
		objects.FieldKeyCommand:   "go test ./pkg/other/...",
		"stderr":                  "--- FAIL: TestGamma (0.01s)\nFAIL\nFAIL\tgithub.com/zqk-os/zqk/pkg/other\t0.02s\n",
	}
	schedData, _ := json.Marshal(schedEntry)
	require.NoError(t, fileutil.WriteFile(filepath.Join(schedLogDir, "SCH-TEST.log"), append(schedData, '\n'), 0644))

	cmd := NewTestFailuresListCmd()
	cli.SetContext(cmd, cliCtx)
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	_ = cmd.Flags().Set("limit", "1")

	err := listTestFailures(cliCtx, cmd)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "more (use --limit to see more)")
}

func TestTestFailuresConvergence_AgentPromptOutput(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// Create convergence session object in storage
	storageProvider, err := cli.NewStorageProviderFromFactory(pkgctx.NewSystemContext(), proj.Root)
	require.NoError(t, err)
	secCtx := pkgctx.NewSystemSecurityContext()
	sessData := map[string]any{
		objects.FieldKeyID:            "CVS-TEST-CONV-01",
		objects.FieldKeyKind:          "convergence_session",
		objects.FieldKeySchemaVersion: "convergence_session_v1",
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyCreatedAt:     time.Now().UTC().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "system",
		objects.FieldKeyUpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:     "system",
		"session_type":                "fix_loop",
	}
	require.NoError(t, storageProvider.Create(pkgctx.NewSystemContext(), secCtx, sessData))

	// Write health.jsonl in test bundles dir
	bundlesDir := schedpkg.JobLogsTestBundlesDir(proj.Root)
	require.NoError(t, fileutil.MkdirAll(bundlesDir, 0755))
	entry := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"test_bundle_run": map[string]any{
			"bundle_id": "TBD-SAMPLE",
			"status":    "passed",
		},
	}
	data, _ := json.Marshal(entry)
	require.NoError(t, fileutil.WriteFile(filepath.Join(bundlesDir, "health.jsonl"), append(data, '\n'), 0644))

	cmd := &cobra.Command{}
	cli.AddCommonFlags(cmd)
	cmd.Flags().Int("limit", 500, "")
	cmd.Flags().String("session-id", "", "")
	cmd.Flags().String("current-phase", "", "")
	cmd.Flags().String("flow-variant", "", "")
	cmd.Flags().Bool("persist-session", false, "")
	cmd.Flags().Bool("skip-session-context", false, "")
	cmd.Flags().Bool("stamp-tombstone", false, "")
	cmd.Flags().Bool("finalize-debrief", false, "")
	cmd.Flags().String("debrief-notes", "", "")
	cmd.Flags().String("debrief-notes-file", "", "")
	cmd.Flags().Bool("skip-rollup-gates", true, "")
	cmd.Flags().Bool("paste-ide", false, "")
	cmd.Flags().Bool("copy", false, "")
	cmd.Flags().String("attention-mode", "", "")

	cli.SetContext(cmd, cliCtx)
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	_ = cmd.Flags().Set("format", "agent-prompt")
	_ = cmd.Flags().Set("limit", "10")
	_ = cmd.Flags().Set("session-id", "CVS-TEST-CONV-01")
	_ = cmd.Flags().Set("skip-session-context", "false")

	err = runTestFailuresConvergence(cliCtx, cmd)
	assert.NoError(t, err)
}

func TestSubmitJob_ExistingJobUpdateAndCollision(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// 1. Initial submission of a one_time job
	cmd := NewSubmitCmd()
	cli.SetContext(cmd, cliCtx)
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

	jobID := "SCH-TEST-EXIST-01"
	_ = cmd.Flags().Set("id", jobID)
	_ = cmd.Flags().Set("execution-mode", "one_time")
	require.NoError(t, submitJob(cliCtx, cmd, []string{"echo", "initial"}))

	// 2. Second submission of the same one_time job ID -> updates existing job
	buf.Reset()
	_ = cmd.Flags().Set("title", "Updated Title")
	_ = cmd.Flags().Set("description", "Updated Description")
	_ = cmd.Flags().Set("max-runtime", "60")
	_ = cmd.Flags().Set("workdir", proj.Root)
	_ = cmd.Flags().Set("env", "FOO=BAR")
	_ = cmd.Flags().Set("callback-completion", "http://localhost:9999/done")
	_ = cmd.Flags().Set("callback-failure", "http://localhost:9999/fail")
	require.NoError(t, submitJob(cliCtx, cmd, []string{"echo", "updated"}))
	assert.Contains(t, buf.String(), "Reusing existing job")

	// 3. Collision on reusable/recurrent job
	reusableCmd := NewSubmitCmd()
	cli.SetContext(reusableCmd, cliCtx)
	var buf2 bytes.Buffer
	reusableCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf2))
	reusableID := "SCH-TEST-REUSABLE-01"
	_ = reusableCmd.Flags().Set("id", reusableID)
	_ = reusableCmd.Flags().Set("execution-mode", "recurrent")
	require.NoError(t, submitJob(cliCtx, reusableCmd, []string{"echo", "recurrent-initial"}))

	// Second submission of the same reusable job ID -> should error with collision
	err := submitJob(cliCtx, reusableCmd, []string{"echo", "recurrent-second"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestStageCreateBacklogItems_WithFailures(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)

	// Dry run with persistent failures
	payload := &testFailureAnalysisPayload{
		projectRoot:          proj.Root,
		minFailures:          2,
		failureRateThreshold: 0.5,
		createItems:          false,
		persistentFailures: []*TestFailureStats{
			{
				PackagePath:  "./pkg/sample",
				TestName:     "TestFoo",
				FailureCount: 5,
				FailureRate:  0.8,
				FirstSeen:    time.Now().Add(-time.Hour),
				LastSeen:     time.Now(),
			},
		},
	}
	res, err := stageCreateBacklogItems(nil, payload)
	require.NoError(t, err)
	assert.NotNil(t, res)
	assert.Contains(t, payload.outputBuf.String(), "Would create backlog item")

	// Empty persistent failures
	payloadEmpty := &testFailureAnalysisPayload{
		projectRoot:          proj.Root,
		minFailures:          3,
		failureRateThreshold: 0.5,
		createItems:          true,
		persistentFailures:   []*TestFailureStats{},
	}
	resEmpty, err := stageCreateBacklogItems(nil, payloadEmpty)
	require.NoError(t, err)
	assert.NotNil(t, resEmpty)
	assert.Contains(t, payloadEmpty.outputBuf.String(), "No persistent test failures found")
}

func TestSchedulerStop_WithSleepProcessAndForce(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// Spawn a real benign sleep subprocess to act as the daemon
	sleepCmd := testkit.ManagedCommand(t, t.Context(), "sleep", "60")
	require.NoError(t, sleepCmd.Start())
	defer func() {
		if sleepCmd.Process != nil {
			_ = sleepCmd.Process.Kill()
		}
	}()

	schedDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.SchedulerDir)
	require.NoError(t, fileutil.MkdirAll(schedDir, 0755))
	pidFile := filepath.Join(schedDir, "scheduler.pid")
	keepAliveFile := filepath.Join(schedDir, "keep_alive")
	require.NoError(t, fileutil.WriteFile(pidFile, []byte(strconv.Itoa(sleepCmd.Process.Pid)), 0644))
	require.NoError(t, fileutil.WriteFile(keepAliveFile, []byte(time.Now().UTC().Format(time.RFC3339)), 0644))

	stopCmd := NewStopCmd()
	cli.SetContext(stopCmd, cliCtx)
	_ = stopCmd.Flags().Set("force", "true")
	var buf bytes.Buffer
	stopCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

	err := stopScheduler(cliCtx, stopCmd)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "SIGKILL sent to daemon")
}

func TestServiceCmd_GCAndRebind(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// GC
	gcCmd := bldr_cli_cmd_v1.NewSchedulerServiceGcCommandBuilder()
	gcCmd.SetContext(pkgctx.WithSecurityContext(context.Background(), pkgctx.NewSystemSecurityContext()))
	cli.SetContext(gcCmd, cliCtx)
	var gcBuf bytes.Buffer
	gcCmd.SetContext(pkgctx.WithCommandOutputWriter(gcCmd.Context(), &gcBuf))
	assert.NoError(t, runSchedulerServiceGC(gcCmd, nil))

	// Rebind with missing flags -> errors
	rebindCmd := bldr_cli_cmd_v1.NewSchedulerServiceRebindCommandBuilder()
	rebindCmd.SetContext(pkgctx.WithSecurityContext(context.Background(), pkgctx.NewSystemSecurityContext()))
	cli.SetContext(rebindCmd, cliCtx)
	assert.Error(t, runSchedulerServiceRebind(rebindCmd, nil))
}

func TestShowHistory_VerboseAuditQuery(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cliCtx.Verbose = true

	storageProvider, err := cli.NewStorageProviderFromFactory(pkgctx.NewSystemContext(), proj.Root)
	require.NoError(t, err)

	jhc := &JobHistoryContext{
		ProjectRoot:     proj.Root,
		StorageProvider: storageProvider,
		SecCtx:          pkgctx.NewSystemSecurityContext(),
		StorageCtx:      pkgctx.NewStorageContext(),
		Limit:           10,
		Ctx:             cliCtx,
	}

	events, err := queryAuditEvents(jhc)
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestSchedulerStop_WithSleepProcessAndWaitTimeout(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	sleepCmd := testkit.ManagedCommand(t, t.Context(), "sleep", "60")
	require.NoError(t, sleepCmd.Start())
	defer func() {
		if sleepCmd.Process != nil {
			_ = sleepCmd.Process.Kill()
		}
	}()

	schedDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.SchedulerDir)
	require.NoError(t, fileutil.MkdirAll(schedDir, 0755))
	pidFile := filepath.Join(schedDir, "scheduler.pid")
	keepAliveFile := filepath.Join(schedDir, "keep_alive")
	require.NoError(t, fileutil.WriteFile(pidFile, []byte(strconv.Itoa(sleepCmd.Process.Pid)), 0644))
	require.NoError(t, fileutil.WriteFile(keepAliveFile, []byte(time.Now().UTC().Format(time.RFC3339)), 0644))

	stopCmd := NewStopCmd()
	cli.SetContext(stopCmd, cliCtx)
	_ = stopCmd.Flags().Set("wait", "true")
	_ = stopCmd.Flags().Set("max-wait", "10ms")
	var buf bytes.Buffer
	stopCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

	// Attempt wait and signal
	_ = stopScheduler(cliCtx, stopCmd)
}

func TestSchedulerHelpers_GetInstanceAndEnqueue(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// 1. Not running
	_, _, err := getSchedulerInstance(cliCtx)
	assert.Error(t, err)

	// 2. Running in another process
	sleepCmd := testkit.ManagedCommand(t, t.Context(), "sleep", "60")
	require.NoError(t, sleepCmd.Start())
	defer func() {
		if sleepCmd.Process != nil {
			_ = sleepCmd.Process.Kill()
		}
	}()

	schedDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.SchedulerDir)
	require.NoError(t, fileutil.MkdirAll(schedDir, 0755))
	pidFile := filepath.Join(schedDir, "scheduler.pid")
	keepAliveFile := filepath.Join(schedDir, "keep_alive")
	require.NoError(t, fileutil.WriteFile(pidFile, []byte(strconv.Itoa(sleepCmd.Process.Pid)), 0644))
	require.NoError(t, fileutil.WriteFile(keepAliveFile, []byte(time.Now().UTC().Format(time.RFC3339)), 0644))

	_, _, err = getSchedulerInstance(cliCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "running in another process")

	// 3. Enqueue job trigger request (cross process)
	cmd := &cobra.Command{}
	cli.SetContext(cmd, cliCtx)
	err = enqueueJobTriggerRequest(cliCtx, cmd, "JOB-TRIGGER-01", true)
	assert.NoError(t, err)
	err = enqueueJobTriggerRequest(cliCtx, cmd, "JOB-TRIGGER-02", false)
	assert.NoError(t, err)
}

func TestAnalyzeTestFailures_FullPipelineWithLogs(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// Write callback logs with 3 failures for the same test
	cbDir := filepath.Join(proj.Root, paths.ProjectDataDir, "callbacks")
	require.NoError(t, fileutil.MkdirAll(cbDir, 0755))
	for i := 0; i < 3; i++ {
		cbEntry := map[string]any{
			objects.FieldKeyEventType: schedulerStateFailed,
			"timestamp":               time.Now().Add(-time.Duration(i) * time.Hour).UTC().Format(time.RFC3339),
			"test_failures": []any{
				"pkg/storage.TestPersistentFail",
			},
		}
		cbData, _ := json.Marshal(cbEntry)
		require.NoError(t, fileutil.WriteFile(filepath.Join(cbDir, fmt.Sprintf("cb_%d.jsonl", i)), append(cbData, '\n'), 0644))
	}

	cmd := NewTestFailuresAnalyzeCmd()
	cli.SetContext(cmd, cliCtx)
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	_ = cmd.Flags().Set("since", "7d")
	_ = cmd.Flags().Set("min-failures", "2")
	_ = cmd.Flags().Set("failure-rate", "0.1")
	_ = cmd.Flags().Set("create-backlog-items", "false")

	err := analyzeTestFailuresViaPipeline(cliCtx, cmd)
	assert.NoError(t, err)
}

func TestSchedulerCore_ConfigFlagsAndControlTimeout(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// runSchedulerControlWithTimeout success
	assert.NoError(t, runSchedulerControlWithTimeout("test-op", func() error { return nil }))

	// runSchedulerControlWithTimeoutDur timeout
	err := runSchedulerControlWithTimeoutDur("test-timeout", 20*time.Millisecond, func() error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})
	assert.Error(t, err)

	// validateSchedulerProjectRootAndBrand
	profile, err := validateSchedulerProjectRootAndBrand(cliCtx, proj.Root)
	assert.NoError(t, err)
	assert.NotEmpty(t, profile)

	_, err = validateSchedulerProjectRootAndBrand(cliCtx, "")
	assert.Error(t, err)

	// startScheduler with disabled config
	schedDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.SchedulerDir)
	require.NoError(t, fileutil.MkdirAll(schedDir, 0755))
	configYAML := "enabled: false\n"
	require.NoError(t, fileutil.WriteFile(filepath.Join(schedDir, paths.SchedulerConfigFile), []byte(configYAML), 0644))

	cmd := NewStartCmd()
	cli.SetContext(cmd, cliCtx)
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	err = startScheduler(cliCtx, cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "disabled for this project")
}

func TestCoerceFloat64_AllTypes(t *testing.T) {
	v, ok := coerceFloat64(float64(1.23))
	assert.True(t, ok)
	assert.Equal(t, 1.23, v)

	v, ok = coerceFloat64(float32(1.5))
	assert.True(t, ok)
	assert.Equal(t, float64(float32(1.5)), v)

	v, ok = coerceFloat64(int(10))
	assert.True(t, ok)
	assert.Equal(t, float64(10), v)

	v, ok = coerceFloat64(int8(2))
	assert.True(t, ok)
	assert.Equal(t, float64(2), v)

	v, ok = coerceFloat64(int16(3))
	assert.True(t, ok)
	assert.Equal(t, float64(3), v)

	v, ok = coerceFloat64(int32(4))
	assert.True(t, ok)
	assert.Equal(t, float64(4), v)

	v, ok = coerceFloat64(int64(5))
	assert.True(t, ok)
	assert.Equal(t, float64(5), v)

	v, ok = coerceFloat64(uint(6))
	assert.True(t, ok)
	assert.Equal(t, float64(6), v)

	v, ok = coerceFloat64(uint8(7))
	assert.True(t, ok)
	assert.Equal(t, float64(7), v)

	v, ok = coerceFloat64(uint16(8))
	assert.True(t, ok)
	assert.Equal(t, float64(8), v)

	v, ok = coerceFloat64(uint32(9))
	assert.True(t, ok)
	assert.Equal(t, float64(9), v)

	v, ok = coerceFloat64(uint64(10))
	assert.True(t, ok)
	assert.Equal(t, float64(10), v)

	v, ok = coerceFloat64(json.Number("42.5"))
	assert.True(t, ok)
	assert.Equal(t, 42.5, v)

	v, ok = coerceFloat64(json.Number("invalid"))
	assert.False(t, ok)

	v, ok = coerceFloat64("  99.9  ")
	assert.True(t, ok)
	assert.Equal(t, 99.9, v)

	v, ok = coerceFloat64("not-a-number")
	assert.False(t, ok)

	v, ok = coerceFloat64([]string{"unsupported"})
	assert.False(t, ok)
	assert.Equal(t, float64(0), v)
}

func TestBuildLastCompletionsMap_AndShouldCheckMissedTriggers(t *testing.T) {
	events := []jobActivityEvent{
		{JobID: "J1", EventType: "scheduler_job_completed", CreatedAt: "2026-01-01T12:00:00Z"},
		{JobID: "J1", EventType: "scheduler_job_failed", CreatedAt: "2026-01-01T13:00:00Z"},
		{JobID: "J1", EventType: "scheduler_job_completed", CreatedAt: "2026-01-01T11:00:00Z"},
		{JobID: "J2", EventType: "other_event", CreatedAt: "2026-01-01T12:00:00Z"},
		{JobID: "J3", EventType: "scheduler_job_completed", CreatedAt: "invalid-time-format"},
	}
	m := buildLastCompletionsMap(events)
	assert.Len(t, m, 1)
	expectedT, _ := time.Parse(time.RFC3339, "2026-01-01T13:00:00Z")
	assert.Equal(t, expectedT, m["J1"])

	// shouldCheckJobForMissedTriggers
	assert.False(t, shouldCheckJobForMissedTriggers(map[string]any{"trigger_type": "immediate"}))
	assert.False(t, shouldCheckJobForMissedTriggers(map[string]any{"trigger_type": "timer", "enabled": false}))
	assert.False(t, shouldCheckJobForMissedTriggers(map[string]any{"trigger_type": "timer", "enabled": true, "schedule_expression": ""}))
	assert.True(t, shouldCheckJobForMissedTriggers(map[string]any{"trigger_type": "timer", "enabled": true, "schedule_expression": "@hourly"}))
}

func TestSchedulerJobEventTypeCategories(t *testing.T) {
	assert.Empty(t, getSchedulerJobEventTypeCategory("unrelated_event"))
	assert.Empty(t, getSchedulerJobEventTypeCategory(schedulerJobEventPrefix))
	assert.Equal(t, schedulerStateStarted, getSchedulerJobEventTypeCategory(schedulerJobEventPrefix+schedulerStateStarted))
	assert.Equal(t, schedulerStateCompleted, getSchedulerJobEventTypeCategory(schedulerJobEventPrefix+schedulerStateCompleted))
	assert.Equal(t, schedulerStateFailed, getSchedulerJobEventTypeCategory(schedulerJobEventPrefix+schedulerStateFailed))

	assert.True(t, isSchedulerJobStartedEventType(schedulerJobEventPrefix+schedulerStateStarted))
	assert.False(t, isSchedulerJobStartedEventType(schedulerJobEventPrefix+schedulerStateCompleted))
	assert.False(t, isSchedulerJobStartedEventType("unrelated"))

	assert.True(t, isSchedulerJobCompletedEventType(schedulerJobEventPrefix+schedulerStateCompleted))
	assert.False(t, isSchedulerJobCompletedEventType(schedulerJobEventPrefix+schedulerStateFailed))
	assert.False(t, isSchedulerJobCompletedEventType("unrelated"))

	assert.True(t, isSchedulerJobFailedEventType(schedulerJobEventPrefix+schedulerStateFailed))
	assert.False(t, isSchedulerJobFailedEventType(schedulerJobEventPrefix+schedulerStateStarted))
	assert.False(t, isSchedulerJobFailedEventType("unrelated"))
}

func TestConvergenceAgentPrompt_BlockersAndPreamble(t *testing.T) {
	// formatRollupBlockerLine
	assert.Equal(t, "plain text", formatRollupBlockerLine("plain text"))
	assert.Equal(t, `{"extra":"val"}`, formatRollupBlockerLine(map[string]any{"extra": "val"}))
	assert.Equal(t, "`BLOCK_01`", formatRollupBlockerLine(map[string]any{objects.FieldKeyCode: "BLOCK_01"}))
	assert.Equal(t, "`BLOCK_01` — Needs review", formatRollupBlockerLine(map[string]any{objects.FieldKeyCode: "BLOCK_01", "detail": "Needs review"}))

	// appendAgentPromptAttentionPreamble
	var b strings.Builder
	appendAgentPromptAttentionPreamble(&b, "")
	assert.Empty(t, b.String())

	appendAgentPromptAttentionPreamble(&b, "unknown_mode_invalid")
	assert.Empty(t, b.String())

	appendAgentPromptAttentionPreamble(&b, agentPromptAttentionAliasTestNonDirective)
	assert.Contains(t, b.String(), "Attention — test / non-directive")

	var b2 strings.Builder
	appendAgentPromptAttentionPreamble(&b2, agentPromptAttentionAliasInterruptPlumbing)
	assert.Contains(t, b2.String(), "critical interrupt / queue plumbing")

	// agentPromptPhaseRouterNotesContainUnknownVariant
	assert.False(t, agentPromptPhaseRouterNotesContainUnknownVariant(nil))
	assert.False(t, agentPromptPhaseRouterNotesContainUnknownVariant(map[string]any{}))
	assert.False(t, agentPromptPhaseRouterNotesContainUnknownVariant(map[string]any{
		objects.FieldKeyNotes: []any{"All good", "Proceeding normally"},
	}))
	assert.True(t, agentPromptPhaseRouterNotesContainUnknownVariant(map[string]any{
		objects.FieldKeyNotes: []any{"Warning: Unknown flow_variant specified"},
	}))

	// agentPromptIngestBlockedReasonCode
	assert.Equal(t, "missing_session_id", agentPromptIngestBlockedReasonCode(errAgentPromptMissingSession))
	assert.Equal(t, "skip_session_context_incompatible", agentPromptIngestBlockedReasonCode(errAgentPromptSkipSessionContext))
	assert.Equal(t, "paste_ide_requires_darwin", agentPromptIngestBlockedReasonCode(errAgentPromptPasteRequiresDarwin))
	assert.Equal(t, "copy_requires_darwin", agentPromptIngestBlockedReasonCode(errAgentPromptCopyRequiresDarwin))
	assert.Equal(t, "unknown", agentPromptIngestBlockedReasonCode(errors.New("other error")))
}

func TestRequireSchedulerServiceControl_Unauthorized(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	t.Setenv(zqkenv.TestBypassAuth().Name(), "")
	t.Setenv(zqkenv.TestRoot().Name(), "")
	t.Setenv(zqkenv.APIKey().Name(), "")

	err := requireSchedulerServiceControl(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scheduler service control requires")
}

func TestSchedulerRestart_ContextAndForce(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// nil context
	cmd := NewRestartCmd()
	assert.Error(t, runRestart(cmd, nil))

	// daemon not running, force false -> error
	cli.SetContext(cmd, cliCtx)
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	assert.Error(t, runRestart(cmd, nil))
}

func TestSubmitEnvFlagValues_StringSliceFallback(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("env", []string{"FOO=BAR", "BAZ=QUX"}, "")
	values := submitEnvFlagValues(cmd)
	assert.Equal(t, []string{"FOO=BAR", "BAZ=QUX"}, values)
}

func TestNormalizeIDEPasteToken_AllCases(t *testing.T) {
	tok, ok := normalizeIDEPasteToken("focus_chat")
	assert.True(t, ok)
	assert.Equal(t, idePasteTokenCmdY, tok)

	tok, ok = normalizeIDEPasteToken(idePasteTokenCmdY)
	assert.True(t, ok)
	assert.Equal(t, idePasteTokenCmdY, tok)

	tok, ok = normalizeIDEPasteToken("explorer")
	assert.True(t, ok)
	assert.Equal(t, idePasteCmdShiftE, tok)

	tok, ok = normalizeIDEPasteToken("show_explorer")
	assert.True(t, ok)
	assert.Equal(t, idePasteCmdShiftE, tok)

	tok, ok = normalizeIDEPasteToken(idePasteCmdShiftE)
	assert.True(t, ok)
	assert.Equal(t, idePasteCmdShiftE, tok)

	tok, ok = normalizeIDEPasteToken(idePasteTokenEscape)
	assert.True(t, ok)
	assert.Equal(t, idePasteTokenEscape, tok)

	tok, ok = normalizeIDEPasteToken(idePasteTokenCmdL)
	assert.True(t, ok)
	assert.Equal(t, idePasteTokenCmdL, tok)

	tok, ok = normalizeIDEPasteToken(idePasteOptionCmdE)
	assert.True(t, ok)
	assert.Equal(t, idePasteOptionCmdE, tok)

	tok, ok = normalizeIDEPasteToken("not_a_valid_token_xyz")
	assert.False(t, ok)
	assert.Empty(t, tok)

	// parseIDEPastePrefixTokens
	tokens, err := parseIDEPastePrefixTokens("focus_chat,explorer,escape")
	assert.NoError(t, err)
	assert.Len(t, tokens, 3)

	_, err = parseIDEPastePrefixTokens("focus_chat,invalid_token_xyz")
	assert.Error(t, err)
}

func TestReadKnownBundleJobIDs_AndPendingRows(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)

	// No test bundles directory yet
	ids := readKnownBundleJobIDs(proj.Root)
	assert.Empty(t, ids)

	// Create test bundles directory and add a bundle json
	bundleDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.TestBundlesDir)
	require.NoError(t, fileutil.MkdirAll(bundleDir, 0755))
	bundleFile := filepath.Join(bundleDir, "my-bundle.json")
	require.NoError(t, fileutil.WriteFile(bundleFile, []byte(`{"id":"my-bundle"}`), 0644))

	// Also add a non-json file to test branch
	require.NoError(t, fileutil.WriteFile(filepath.Join(bundleDir, "ignore.txt"), []byte("txt"), 0644))

	ids = readKnownBundleJobIDs(proj.Root)
	assert.Contains(t, ids, "SCH-run-bundle-my-bundle")

	rows := buildDefaultPendingRows(proj.Root)
	assert.NotEmpty(t, rows)
	assert.Equal(t, "SCH-run-bundle-my-bundle", rows[0].JobID)
	assert.Equal(t, "pending", rows[0].RunState)
}

func TestResolveSchedulerRootAndLimit_Cases(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	cmd := &cobra.Command{}
	cmd.Flags().Int("limit", 0, "")
	cli.SetContext(cmd, cliCtx)

	// Limit <= 0 -> defaultLimit used
	root, limit, err := resolveSchedulerRootAndLimit(cmd, 50)
	assert.NoError(t, err)
	assert.Equal(t, proj.Root, root)
	assert.Equal(t, 50, limit)

	// Limit > 0
	_ = cmd.Flags().Set("limit", "25")
	root, limit, err = resolveSchedulerRootAndLimit(cmd, 50)
	assert.NoError(t, err)
	assert.Equal(t, 25, limit)
}

func TestResolveSchedulerSkipWindowDir_SuccessAndFailure(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	cmd := &cobra.Command{}
	cli.SetContext(cmd, cliCtx)

	dir, err := resolveSchedulerSkipWindowDir(cmd)
	assert.NoError(t, err)
	assert.Contains(t, dir, paths.SchedulerDir)
}

func TestShowActivity_ContextAndEventMetadataParsing(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	cmd := bldr_cli_cmd_v1.NewSchedulerActivityCommandBuilder()
	cli.AddCommonFlags(cmd)

	// initializeActivityContext with nil ctx
	_, err := initializeActivityContext(nil, cmd)
	assert.Error(t, err)

	// initializeActivityContext with invalid since
	_ = cmd.Flags().Set("since", "invalid-since-string")
	_, err = initializeActivityContext(cliCtx, cmd)
	assert.Error(t, err)

	// initializeActivityContext with stuck flags <= 0 fallback
	_ = cmd.Flags().Set("since", "")
	_ = cmd.Flags().Set("stuck-recent-after", "0s")
	_ = cmd.Flags().Set("stuck-stale-after", "0s")
	actCtx, err := initializeActivityContext(cliCtx, cmd)
	assert.NoError(t, err)
	assert.Equal(t, 2*time.Minute, actCtx.StuckRecentThreshold)
	assert.Equal(t, 30*time.Minute, actCtx.StuckStaleThreshold)

	// parseEventMetadata with various shapes
	event1 := map[string]any{
		objects.FieldKeyDurationSeconds: 1.25,
		GetMetadataErrorField():         "some error",
		GetMetadataSuccessField():       false,
	}
	dur, errMsg, succ := parseEventMetadata(event1)
	assert.Equal(t, "1.25s", dur)
	assert.Equal(t, "some error", errMsg)
	assert.False(t, succ)

	event2 := map[string]any{
		GetMetadataDurationField(): 2.5,
		GetMetadataSuccessField():  true,
	}
	dur2, _, succ2 := parseEventMetadata(event2)
	assert.Equal(t, "2.50s", dur2)
	assert.True(t, succ2)
}

func TestSchedulerDaemon_ConfigureAndStartForRoot(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)

	// configureSchedulerDaemonExec
	dummyCmd := testkit.ManagedCommand(t, t.Context(), "sleep", "1")
	cleanup, err := configureSchedulerDaemonExec(dummyCmd, proj.Root, "test-daemon")
	require.NoError(t, err)
	cleanup()

	// StartSchedulerDaemonForRoot when daemon is already marked running
	schedDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.SchedulerDir)
	require.NoError(t, fileutil.MkdirAll(schedDir, 0755))
	pidFile := filepath.Join(schedDir, "scheduler.pid")
	keepAliveFile := filepath.Join(schedDir, "keep_alive")
	require.NoError(t, fileutil.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0644))
	require.NoError(t, fileutil.WriteFile(keepAliveFile, []byte(time.Now().UTC().Format(time.RFC3339)), 0644))

	assert.NoError(t, StartSchedulerDaemonForRoot(proj.Root))
}

func TestFailuresMerge_Variants(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)

	// Write health.jsonl with failure events where failure string has no dots (parts < 2)
	bundlesDir := schedpkg.JobLogsTestBundlesDir(proj.Root)
	require.NoError(t, fileutil.MkdirAll(bundlesDir, 0755))

	entry1 := map[string]any{
		objects.FieldKeyEventType: schedulerStateFailed,
		"timestamp":               time.Now().UTC().Format(time.RFC3339),
		"package":                 "./pkg/cmd",
		"job_id":                  "JOB-100",
		"test_failures":           []any{"TestSolo"},
	}
	entry2 := map[string]any{
		objects.FieldKeyEventType: schedulerStateFailed,
		"timestamp":               time.Now().UTC().Format(time.RFC3339),
		objects.FieldKeyCommand:   "go test ./pkg/other/sub",
		"test_failures":           []any{"TestCmd"},
	}
	data1, _ := json.Marshal(entry1)
	data2, _ := json.Marshal(entry2)
	content := string(data1) + "\n" + string(data2) + "\n"
	require.NoError(t, fileutil.WriteFile(filepath.Join(bundlesDir, "health.jsonl"), []byte(content), 0644))

	failures := make(map[string][]string)
	var total int
	err := mergeTestBundleEventFailures(proj.Root, time.Now().Add(-time.Hour), "", failures, &total)
	assert.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Contains(t, failures, "./pkg/cmd")
	assert.Contains(t, failures, "./pkg/other/sub")
}

func TestShowHistory_AggregateFromCacheWithFilter(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// Populate global activity cache file
	cacheDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.CacheDir)
	require.NoError(t, fileutil.MkdirAll(cacheDir, 0755))
	cachePath := filepath.Join(cacheDir, "scheduler_activity_cache.json")

	cacheData := map[string]any{
		"metadata": map[string]any{
			"version":      "1.0",
			"updated_at":   time.Now().Format(time.RFC3339),
			"project_root": proj.Root,
		},
		"entries": map[string]any{
			"JOB-ALPHA": map[string]any{
				"job_id":          "JOB-ALPHA",
				"total_completed": 5,
				"total_failed":    1,
				"last_completed":  time.Now().Add(-time.Minute).Format(time.RFC3339),
				"last_failed":     time.Now().Add(-time.Hour).Format(time.RFC3339),
			},
			"JOB-BETA": map[string]any{
				"job_id":          "JOB-BETA",
				"total_completed": 2,
				"total_failed":    3,
				"last_completed":  time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
				"last_failed":     time.Now().Add(-time.Minute).Format(time.RFC3339),
			},
		},
	}
	raw, _ := json.Marshal(cacheData)
	require.NoError(t, fileutil.WriteFile(cachePath, raw, 0644))

	jhc := &JobHistoryContext{
		ProjectRoot: proj.Root,
		Ctx:         cliCtx,
	}

	// 1. With JobIDFilter
	jhc.JobIDFilter = "JOB-ALPHA"
	stats := aggregateJobStatsFromCache(jhc)
	assert.Contains(t, stats, "JOB-ALPHA")
	assert.NotContains(t, stats, "JOB-BETA")

	// 2. Without filter (all entries)
	jhc.JobIDFilter = ""
	allStats := aggregateJobStatsFromCache(jhc)
	assert.Contains(t, allStats, "JOB-ALPHA")
	assert.Contains(t, allStats, "JOB-BETA")
}

func TestStartSchedulerDaemonForRoot_AlreadyRunning(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)

	// Write PID file for current process
	schedDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.SchedulerDir)
	require.NoError(t, fileutil.MkdirAll(schedDir, 0755))
	pidFile := filepath.Join(schedDir, "scheduler.pid")
	require.NoError(t, fileutil.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0644))

	// Should return nil because it's already running
	err := StartSchedulerDaemonForRoot(proj.Root)
	assert.NoError(t, err)
}

func TestRunDump_WithRunningDaemonProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGUSR1 not supported on Windows")
	}
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// Spawn a real benign sleep subprocess to act as daemon
	sleepCmd := testkit.ManagedCommand(t, t.Context(), "sleep", "60")
	require.NoError(t, sleepCmd.Start())
	defer func() {
		if sleepCmd.Process != nil {
			_ = sleepCmd.Process.Kill()
		}
	}()

	schedDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.SchedulerDir)
	require.NoError(t, fileutil.MkdirAll(schedDir, 0755))
	pidFile := filepath.Join(schedDir, "scheduler.pid")
	keepAliveFile := filepath.Join(schedDir, "keep_alive")
	require.NoError(t, fileutil.WriteFile(pidFile, []byte(strconv.Itoa(sleepCmd.Process.Pid)), 0644))
	require.NoError(t, fileutil.WriteFile(keepAliveFile, []byte(time.Now().UTC().Format(time.RFC3339)), 0644))

	cmd := NewDumpCmd()
	cli.SetContext(cmd, cliCtx)
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

	err := runDump(cliCtx, cmd)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Process dump requested")
}

func TestAgentPromptStages_CommitAndFinalize(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	t.Setenv(zqkenv.DiagnosticsDir().Name(), filepath.Join(proj.Root, paths.ProjectDataDir, "diagnostics"))
	pctx := &pipeline.Context{
		Ctx:     context.Background(),
		Outcome: make(map[string]any),
	}

	payloadDefault := &agentPromptPipelinePayload{
		projectRoot: proj.Root,
		markdown:    "# Test Convergence",
	}
	res, err := agentPromptStageCommit(pctx, payloadDefault)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, agentPromptDeliverNone, pctx.Outcome[outcomeAgentPromptDeliverMode])

	_, err = agentPromptStageFinalize(pctx, payloadDefault)
	assert.NoError(t, err)
	assert.True(t, pctx.Outcome[outcomeAgentPromptComplete].(bool))

	// pasteIDE delivery path using mock antigravity transcript (cross-platform compatible on all OSes)
	brainDir := t.TempDir()
	mockTranscript := filepath.Join(brainDir, "conv-cross-os", ".system_generated", "logs", "transcript.jsonl")
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(mockTranscript), 0755))
	require.NoError(t, fileutil.WriteFile(mockTranscript, []byte("{}\n"), 0644))
	t.Setenv(zqkenv.AGTranscriptPath().Name(), mockTranscript)

	pctxPaste := &pipeline.Context{
		Ctx:     context.Background(),
		Outcome: make(map[string]any),
	}
	payloadPaste := &agentPromptPipelinePayload{
		projectRoot: proj.Root,
		markdown:    "# Test Paste",
		pasteIDE:    true,
	}
	res, err = agentPromptStageCommit(pctxPaste, payloadPaste)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, agentPromptDeliverPasteIDE, pctxPaste.Outcome[outcomeAgentPromptDeliverMode])

	_, err = agentPromptStageFinalize(pctxPaste, payloadPaste)
	assert.NoError(t, err)
	assert.True(t, pctxPaste.Outcome[outcomeAgentPromptComplete].(bool))

	// On non-Darwin platforms, attempting pasteIDE delivery without an Antigravity transcript
	// falls back to the Darwin-only clipboard-paste adapter, which fails closed.
	if runtime.GOOS != "darwin" {
		t.Setenv(zqkenv.AGTranscriptPath().Name(), "")
		pctxPasteNonDarwin := &pipeline.Context{
			Ctx:     context.Background(),
			Outcome: make(map[string]any),
		}
		_, errPasteNonDarwin := agentPromptStageCommit(pctxPasteNonDarwin, payloadPaste)
		assert.Error(t, errPasteNonDarwin)
		assert.Contains(t, errPasteNonDarwin.Error(), "requires darwin")
	}
}

func TestBundleProgress_TableAndEvents(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)

	// Case 1: no diagnostics file yet (returns default pending rows)
	cmd := NewSchedulerBundleProgressCmd()
	cli.SetContext(cmd, cliCtx)
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	err := runBundleProgress(cmd, nil)
	assert.NoError(t, err)

	// Case 2: progress file with events
	progPath := schedpkg.TestBundlesProgressFilePath(proj.Root)
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(progPath), 0755))

	lines := []string{
		`{"job_id":"SCH-1","event_type":"started","timestamp":"2026-01-01T12:00:00Z"}`,
		`{"job_id":"SCH-1","event_type":"passed","timestamp":"2026-01-01T12:05:00Z"}`,
		`{"job_id":"SCH-2","event_type":"started","timestamp":"2026-01-01T12:00:00Z"}`,
		`{"job_id":"SCH-2","event_type":"failed","timestamp":"2026-01-01T12:03:00Z"}`,
		`{"job_id":"SCH-3","event_type":"started","timestamp":"2026-01-01T12:00:00Z"}`,
		`{"job_id":"SCH-4","event_type":"started","timestamp":"invalid-ts"}`,
		`{"job_id":""}`,
		`invalid json`,
		``,
	}
	require.NoError(t, fileutil.WriteFile(progPath, []byte(strings.Join(lines, "\n")), 0644))

	buf.Reset()
	err = runBundleProgress(cmd, nil)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Bundle Progress Matrix")
	assert.Contains(t, buf.String(), "SCH-1")
	assert.Contains(t, buf.String(), "SCH-2")
}

func TestConvergenceAgentPrompt_KeystrokeLogAndBucketing(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)

	// ideKeystrokeDetailPbcopyBegin
	detail := ideKeystrokeDetailPbcopyBegin(128, "/path/to/log")
	assert.Contains(t, detail, "128")
	assert.Contains(t, detail, "/path/to/log")

	// appendAgentPromptIDEKeystrokeLog with and without logger
	appendAgentPromptIDEKeystrokeLog(nil, proj.Root, "TEST_EVT", "some details")
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	appendAgentPromptIDEKeystrokeLog(logger, proj.Root, "TEST_EVT2", "more details")

	logPath := agentPromptIDEKeystrokeLogPath(proj.Root)
	assert.FileExists(t, logPath)

	// agentPromptMetricsBucketing
	b := agentPromptMetricsBucketing{}
	pctx := &pipeline.Context{
		Outcome: map[string]any{
			"attention_mode": "custom_mode",
		},
	}
	m := b.Buckets(pctx, "test_pipe", "test_stage")
	assert.Equal(t, "custom_mode", m["attention_mode"])

	// Empty context fallback
	mDefault := b.Buckets(nil, "test_pipe", "test_stage")
	assert.Equal(t, "default", mDefault["attention_mode"])
}

func TestShowActivity_CompactHelpersAndLastRunTime(t *testing.T) {
	// Compact width toggles
	activityOutputCompact = true
	assert.Equal(t, 100, effTableWidth())
	assert.Equal(t, 24, effColJobID())
	assert.Equal(t, 16, effMissedColSchedule())

	activityOutputCompact = false
	assert.Equal(t, activityTableWidth, effTableWidth())
	assert.Equal(t, activityColJobID, effColJobID())
	assert.Equal(t, missedColSchedule, effMissedColSchedule())

	// formatLastRunTime with ISO format fallback
	job := map[string]any{
		"last_run_at": "2026-05-01T12:00:00Z",
	}
	lastRuns := map[string]string{
		"JOB-1": "2026-05-01T14:00:00Z",
	}
	formatted := formatLastRunTime("JOB-1", job, lastRuns)
	assert.Contains(t, formatted, "2026-05-01")

	// formatLastRunTime with empty values
	emptyFormatted := formatLastRunTime("JOB-NONE", map[string]any{}, map[string]string{})
	assert.Equal(t, "Never", emptyFormatted)

	// outputDaemonDownWarningToBuffer
	var buf strings.Builder
	outputDaemonDownWarningToBuffer(&buf, true)
	assert.Contains(t, buf.String(), "CRITICAL: Scheduler daemon is not running")

	// outputBusynessToBuffer
	buf.Reset()
	outputBusynessToBuffer(&buf, SchedulerBusyness{Executing: 2, PendingInQueue: 1, CompletedSuccess: 5, CompletedFailed: 1})
	assert.Contains(t, buf.String(), "Scheduler busyness:")

	// outputActivityDataFreshnessToBuffer
	buf.Reset()
	outputActivityDataFreshnessToBuffer(&buf, nil, time.Now())
	assert.Contains(t, buf.String(), "Activity data window")

	// buildActivityDataFreshness
	freshness := buildActivityDataFreshness(nil, time.Now())
	assert.NotNil(t, freshness)
	assert.Contains(t, freshness.Note, "No scheduler job events")

	// stuckJobAssessmentParts
	code, detail := stuckJobAssessmentParts(30*time.Minute, 10*time.Minute, 60*time.Minute)
	assert.NotEmpty(t, code)
	assert.NotEmpty(t, detail)

	// formatActivityCompactUTC
	assert.NotEmpty(t, formatActivityCompactUTC(time.Time{}, time.Now()))
	assert.NotEmpty(t, formatActivityCompactUTC(time.Now(), time.Now()))

	// Legends
	buf.Reset()
	outputMissedTriggersLegendToBuffer(&buf)
	assert.Contains(t, buf.String(), "HF high-freq grace")

	buf.Reset()
	outputStuckJobsLegendToBuffer(&buf, 10*time.Minute, 60*time.Minute)
	assert.Contains(t, buf.String(), "REC within")

	buf.Reset()
	outputCronLegendToBuffer(&buf)
	assert.Contains(t, buf.String(), "Cron schedule")
}

func TestAuditEvents_FieldNamesAndExtraction(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	t.Setenv(zqkenv.DiagnosticsDir().Name(), filepath.Join(proj.Root, paths.ProjectDataDir, "diagnostics"))

	// Unknown field should return fallback
	assert.Equal(t, "unknown_field", getAuditEventFieldName("unknown_field"))

	// Known fields
	assert.Equal(t, objects.FieldKeyTargetID, getAuditEventFieldName("target_id"))
	assert.Equal(t, objects.FieldKeyTargetKind, getAuditEventFieldName("target_kind"))
	assert.Equal(t, objects.FieldKeyEventType, getAuditEventFieldName("event_type"))
	assert.Equal(t, objects.FieldKeyCreatedAt, getAuditEventFieldName("created_at"))
	assert.Equal(t, objects.FieldKeyMetadata, getAuditEventFieldName("metadata"))

	// runSubmit with nil context or empty args returns error immediately
	cmd := NewSubmitCmd()
	_ = cmd.Flags().Set("project-root", proj.Root)
	_ = cmd.Flags().Set("repo-root", proj.Root)
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)
	assert.Error(t, runSubmit(cmd, nil))
	assert.Error(t, runSubmit(cmd, []string{}))

	// verify runSubmit without context returns error fail-closed
	cmdNoCtx := NewSubmitCmd()
	_ = cmdNoCtx.Flags().Set("project-root", proj.Root)
	assert.Error(t, runSubmit(cmdNoCtx, nil))
}

func TestServiceCmd_AllOperations(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestBypassAuth().Name(), "true")

	cmd := NewServiceCmd()
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	// Subcommands with uninstalled/non-existent root
	_ = runSchedulerServiceInstall(cmd, nil)
	_ = runSchedulerServiceStart(cmd, nil)
	_ = runSchedulerServiceStop(cmd, nil)
	_ = runSchedulerServiceRestart(cmd, nil)
	_ = runSchedulerServiceUninstall(cmd, nil)
	_ = runSchedulerServiceGC(cmd, nil)
	_ = runSchedulerServiceRebind(cmd, nil)
	_ = runSchedulerServiceStatus(cmd, nil)
}

func TestRestart_EdgeCases(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := NewRestartCmd()
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	// Missing context
	cmdNoCtx := NewRestartCmd()
	assert.Error(t, runRestart(cmdNoCtx, nil))

	// Force flag set
	_ = cmd.Flags().Set("force", "true")
	_ = runRestart(cmd, nil)
}

func TestSkipWindow_Helpers(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := NewSkipWindowCmd()
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	dir, err := resolveSchedulerSkipWindowDir(cmd)
	assert.NoError(t, err)
	assert.NotEmpty(t, dir)

	cmdNoCtx := &cobra.Command{}
	_, _ = resolveSchedulerSkipWindowDir(cmdNoCtx)

	_ = runSkipWindowClear(cmd, nil)
}

func TestWaitResult_Outputs(t *testing.T) {
	cmd := NewWaitCmd()

	resWithScope := &WaitResult{
		JobID:       "JOB-TEST-WAIT-01",
		Status:      "completed",
		Duration:    "120ms",
		DurationMs:  120,
		CompletedAt: time.Date(2026, 10, 15, 7, 0, 0, 0, time.UTC),
		Scope: map[string]string{
			"env": "prod",
			"app": "backend",
		},
	}

	assert.NoError(t, outputWaitResultTable(cmd, resWithScope))

	// Empty scope and zero completed at
	resMinimal := &WaitResult{
		JobID:    "JOB-TEST-WAIT-02",
		Status:   "running",
		Duration: "10ms",
	}
	assert.NoError(t, outputWaitResultTable(cmd, resMinimal))

	// JSON format
	_ = cmd.Flags().Set("format", "json")
	assert.NoError(t, outputWaitResult(cmd, resWithScope))

	// YAML format
	_ = cmd.Flags().Set("format", "yaml")
	assert.NoError(t, outputWaitResult(cmd, resWithScope))

	// Table format
	_ = cmd.Flags().Set("format", "table")
	assert.NoError(t, outputWaitResult(cmd, resWithScope))

	// runWait with empty args
	assert.Equal(t, ErrJobNotFound, runWait(cmd, nil))
	assert.Equal(t, ErrJobNotFound, runWait(cmd, []string{""}))
}

func TestSchedulerStatus_TableFormats(t *testing.T) {
	cmd := NewStatusCmd()

	// Missing running key
	dataInvalid := map[string]any{"invalid": 123}
	assert.NoError(t, outputSchedulerStatusTable(cmd, dataInvalid))

	// Not running without project root
	dataNotRunningNoRoot := map[string]any{
		schedulerStatusRunning: false,
	}
	assert.NoError(t, outputSchedulerStatusTable(cmd, dataNotRunningNoRoot))

	// Not running with project root
	dataNotRunningRoot := map[string]any{
		schedulerStatusRunning:    false,
		schedulerFieldProjectRoot: "/tmp/sample-project",
	}
	assert.NoError(t, outputSchedulerStatusTable(cmd, dataNotRunningRoot))

	// Running in process
	dataRunningInProcess := map[string]any{
		schedulerStatusRunning:  true,
		schedulerFieldInProcess: true,
	}
	assert.NoError(t, outputSchedulerStatusTable(cmd, dataRunningInProcess))

	// Running with PID
	dataRunningPID := map[string]any{
		schedulerStatusRunning: true,
		schedulerFieldPID:      4242,
	}
	assert.NoError(t, outputSchedulerStatusTable(cmd, dataRunningPID))

	// Running with no PID or in-process info
	dataRunningPlain := map[string]any{
		schedulerStatusRunning: true,
	}
	assert.NoError(t, outputSchedulerStatusTable(cmd, dataRunningPlain))

	// outputSchedulerStatus formats
	_ = cmd.Flags().Set("format", "json")
	assert.NoError(t, outputSchedulerStatus(cmd, dataRunningPID))

	_ = cmd.Flags().Set("format", "yaml")
	assert.NoError(t, outputSchedulerStatus(cmd, dataRunningPID))

	_ = cmd.Flags().Set("format", "table")
	assert.NoError(t, outputSchedulerStatus(cmd, dataRunningPID))
}

func TestSchedulerConfig_MutuallyExclusiveFlags(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := NewConfigCmd()
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cli.SetContext(cmd, cliCtx)

	_ = cmd.Flags().Set("jobs-paused", "true")
	_ = cmd.Flags().Set("no-jobs-paused", "true")

	err := runConfig(cmd, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot set both")
}

func TestFlagDuration_Comprehensive(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Duration("duration-flag", 5*time.Second, "")
	cmd.Flags().String("string-flag", "10m", "")
	cmd.Flags().String("invalid-flag", "invalid-duration", "")
	cmd.Flags().String("empty-flag", "", "")

	// Fallback when flag missing
	assert.Equal(t, 2*time.Second, flagDuration(cmd, "non-existent", 2*time.Second))

	// Duration flag
	assert.Equal(t, 5*time.Second, flagDuration(cmd, "duration-flag", 1*time.Second))

	// Valid string flag
	assert.Equal(t, 10*time.Minute, flagDuration(cmd, "string-flag", 1*time.Second))

	// Invalid string flag
	assert.Equal(t, 3*time.Second, flagDuration(cmd, "invalid-flag", 3*time.Second))

	// Empty string flag
	assert.Equal(t, 4*time.Second, flagDuration(cmd, "empty-flag", 4*time.Second))
}

func TestDumpCmd_NotRunningDaemon(t *testing.T) {
	cmd := NewDumpCmd()
	err := cmd.RunE(cmd, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scheduler daemon is not running")
}
