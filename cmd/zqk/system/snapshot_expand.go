package system

import (
	"context"
	"maps"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewSnapshotExpandCmd creates the snapshot-expand command
func NewSnapshotExpandCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Expand a compressed snapshot file",
		"Expand a compressed snapshot (.csnap) file back to original objects.",
		"",
		"This command reads a compressed snapshot file and expands it, optionally",
		"writing the expanded objects to a directory or outputting them.",
	).
		AddExample("Expand and show object count", "%s system snapshot-expand test-scenarios/my-snapshot/snapshot.csnap --verbose").
		AddExample("Expand and write to directory", "%s system snapshot-expand snapshot.csnap --output-dir expanded/").
		AddExample("Expand and show first object", "%s system snapshot-expand snapshot.csnap --show-object 0").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemSnapshotExpandCommandBuilder(), &cobra.Command{
		Use:  "snapshot-expand [flags] <csnap-file>",
		Args: cobra.ExactArgs(1),
		RunE: runSnapshotExpand,
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().String("output-dir", "", "Directory to write expanded objects (default: stdout)")
	cmd.Flags().Int("show-object", -1, "Show specific object by index (0-based)")
	cmd.Flags().Bool("verify-only", false, "Only verify the snapshot without expanding")

	return cmd
}

// runSnapshotExpand executes the snapshot-expand command
func runSnapshotExpand(cmd *cobra.Command, args []string) error {
	// Use command context if available, otherwise use system context
	ctx := cmd.Context()
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	csnapFile := args[0]

	// Get CLI context for project root and profile
	cliCtx := cli.GetContext(cmd)
	projectRoot := "."
	profile := systemProfileSystem // Default for snapshot-expand (system operation)
	if cliCtx != nil {
		if cliCtx.ProjectRoot != emptyValue {
			projectRoot = cliCtx.ProjectRoot
		}
		if cliCtx.Profile != emptyValue {
			profile = cliCtx.Profile
		}
	}
	projectRoot = ProjectRootOrResolveDot(projectRoot)

	// Check if file exists
	if _, err := fileutil.Stat(csnapFile); fileutil.IsNotExist(err) {
		return errfmt.Errorf("compressed snapshot file not found: %s", csnapFile)
	}

	// Get relative path for audit events
	relSnapPath := csnapFile
	if projectRoot != emptyValue {
		if rel, err := filepath.Rel(projectRoot, csnapFile); err == nil {
			relSnapPath = rel
		}
	}

	startTime := time.Now()

	// Get storage provider for coordinator (needed for audit and metrics)
	var storageProvider storage.ObjectStorageProvider
	if projectRoot != emptyValue {
		if factory, err := storage.NewStorageFactory(ctx, projectRoot); err == nil && factory != nil {
			sp := factory.GetStorage()
			if sp != nil {
				defer func() { _ = sp.Shutdown(context.Background()) }() // Background: request-or-shutdown derived
			}
			storageProvider = sp
		}
	}

	// Build shared event data for start
	startEventData := buildSnapshotExpandEventData(csnapFile, relSnapPath, startTime, nil, nil, nil)

	// Emit start event via coordinator (routes to logging, audit, metrics, operational channels)
	emitSnapshotExpandEventViaCoordinator(ctx, projectRoot, storageProvider, "snapshot_expand_start", "snapshot_expand", "start", startEventData, nil, 0, profile)

	// Read compressed snapshot
	logging.Fluent(logger).Info("Reading compressed snapshot").File(csnapFile).Log()
	expandStartTime := time.Now()
	cs, err := storage.ReadCompressedSnapshot(csnapFile)
	if err != nil {
		// Build shared event data for error
		errorEventData := buildSnapshotExpandEventData(csnapFile, relSnapPath, time.Now(), err, nil, nil)

		// Emit error event via coordinator (routes to logging, audit, metrics, operational channels)
		emitSnapshotExpandEventViaCoordinator(ctx, projectRoot, storageProvider, "snapshot_expand_error", "snapshot_expand", "error", errorEventData, err, 0, profile)
		return errfmt.Newf("failed to read compressed snapshot").Wrap(err)
	}
	expandDuration := time.Since(expandStartTime)

	// Display header information via logging (POL-CODE-007)
	logging.Fluent(logger).Info("Compressed snapshot information").
		String("format_version", cs.Header.FormatVersion).
		String("timestamp", cs.Header.Timestamp.Format("2006-01-02 15:04:05 UTC")).
		ObjectCount(cs.Header.ObjectCount).
		Int("dictionary_size", cs.Header.DictionarySize).
		String("compression_algo", cs.Header.CompressionAlgo).
		String("checksum_preview", cs.Header.Checksum[:16]+"...").
		Log()

	logging.Fluent(logger).Debug("Loaded compressed snapshot").
		String("format_version", cs.Header.FormatVersion).
		ObjectCount(cs.Header.ObjectCount).
		Int("dictionary_size", cs.Header.DictionarySize).
		Log()

	// Verify only mode
	verifyOnly, _ := cmd.Flags().GetBool("verify-only")
	if verifyOnly {
		logging.Fluent(logger).Info("Snapshot verified successfully").Log()
		return nil
	}

	// Expand snapshot
	logging.Fluent(logger).Info("Expanding snapshot").Log()
	expanded, err := cs.Expand()
	if err != nil {
		logging.Fluent(logger).Error("Failed to expand snapshot", err).Log()
		return errfmt.Newf("failed to expand snapshot").Wrap(err)
	}

	logging.Fluent(logger).Info("Snapshot expanded successfully").ObjectCount(len(expanded)).Log()

	// Show specific object if requested
	showObject, _ := cmd.Flags().GetInt("show-object")
	if showObject >= 0 {
		if showObject >= len(expanded) {
			return errfmt.Errorf("object index %d out of range (0-%d)", showObject, len(expanded)-1)
		}
		obj := expanded[showObject]
		id, _ := obj[objects.FieldKeyID].(string)
		kind, _ := obj[objects.FieldKeyKind].(string)
		title, _ := obj[objects.FieldKeyTitle].(string)
		logging.Fluent(logger).Info("Object at index").
			Index(showObject).
			ObjectID(id).
			Kind(kind).
			Title(title).
			Log()
		return nil
	}

	// Write to output directory if specified
	outputDir, _ := cmd.Flags().GetString("output-dir")
	if outputDir != emptyValue {
		logging.Fluent(logger).Info("Writing expanded objects to directory").OutputDir(outputDir).Log()

		// Create output directory if it doesn't exist
		if err := fileutil.MkdirAll(outputDir, paths.DirPerm755); err != nil {
			logging.Fluent(logger).Error("Failed to create output directory", err).OutputDir(outputDir).Log()
			return errfmt.Newf("failed to create output directory").Wrap(err)
		}

		// Write each object to its appropriate directory based on kind
		// Use parallel workers: budgeted pool when available, else N goroutines with channel
		const numWorkers = 10
		type writeJob struct {
			obj map[string]any
			idx int
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		written := 0
		skipped := 0
		var firstError error
		writeStartTime := time.Now()

		writeOneJob := func(job writeJob) {
			obj := job.obj
			kind, ok := obj[objects.FieldKeyKind].(string)
			if !ok || kind == emptyValue {
				_ = concurrency.RunInLock(&mu, func() error { skipped++; return nil })
				return
			}
			id, ok := obj[objects.FieldKeyID].(string)
			if !ok || id == emptyValue {
				logging.Fluent(logger).Debug("Skipping object with missing ID").Kind(kind).JobIdx(job.idx).Log()
				_ = concurrency.RunInLock(&mu, func() error { skipped++; return nil })
				return
			}
			dirName := objects.GetDirectoryFromKind(kind)
			if dirName == emptyValue {
				logging.Fluent(logger).Debug("Skipping object with unknown kind").ObjectID(id).Kind(kind).Log()
				_ = concurrency.RunInLock(&mu, func() error { skipped++; return nil })
				return
			}
			kindDir := filepath.Join(outputDir, dirName)
			if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
				logging.Fluent(logger).Error("Failed to create kind directory", err).KindDir(kindDir).Log()
				_ = concurrency.RunInLock(&mu, func() error {
					if firstError == nil {
						firstError = errfmt.Errorf("failed to create kind directory %s: %w", kindDir, err)
					}
					return nil
				})
				return
			}
			data, err := yaml.Marshal(obj)
			if err != nil {
				logging.Fluent(logger).Error("Failed to marshal object", err).ObjectID(id).Kind(kind).Log()
				_ = concurrency.RunInLock(&mu, func() error {
					if firstError == nil {
						firstError = errfmt.Errorf("failed to marshal object %s: %w", id, err)
					}
					return nil
				})
				return
			}
			filePath := filepath.Join(kindDir, id+".yaml")
			if err := fileutil.WriteFile(filePath, data, paths.FilePerm644); err != nil { //nolint:gosec // Object files - 0600 is acceptable for user-readable files
				logging.Fluent(logger).Error("Failed to write object file", err).FilePath(filePath).ObjectID(id).Log()
				_ = concurrency.RunInLock(&mu, func() error {
					if firstError == nil {
						firstError = errfmt.Errorf("failed to write object %s: %w", id, err)
					}
					return nil
				})
				return
			}
			_ = concurrency.RunInLock(&mu, func() error { written++; return nil })
		}

		queueSize := len(expanded)
		if queueSize > 2000 {
			queueSize = 2000
		}
		if queueSize < numWorkers {
			queueSize = numWorkers
		}

		pool := goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "snapshot_expand_worker", "expanding snapshot objects", numWorkers, queueSize)
		pool.Start(ctx)
		for i, obj := range expanded {
			if ctx.Err() != nil {
				break
			}
			job := writeJob{obj: obj, idx: i}
			wg.Add(1)
			submitErr := pool.Submit(ctx, func(taskCtx context.Context) error {
				defer wg.Done()
				writeOneJob(job)
				return nil
			})
			if submitErr != nil {
				wg.Done()
				if ctx.Err() != nil {
					break
				}
				return errfmt.Newf("submitting snapshot expand work").Wrap(submitErr)
			}
		}
		wg.Wait()
		pool.Stop()

		// Check for errors
		if firstError != nil {
			logging.Fluent(logger).Error("Error during parallel write", firstError).Log()
			return firstError
		}

		if skipped > 0 {
			logging.Fluent(logger).Warn("Skipped objects during expansion").SkippedCount(skipped).Log()
		}

		logging.Fluent(logger).Info("Successfully wrote objects to directory").
			WrittenCount(written).
			SkippedCount(skipped).
			OutputDir(outputDir).
			Log()

		// Get relative output directory for audit events
		relOutputDir := outputDir
		if projectRoot != "." {
			if rel, err := filepath.Rel(projectRoot, outputDir); err == nil {
				relOutputDir = rel
			}
		}

		writeDuration := time.Since(writeStartTime)
		totalDuration := time.Since(startTime)

		// Calculate throughput
		throughput := 0.0
		if totalDuration > 0 {
			throughput = float64(written) / totalDuration.Seconds()
		}

		// Build shared event data for completion
		completeEventData := buildSnapshotExpandEventData(csnapFile, relSnapPath, time.Now(), nil, &snapshotExpandCompletionData{
			outputDir:       relOutputDir,
			objectsExpanded: len(expanded),
			objectsWritten:  written,
			objectsSkipped:  skipped,
			totalDuration:   totalDuration,
			expandDuration:  expandDuration,
			writeDuration:   writeDuration,
			throughput:      throughput,
			workers:         numWorkers,
		}, nil)

		// Emit completion event via coordinator (routes to logging, audit, metrics, operational channels)
		// Note: The fields below (objects_expanded, objects_written, etc.) are stored in the
		// metadata sub-object, not as formal top-level schema fields. The metadata field is
		// a free-form object (key-value pairs) per the audit_event schema.
		emitSnapshotExpandEventViaCoordinator(ctx, projectRoot, storageProvider, "snapshot_expand_complete", "snapshot_expand", "complete", completeEventData, nil, totalDuration, profile)

		logging.Fluent(logger).Info("Wrote objects to directory").Written(written).OutputDir(outputDir).Log()
	} else {
		// Show summary via logging
		logging.Fluent(logger).Info("Expanded objects summary").Count(len(expanded)).Log()
		for i, obj := range expanded {
			id := "unknown"
			kind := "unknown"
			if idVal, ok := obj[objects.FieldKeyID].(string); ok {
				id = idVal
			}
			if kindVal, ok := obj[objects.FieldKeyKind].(string); ok {
				kind = kindVal
			}
			logging.Fluent(logger).Info("Expanded object").
				Index(i + 1).
				ObjectID(id).
				Kind(kind).
				Log()
		}
	}

	return nil
}

