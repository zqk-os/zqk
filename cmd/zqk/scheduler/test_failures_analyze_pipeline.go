package scheduler

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/gotestparse"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/storage/id_generation"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const pipelineKindTestFailureAnalysis = "test_failures_analyze"

type testFailureAnalysisPayload struct {
	cmd                  *cobra.Command
	cliCtx               *cli.Context
	projectRoot          string
	cutoffTime           time.Time
	minFailures          int
	failureRateThreshold float64
	createItems          bool

	// intermediate data
	logLines           []string
	failureStats       map[string]*TestFailureStats
	persistentFailures []*TestFailureStats

	outputBuf strings.Builder
}

func analyzeTestFailuresViaPipeline(cliCtx *cli.Context, cmd *cobra.Command) error {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	pctx := &pipeline.Context{
		Ctx:     pkgctx.NewSystemContext(),
		Outcome: make(map[string]any),
	}

	pl := pipeline.NewBuilder(pipelineKindTestFailureAnalysis, logger).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage("INGEST", stageIngestTestFailures).
		AddStage("COLLECT_LOGS", stageCollectLogs).
		AddStage("ANALYZE_FAILURES", stageAnalyzeFailures).
		AddStage("CREATE_BACKLOG_ITEMS", stageCreateBacklogItems).
		AddStage("FINALIZE", stageFinalizeOutput).
		Build()

	payload := &testFailureAnalysisPayload{
		cmd:          cmd,
		cliCtx:       cliCtx,
		failureStats: make(map[string]*TestFailureStats),
	}

	_, err := pl.Run(pctx, payload)
	if err != nil {
		return err
	}
	return nil
}

func stageIngestTestFailures(stageCtx *pipeline.Context, p any) (any, error) {
	stageCtx.Outcome[pipeline.OutcomeKeyIngestStageStarted] = true
	payload, ok := nildecode.DecodeNonNilPayload[*testFailureAnalysisPayload](p)
	if !ok {
		return nil, errfmt.Errorf("expected *testFailureAnalysisPayload")
	}

	projectRoot := payload.cliCtx.ProjectRoot
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
		if projectRoot == emptyValue {
			return nil, errfmt.Errorf("project root not found")
		}
	}
	payload.projectRoot = projectRoot

	sinceStr, _ := payload.cmd.Flags().GetString("since")
	payload.minFailures, _ = payload.cmd.Flags().GetInt("min-failures")
	payload.failureRateThreshold, _ = payload.cmd.Flags().GetFloat64("failure-rate")
	payload.createItems, _ = payload.cmd.Flags().GetBool("create-backlog-items")

	sinceStr = strings.TrimSpace(sinceStr)
	var since time.Duration
	if strings.HasSuffix(sinceStr, "d") {
		daysStr := strings.TrimSuffix(sinceStr, "d")
		var days int
		if _, err := fmt.Sscanf(daysStr, "%d", &days); err != nil {
			return nil, errfmt.Errorf("invalid duration %q: %w", sinceStr, err)
		}
		since = time.Duration(days) * 24 * time.Hour
	} else {
		var err error
		since, err = time.ParseDuration(sinceStr)
		if err != nil {
			return nil, errfmt.Errorf("invalid duration %q: %w", sinceStr, err)
		}
	}
	payload.cutoffTime = time.Now().Add(-since)

	return payload, nil
}

func stageCollectLogs(stageCtx *pipeline.Context, p any) (any, error) {
	payload := p.(*testFailureAnalysisPayload)

	// We collect raw file content into logLines for parsing
	callbackLogsDir := filepath.Join(payload.projectRoot, paths.ProjectDataDir, "callbacks")
	if _, err := fileutil.Stat(callbackLogsDir); err == nil {
		err = filepath.Walk(callbackLogsDir, func(path string, info fileutil.FileInfo, err error) error {
			if err != nil || info.ModTime().Before(payload.cutoffTime) {
				return nil //nolint:nilerr // skip unreadable or outdated callback logs
			}
			if !strings.HasSuffix(path, ".log") && !strings.HasSuffix(path, ".jsonl") {
				return nil
			}
			data, err := fileutil.ReadFile(path)
			if err != nil {
				return nil //nolint:nilerr // best-effort log parsing
			}
			for line := range strings.SplitSeq(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line != emptyValue {
					payload.logLines = append(payload.logLines, line)
				}
			}
			return nil
		})
		if err != nil {
			return nil, errfmt.Newf("failed to scan callback logs").Wrap(err)
		}
	}

	schedulerLogsDir := filepath.Join(payload.projectRoot, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir)
	if _, err := fileutil.Stat(schedulerLogsDir); err == nil {
		err = filepath.Walk(schedulerLogsDir, func(path string, info fileutil.FileInfo, err error) error {
			if err != nil || info.ModTime().Before(payload.cutoffTime) {
				return nil //nolint:nilerr // skip unreadable or outdated scheduler logs
			}
			if !strings.HasSuffix(path, ".log") {
				return nil
			}
			data, err := fileutil.ReadFile(path)
			if err != nil {
				return nil //nolint:nilerr // best-effort log parsing
			}
			for line := range strings.SplitSeq(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line != emptyValue {
					payload.logLines = append(payload.logLines, line)
				}
			}
			return nil
		})
		if err != nil {
			return nil, errfmt.Newf("failed to scan scheduler logs").Wrap(err)
		}
	}

	stageCtx.Outcome["lines_collected"] = len(payload.logLines)
	return payload, nil
}

func stageAnalyzeFailures(stageCtx *pipeline.Context, p any) (any, error) {
	payload := p.(*testFailureAnalysisPayload)

	for _, line := range payload.logLines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		eventType, _ := entry[objects.FieldKeyEventType].(string)
		if eventType != schedulerStateFailed && eventType != schedulerStateError {
			continue
		}

		timestampStr, _ := entry["timestamp"].(string)
		var timestamp time.Time
		if timestampStr != emptyValue {
			if t, err := time.Parse(time.RFC3339, timestampStr); err == nil {
				timestamp = t
			}
		}
		if timestamp.IsZero() {
			timestamp = time.Now() // Fallback
		}

		// Handle callback logs
		if testFailures, ok := entry["test_failures"].([]any); ok {
			for _, failure := range testFailures {
				failureStr, ok := failure.(string)
				if !ok {
					continue
				}
				parts := strings.Split(failureStr, ".")
				if len(parts) < 2 {
					continue
				}
				pkg := strings.Join(parts[:len(parts)-1], ".")
				testName := parts[len(parts)-1]
				pkg = strings.TrimPrefix(pkg, "github.com/zqk-os/zqk/")
				if !strings.HasPrefix(pkg, "./") {
					pkg = "./" + pkg
				}
				key := fmt.Sprintf("%s.%s", pkg, testName)
				if stats, exists := payload.failureStats[key]; exists {
					stats.FailureCount++
					if timestamp.Before(stats.FirstSeen) {
						stats.FirstSeen = timestamp
					}
					if timestamp.After(stats.LastSeen) {
						stats.LastSeen = timestamp
					}
				} else {
					payload.failureStats[key] = &TestFailureStats{
						TestName:     testName,
						PackagePath:  pkg,
						FailureCount: 1,
						FirstSeen:    timestamp,
						LastSeen:     timestamp,
					}
				}
			}
		}

		// Handle scheduler execution logs
		if command, ok := entry[objects.FieldKeyCommand].(string); ok && strings.Contains(command, "go test") {
			if stderr, ok := entry["stderr"].(string); ok && stderr != emptyValue {
				if summary, err := gotestparse.ParseGoTestOutput(stderr); err == nil && summary.FailedCount > 0 {
					for _, test := range summary.FailedTestList {
						pkg := test.PackagePath
						testName := test.TestName
						pkg = strings.TrimPrefix(pkg, "github.com/zqk-os/zqk/")
						if !strings.HasPrefix(pkg, "./") {
							pkg = "./" + pkg
						}
						key := fmt.Sprintf("%s.%s", pkg, testName)
						if stats, exists := payload.failureStats[key]; exists {
							stats.FailureCount++
							if timestamp.Before(stats.FirstSeen) {
								stats.FirstSeen = timestamp
							}
							if timestamp.After(stats.LastSeen) {
								stats.LastSeen = timestamp
							}
							if test.Error != emptyValue {
								stats.ErrorMessages = append(stats.ErrorMessages, test.Error)
							}
						} else {
							errorMsgs := []string{}
							if test.Error != emptyValue {
								errorMsgs = append(errorMsgs, test.Error)
							}
							payload.failureStats[key] = &TestFailureStats{
								TestName:      testName,
								PackagePath:   pkg,
								FailureCount:  1,
								FirstSeen:     timestamp,
								LastSeen:      timestamp,
								ErrorMessages: errorMsgs,
							}
						}
					}
				}
			}
		}
	}

	for _, stats := range payload.failureStats {
		if stats.FailureCount >= payload.minFailures {
			timeSpan := stats.LastSeen.Sub(stats.FirstSeen)
			if timeSpan > 0 {
				estimatedRuns := int(timeSpan.Hours()/24) + stats.FailureCount
				stats.TotalRuns = estimatedRuns
				stats.FailureRate = float64(stats.FailureCount) / float64(estimatedRuns)
			} else {
				stats.TotalRuns = stats.FailureCount
				stats.FailureRate = 1.0
			}
		}
	}

	for _, stats := range payload.failureStats {
		if stats.FailureCount >= payload.minFailures && stats.FailureRate >= payload.failureRateThreshold {
			payload.persistentFailures = append(payload.persistentFailures, stats)
		}
	}

	sort.Slice(payload.persistentFailures, func(i, j int) bool {
		return payload.persistentFailures[i].FailureCount > payload.persistentFailures[j].FailureCount
	})

	stageCtx.Outcome["persistent_failures_count"] = len(payload.persistentFailures)
	return payload, nil
}