// snapshotExpandCompletionData contains completion-specific data for snapshot expansion
type snapshotExpandCompletionData struct {
	outputDir       string
	objectsExpanded int
	objectsWritten  int
	objectsSkipped  int
	totalDuration   time.Duration
	expandDuration  time.Duration
	writeDuration   time.Duration
	throughput      float64
	workers         int
}

// snapshotExpandEventData contains shared event data for logging, audit, and metrics
type snapshotExpandEventData struct {
	LoggingFields []logging.Field
	AuditMetadata map[string]any
	MetricsData   map[string]any
}

// buildSnapshotExpandEventData builds shared event data for logging, audit, and metrics
// This eliminates duplication by creating a single source of truth for event data
func buildSnapshotExpandEventData(
	snapshot string,
	snapshotPath string,
	timestamp time.Time,
	err error,
	completionData *snapshotExpandCompletionData,
	metricsOnlyData map[string]any, // Additional metrics-only data if needed
) snapshotExpandEventData {
	timestampStr := timestamp.Format(time.RFC3339)

	// Determine event type based on what data is present
	eventType := "snapshot_expand_start"
	if err != nil {
		eventType = "snapshot_expand_error"
	} else if completionData != nil {
		eventType = "snapshot_expand_complete"
	}

	// Base fields common to all event types
	loggingFields := []logging.Field{
		logging.String("event", eventType),
		logging.String("snapshot", snapshot),
		logging.String("snapshot_path", snapshotPath),
	}

	auditMetadata := map[string]any{
		objects.FieldKeySource:  "cli",
		objects.FieldKeyCommand: paths.CLIUsage("system", "snapshot-expand"),
		"snapshot":              snapshot,
		"timestamp":             timestampStr,
	}

	metricsData := map[string]any{
		"snapshot": snapshot,
	}

	// Add error information if present
	if err != nil {
		loggingFields = append(loggingFields, logging.Error(err))
		auditMetadata["error"] = err.Error()
	}

	// Add completion data if present
	if completionData != nil {
		loggingFields = append(loggingFields,
			logging.String("output_dir", completionData.outputDir),
			logging.Int("objects_expanded", completionData.objectsExpanded),
			logging.Int("objects_written", completionData.objectsWritten),
			logging.Int("objects_skipped", completionData.objectsSkipped),
			logging.Field{Key: "duration_seconds", Value: completionData.totalDuration.Seconds()},
			logging.Field{Key: "throughput", Value: completionData.throughput},
		)

		auditMetadata["output_dir"] = completionData.outputDir
		auditMetadata["objects_expanded"] = completionData.objectsExpanded
		auditMetadata["objects_written"] = completionData.objectsWritten
		auditMetadata["objects_skipped"] = completionData.objectsSkipped
		auditMetadata[objects.FieldKeyDurationSeconds] = completionData.totalDuration.Seconds()
		auditMetadata["duration_ns"] = completionData.totalDuration.Nanoseconds()

		metricsData["output_dir"] = completionData.outputDir
		metricsData["objects_expanded"] = completionData.objectsExpanded
		metricsData["objects_written"] = completionData.objectsWritten
		metricsData["objects_skipped"] = completionData.objectsSkipped
		metricsData["workers"] = completionData.workers
		metricsData["total_duration_sec"] = completionData.totalDuration.Seconds()
		metricsData["expand_duration_sec"] = completionData.expandDuration.Seconds()
		metricsData["write_duration_sec"] = completionData.writeDuration.Seconds()
		metricsData["throughput"] = completionData.throughput
	}

	// Merge any additional metrics-only data
	maps.Copy(metricsData, metricsOnlyData)

	return snapshotExpandEventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}
}

// Deprecated: emitSnapshotExpandEvent and emitSnapshotExpandMetrics have been replaced by
// emitSnapshotExpandEventViaCoordinator in snapshot_expand_coordination.go
// These functions are kept for backwards compatibility but are no longer used.
// They will be removed in a future release.