func stageCreateBacklogItems(stageCtx *pipeline.Context, p any) (any, error) {
	payload := p.(*testFailureAnalysisPayload)

	if len(payload.persistentFailures) == 0 {
		payload.outputBuf.WriteString("✅ No persistent test failures found that meet criteria.\n")
		fmt.Fprintf(&payload.outputBuf, "   Criteria: ≥%d failures, ≥%.0f%% failure rate\n", payload.minFailures, payload.failureRateThreshold*100)
		return payload, nil
	}

	fmt.Fprintf(&payload.outputBuf, "📊 Found %d persistent test failure(s) meeting criteria:\n\n", len(payload.persistentFailures))
	fmt.Fprintf(&payload.outputBuf, "   Criteria: ≥%d failures, ≥%.0f%% failure rate\n\n", payload.minFailures, payload.failureRateThreshold*100)

	ctx := pkgctx.NewSystemContext()
	storageFactory, err := storagepkg.NewStorageFactory(ctx, payload.projectRoot)
	if err != nil {
		return nil, errfmt.Newf("failed to create storage factory").Wrap(err)
	}
	storageProvider := storageFactory.GetStorage()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()

	idValidator := validation.NewIDValidator(payload.projectRoot)
	if err := idValidator.LoadPatterns(); err != nil {
		return nil, errfmt.Newf("failed to load ID patterns").Wrap(err)
	}
	idGenerator := id_generation.NewGenerator(idValidator, payload.projectRoot)

	createdCount := 0
	for _, stats := range payload.persistentFailures {
		filter := storagepkg.ListFilter{
			Kind: objects.KindBacklogItem,
			Filters: map[string]any{
				objects.FieldKeyTitle: map[string]any{
					"$contains": stats.TestName,
				},
			},
			Limit: 10,
		}
		existing, err := storageProvider.List(ctx, secCtx, pkgctx.NewStorageContext(), filter)
		if err == nil && len(existing.Objects) > 0 {
			found := false
			for _, obj := range existing.Objects {
				title, _ := obj[objects.FieldKeyTitle].(string)
				if strings.Contains(title, stats.TestName) && strings.Contains(title, stats.PackagePath) {
					fmt.Fprintf(&payload.outputBuf, "⏭️  Skipping %s.%s (backlog item already exists: %s)\n",
						stats.PackagePath, stats.TestName, obj[objects.FieldKeyID])
					found = true
					break
				}
			}
			if found {
				continue
			}
		}

		strategyConfig := id_generation.StrategyConfig{Strategy: "sequential"}
		backlogID, err := idGenerator.GenerateNextID(ctx, objects.KindBacklogItem, strategyConfig)
		if err != nil {
			schedpkg.SLog(logger).Warn("Failed to generate backlog item ID").TestName(stats.TestName).WithError(err).Log()
			continue
		}

		description := fmt.Sprintf(`Test %s.%s is failing persistently.

**Failure Statistics:**
- Failure Count: %d
- Estimated Failure Rate: %.1f%%
- First Seen: %s
- Last Seen: %s
- Time Span: %s

**Package:** %s
**Test:** %s

This test has been failing consistently and requires investigation and fixes.`,
			stats.PackagePath, stats.TestName,
			stats.FailureCount,
			stats.FailureRate*100,
			stats.FirstSeen.Format(time.RFC3339),
			stats.LastSeen.Format(time.RFC3339),
			stats.LastSeen.Sub(stats.FirstSeen).String(),
			stats.PackagePath,
			stats.TestName)

		if len(stats.ErrorMessages) > 0 {
			description += "\n\n**Recent Error Messages:**\n"
			uniqueErrors := make(map[string]bool)
			for _, errMsg := range stats.ErrorMessages {
				errorPreview := errMsg
				if len(errorPreview) > 200 {
					errorPreview = errorPreview[:200] + "..."
				}
				if !uniqueErrors[errorPreview] {
					description += fmt.Sprintf("- %s\n", errorPreview)
					uniqueErrors[errorPreview] = true
					if len(uniqueErrors) >= 3 {
						break
					}
				}
			}
		}

		backlogItem := map[string]any{
			objects.FieldKeyID:            backlogID,
			objects.FieldKeyKind:          objects.KindBacklogItem,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeyCategory:      "Quality Assurance",
			objects.FieldKeyTitle:         fmt.Sprintf("Fix Persistent Test Failure: %s.%s", stats.PackagePath, stats.TestName),
			objects.FieldKeyDescription:   description,
			objects.FieldKeyTags:          []string{"test-failure", "quality", "automated"},
			objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyCreatedBy:     pkgctx.SystemAccountID,
			objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyUpdatedBy:     pkgctx.SystemAccountID,
			objects.FieldKeyOriginProject: "zqk",
			objects.FieldKeyOriginSystem:  "zqk",
			objects.FieldKeySourceType:    "internal",
		}

		if payload.createItems {
			if err := storageProvider.Create(ctx, secCtx, backlogItem); err != nil {
				schedpkg.SLog(logger).Warn("Failed to create backlog item").TestName(stats.TestName).ObjectID(backlogID).WithError(err).Log()
				fmt.Fprintf(&payload.outputBuf, "❌ Failed to create backlog item for %s.%s: %v\n", stats.PackagePath, stats.TestName, err)
				continue
			}
			fmt.Fprintf(&payload.outputBuf, "✅ Created backlog item %s for %s.%s (%d failures, %.1f%% rate)\n", backlogID, stats.PackagePath, stats.TestName, stats.FailureCount, stats.FailureRate*100)
			createdCount++
		} else {
			fmt.Fprintf(&payload.outputBuf, "📝 Would create backlog item for %s.%s (%d failures, %.1f%% rate)\n", stats.PackagePath, stats.TestName, stats.FailureCount, stats.FailureRate*100)
		}
	}

	if !payload.createItems {
		payload.outputBuf.WriteString("\n💡 This was a dry-run. Use --create-backlog-items to actually create backlog items.\n")
	} else {
		fmt.Fprintf(&payload.outputBuf, "\n✅ Created %d backlog item(s) for persistent test failures.\n", createdCount)
	}

	return payload, nil
}

func stageFinalizeOutput(stageCtx *pipeline.Context, p any) (any, error) {
	payload := p.(*testFailureAnalysisPayload)
	return payload, cli.WriteOutput(payload.cmd, []byte(payload.outputBuf.String()))
}
