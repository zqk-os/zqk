package migration

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// SnapshotCreator creates migration snapshots (pre, post, checkpoint).
// When nil, the executor uses deterministic synthetic IDs for traceability.
// Implementations can integrate with storage.SnapshotManager (e.g. from cmd).
type SnapshotCreator interface {
	CreatePreMigrationSnapshot(ctx context.Context, spec *Spec) (snapshotID string, err error)
	CreatePostMigrationSnapshot(ctx context.Context, spec *Spec, preSnapshotID string) (snapshotID string, err error)
	CreateCheckpointSnapshot(ctx context.Context, spec *Spec, step Step, count int) (snapshotID string, err error)
}

// Executor executes migration specs
type Executor struct {
	storageProvider storage.ObjectStorageProvider
	projectRoot     string
	logger          logging.Logger
	snapshotCreator SnapshotCreator // optional; when nil, synthetic IDs are used
}

// NewExecutor creates a new migration executor
func NewExecutor(storageProvider storage.ObjectStorageProvider, projectRoot string, logger logging.Logger) *Executor {
	return &Executor{
		storageProvider: storageProvider,
		projectRoot:     projectRoot,
		logger:          logger,
	}
}

// SetSnapshotCreator sets the optional snapshot creator for pre/post/checkpoint snapshots.
// When set, the executor calls it instead of returning synthetic IDs.
func (e *Executor) SetSnapshotCreator(c SnapshotCreator) {
	e.snapshotCreator = c
}

// Execute executes a migration spec
func (e *Executor) Execute(ctx context.Context, spec *Spec, options ExecutionOptions) (*ExecutionResult, error) {
	startTime := time.Now()
	result := &ExecutionResult{
		MigrationID:      spec.ID,
		Status:           objects.ObjectStatusRunning,
		StepsTotal:       len(spec.Steps),
		LocationMappings: make(map[string]string),
		FieldMappings:    make(map[string]map[string]string),
		StepResults:      make([]StepResult, 0),
		StartedAt:        startTime,
	}

	// Emit migration start event via coordinator
	operationID := fmt.Sprintf("migration_%s_%d", spec.ID, startTime.Unix())
	emitMigrationEventViaCoordinator(
		ctx,
		e.projectRoot,
		e.storageProvider,
		operationID,
		"migration_execute",
		"start",
		spec.ID,
		map[string]any{
			"spec_name":        spec.Name,
			"steps_total":      len(spec.Steps),
			"dry_run":          spec.Options.DryRun,
			"force":            spec.Options.Force,
			"snapshot_enabled": spec.SnapshotCompatible,
		},
		nil,
		0,
		"human",
	)

	// Override spec options with command-line options
	if options.DryRun != nil {
		spec.Options.DryRun = *options.DryRun
	}
	if options.Force != nil {
		spec.Options.Force = *options.Force
	}

	// Validate prerequisites
	if err := e.validatePrerequisites(ctx, spec); err != nil {
		return nil, errfmt.Newf("prerequisites not met").Wrap(err)
	}

	// Create pre-migration snapshot if required
	if spec.SnapshotCompatible && spec.PreMigrationSnapshot != nil && spec.PreMigrationSnapshot.AutoCreate {
		snapshotID, err := e.createPreMigrationSnapshot(ctx, spec)
		if err != nil {
			return nil, errfmt.Newf("failed to create pre-migration snapshot").Wrap(err)
		}
		result.PreSnapshotID = snapshotID
		logging.Fluent(e.logger).Info("Created pre-migration snapshot").
			String("migration_id", spec.ID).
			String("snapshot_id", snapshotID).
			Log()

		// Emit snapshot creation event via coordinator
		snapshotOperationID := fmt.Sprintf("migration_%s_snapshot_pre_%d", spec.ID, time.Now().Unix())
		emitMigrationEventViaCoordinator(
			ctx,
			e.projectRoot,
			e.storageProvider,
			snapshotOperationID,
			"migration_snapshot",
			"created",
			spec.ID,
			map[string]any{
				objects.FieldKeySnapshotID: snapshotID,
				"snapshot_type":            "pre_migration",
			},
			nil,
			0,
			"human",
		)
	}

	// Execute steps
	stepOutputs := make(map[string]any)
	for i, step := range spec.Steps {
		stepStartTime := time.Now()
		stepResult := e.executeStep(ctx, spec, &step, stepOutputs, options)
		stepDuration := time.Since(stepStartTime)
		result.StepResults = append(result.StepResults, stepResult)

		// Emit step completion event via coordinator
		stepOperationID := fmt.Sprintf("migration_%s_step_%s_%d", spec.ID, step.ID, stepStartTime.Unix())
		stepMetadata := map[string]any{
			"step_id":     step.ID,
			"step_type":   step.Type,
			"step_index":  i + 1,
			"steps_total": len(spec.Steps),
		}
		if stepResult.Metadata != nil {
			for k, v := range stepResult.Metadata {
				stepMetadata[k] = v
			}
		}
		stepStatus := objects.ObjectStatusComplete
		if stepResult.Error != nil {
			stepStatus = objects.ObjectStatusError
		}
		emitMigrationEventViaCoordinator(
			ctx,
			e.projectRoot,
			e.storageProvider,
			stepOperationID,
			"migration_step",
			stepStatus,
			spec.ID,
			stepMetadata,
			stepResult.Error,
			stepDuration,
			"human",
		)

		if stepResult.Error != nil {
			if spec.Options.ContinueOnError {
				logging.Fluent(e.logger).Warn("Step failed, continuing").
					String("step_id", step.ID).
					WithError(stepResult.Error).
					Log()
				continue
			}
			result.Status = objects.ObjectStatusFailed
			result.CompletedAt = time.Now()
			result.Duration = result.CompletedAt.Sub(startTime)

			// Emit migration failure event via coordinator
			emitMigrationEventViaCoordinator(
				ctx,
				e.projectRoot,
				e.storageProvider,
				operationID,
				"migration_execute",
				"failed",
				spec.ID,
				map[string]any{
					"steps_completed": result.StepsCompleted,
					"steps_total":     len(spec.Steps),
					"failed_step":     step.ID,
				},
				stepResult.Error,
				result.Duration,
				"human",
			)

			return result, errfmt.Errorf("step %s failed: %w", step.ID, stepResult.Error)
		}

		// Store step output for dependent steps
		stepOutputs[step.ID] = stepResult.Output
		result.StepsCompleted = i + 1

		// Create checkpoint if configured
		if step.Checkpoint != nil && stepResult.Metadata != nil {
			if count, ok := stepResult.Metadata["count"].(int); ok {
				if count > 0 && step.Checkpoint.Interval > 0 && count%step.Checkpoint.Interval == 0 {
					logging.Fluent(e.logger).Info("Checkpoint reached").
						String("step_id", step.ID).
						Count(count).
						Log()
					if step.Checkpoint.Snapshot {
						checkpointID, err := e.createCheckpointSnapshot(ctx, spec, step, count)
						if err != nil {
							logging.Fluent(e.logger).Warn("Failed to create checkpoint snapshot").
								String("step_id", step.ID).
								Count(count).
								WithError(err).
								Log()
							emitMigrationEventViaCoordinator(ctx, e.projectRoot, e.storageProvider,
								fmt.Sprintf("migration_%s_checkpoint_%s_%d", spec.ID, step.ID, time.Now().Unix()),
								"migration_checkpoint", "failed", spec.ID,
								map[string]any{"step_id": step.ID, "count": count}, err, 0, "human")
						} else {
							logging.Fluent(e.logger).Info("Created checkpoint snapshot").
								String("step_id", step.ID).
								Count(count).
								String("checkpoint_id", checkpointID).
								Log()
							emitMigrationEventViaCoordinator(ctx, e.projectRoot, e.storageProvider,
								fmt.Sprintf("migration_%s_checkpoint_%s_%d", spec.ID, step.ID, time.Now().Unix()),
								"migration_checkpoint", "created", spec.ID,
								map[string]any{"step_id": step.ID, "count": count, "checkpoint_id": checkpointID}, nil, 0, "human")
						}
					}
				}
			}
		}
	}

	// Run validation
	if err := e.runValidation(ctx, spec, stepOutputs); err != nil {
		result.Status = objects.ObjectStatusValidationFailed
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(startTime)

		// Emit validation failure event via coordinator
		emitMigrationEventViaCoordinator(
			ctx,
			e.projectRoot,
			e.storageProvider,
			operationID,
			"migration_execute",
			"validation_failed",
			spec.ID,
			map[string]any{
				"steps_completed": result.StepsCompleted,
				"steps_total":     len(spec.Steps),
			},
			err,
			result.Duration,
			"human",
		)

		return result, errfmt.Newf("validation failed").Wrap(err)
	}

	// Create post-migration snapshot if required
	if spec.SnapshotCompatible && spec.PostMigrationSnapshot != nil && spec.PostMigrationSnapshot.AutoCreate {
		snapshotID, err := e.createPostMigrationSnapshot(ctx, spec, result.PreSnapshotID)
		if err != nil {
			logging.Fluent(e.logger).Warn("Failed to create post-migration snapshot").
				WithError(err).
				Log()

			// Emit snapshot creation failure event via coordinator
			snapshotOperationID := fmt.Sprintf("migration_%s_snapshot_post_%d", spec.ID, time.Now().Unix())
			emitMigrationEventViaCoordinator(
				ctx,
				e.projectRoot,
				e.storageProvider,
				snapshotOperationID,
				"migration_snapshot",
				"failed",
				spec.ID,
				map[string]any{
					"snapshot_type": "post_migration",
				},
				err,
				0,
				"human",
			)
		} else {
			result.PostSnapshotID = snapshotID
			logging.Fluent(e.logger).Info("Created post-migration snapshot").
				String("migration_id", spec.ID).
				String("snapshot_id", snapshotID).
				Log()

			// Emit snapshot creation event via coordinator
			snapshotOperationID := fmt.Sprintf("migration_%s_snapshot_post_%d", spec.ID, time.Now().Unix())
			emitMigrationEventViaCoordinator(
				ctx,
				e.projectRoot,
				e.storageProvider,
				snapshotOperationID,
				"migration_snapshot",
				"created",
				spec.ID,
				map[string]any{
					objects.FieldKeySnapshotID: snapshotID,
					"snapshot_type":            "post_migration",
					"pre_snapshot_id":          result.PreSnapshotID,
				},
				nil,
				0,
				"human",
			)
		}
	}

	result.Status = objects.ObjectStatusCompleted
	result.CompletedAt = time.Now()
	result.Duration = result.CompletedAt.Sub(startTime)

	// Record in migration history when actually applied (not dry run)
	if (options.DryRun == nil || !*options.DryRun) && e.projectRoot != emptyValue {
		if err := RecordSuccess(e.projectRoot, spec.ID); err != nil {
			logging.Fluent(e.logger).Warn("Failed to record migration in history").
				String("migration_id", spec.ID).
				WithError(err).
				Log()
		}
	}

	// Emit migration completion event via coordinator
	completionMetadata := map[string]any{
		"steps_completed":  result.StepsCompleted,
		"steps_total":      result.StepsTotal,
		"objects_created":  result.ObjectsCreated,
		"objects_updated":  result.ObjectsUpdated,
		"objects_deleted":  result.ObjectsDeleted,
		"pre_snapshot_id":  result.PreSnapshotID,
		"post_snapshot_id": result.PostSnapshotID,
	}
	emitMigrationEventViaCoordinator(
		ctx,
		e.projectRoot,
		e.storageProvider,
		operationID,
		"migration_execute",
		"completed",
		spec.ID,
		completionMetadata,
		nil,
		result.Duration,
		"human",
	)

	return result, nil
}

// ExecutionOptions provides options for migration execution
type ExecutionOptions struct {
	DryRun *bool
	Force  *bool
}

// validatePrerequisites validates all prerequisites are met before running the migration.
// Supports prerequisite types: kind (must be in kind mapper), directory (exists or must not exist).
//
//nolint:unparam // ctx reserved for future use (e.g. timeout, cancellation during kind init).
func (e *Executor) validatePrerequisites(ctx context.Context, spec *Spec) error {
	if len(spec.Prerequisites) == 0 {
		return nil
	}
	for i, prereq := range spec.Prerequisites {
		switch prereq.Type {
		case "kind":
			if prereq.Kind == emptyValue {
				continue
			}
			mapper := objects.GetGlobalKindMapper()
			if err := mapper.Initialize(); err != nil {
				return errfmt.Errorf("prerequisite %d: kind mapper init: %w", i, err)
			}
			allKinds := mapper.GetAllKinds()
			found := false
			for _, k := range allKinds {
				if k == prereq.Kind {
					found = true
					break
				}
			}
			if !found {
				return errfmt.Errorf("prerequisite %d: kind %q is not registered", i, prereq.Kind)
			}
		case "directory":
			if prereq.Directory == emptyValue {
				continue
			}
			dirPath := prereq.Directory
			if !filepath.IsAbs(dirPath) {
				dirPath = filepath.Join(e.projectRoot, dirPath)
			}
			_, err := fileutil.Stat(dirPath)
			exists := err == nil
			if prereq.Exists != nil {
				wantExist := *prereq.Exists
				if exists != wantExist {
					if wantExist {
						return errfmt.Errorf("prerequisite %d: directory %q must exist but does not", i, prereq.Directory)
					}
					return errfmt.Errorf("prerequisite %d: directory %q must not exist but exists", i, prereq.Directory)
				}
			}
		case "config":
			// Config prerequisites not yet implemented; skip
		default:
			// Unknown type; skip
		}
	}
	return nil
}

// executeStep executes a single migration step
func (e *Executor) executeStep(ctx context.Context, spec *Spec, step *Step, stepOutputs map[string]any, options ExecutionOptions) StepResult {
	result := StepResult{
		StepID:   step.ID,
		Output:   make(map[string]any),
		Metadata: make(map[string]any),
	}

	// Check dependencies
	for _, dep := range step.DependsOn {
		if _, exists := stepOutputs[dep]; !exists {
			result.Error = errfmt.Errorf("dependency %s not found", dep)
			result.Success = false
			return result
		}
	}

	// Execute step based on type
	switch step.Type {
	case "scan_files":
		result = e.executeScanFilesStep(ctx, step, stepOutputs)
	case "read_id_list":
		result = e.executeReadIDListStep(ctx, step, stepOutputs)
	case "read_objects":
		result = e.executeReadObjectsStep(ctx, step, stepOutputs)
	case "transform":
		result = e.executeTransformStep(ctx, spec, step, stepOutputs)
	case "create_objects":
		result = e.executeCreateObjectsStep(ctx, spec, step, stepOutputs, options)
	case "delete_objects":
		result = e.executeDeleteObjectsStep(ctx, step, stepOutputs, options)
	case "execute_command":
		result = e.executeCommandStep(ctx, step, stepOutputs)
	default:
		result.Error = errfmt.Errorf("unknown step type: %s", step.Type)
		result.Success = false
		return result
	}

	return result
}

// executeScanFilesStep is implemented in steps.go
// executeReadIDListStep is implemented in steps.go
// executeReadObjectsStep is implemented in steps.go
// executeTransformStep is implemented in steps.go
// executeCreateObjectsStep is implemented in steps.go
// executeDeleteObjectsStep is implemented in steps.go
// executeCommandStep is implemented in steps.go

// toInt converts config value (int or float64 from YAML) to int; returns -1 if missing or invalid.
func toInt(v any) int {
	if v == nil {
		return -1
	}
	switch v := v.(type) {
	case int:
		return v
	case float64:
		return int(v)
	}

	return -1
}

// runValidation runs validation rules against step outputs (e.g. object_count, file_count).
// ValidationRule.Config: step_id (required), min/max (optional int), expected (optional int).
func (e *Executor) runValidation(_ context.Context, spec *Spec, stepOutputs map[string]any) error {
	if len(spec.Validation) == 0 {
		return nil
	}
	for i, rule := range spec.Validation {
		config := rule.Config
		if config == nil {
			continue
		}
		stepID, _ := config["step_id"].(string)
		if stepID == emptyValue {
			continue
		}
		output, ok := stepOutputs[stepID].(map[string]any)
		if !ok {
			return errfmt.Errorf("validation %d: step %q output not found", i, stepID)
		}
		var count int
		switch v := output["count"].(type) {
		case int:
			count = v
		case float64:
			count = int(v)
		default:
			continue
		}
		switch rule.Type {
		case "object_count", "file_count":
			if min := toInt(config["min"]); min >= 0 && count < min {
				return errfmt.Errorf("validation %d: step %q count %d below min %d", i, stepID, count, min)
			}
			if max := toInt(config["max"]); max >= 0 && count > max {
				return errfmt.Errorf("validation %d: step %q count %d above max %d", i, stepID, count, max)
			}
			if expected := toInt(config["expected"]); expected >= 0 && count != expected {
				return errfmt.Errorf("validation %d: step %q count %d != expected %d", i, stepID, count, expected)
			}
		}
	}
	return nil
}

// createCheckpointSnapshot creates a checkpoint snapshot when step.Checkpoint.Snapshot is true.
// Called at checkpoint intervals during step execution. Returns checkpoint ID or error.
// Uses SnapshotCreator when set; otherwise returns a deterministic synthetic ID.
func (e *Executor) createCheckpointSnapshot(ctx context.Context, spec *Spec, step Step, count int) (string, error) {
	if e.snapshotCreator != nil {
		return e.snapshotCreator.CreateCheckpointSnapshot(ctx, spec, step, count)
	}
	return fmt.Sprintf("checkpoint-%s-%s-%d", spec.ID, step.ID, count), nil
}

// createPreMigrationSnapshot creates a pre-migration snapshot.
// Uses SnapshotCreator when set; otherwise returns a deterministic synthetic ID (optionally including first tag).
func (e *Executor) createPreMigrationSnapshot(ctx context.Context, spec *Spec) (string, error) {
	if e.snapshotCreator != nil {
		return e.snapshotCreator.CreatePreMigrationSnapshot(ctx, spec)
	}
	return "snapshot-pre-" + spec.ID, nil
}

// createPostMigrationSnapshot creates a post-migration snapshot.
// Uses SnapshotCreator when set; otherwise returns a deterministic synthetic ID (optionally including first tag).
func (e *Executor) createPostMigrationSnapshot(ctx context.Context, spec *Spec, preSnapshotID string) (string, error) {
	if e.snapshotCreator != nil {
		return e.snapshotCreator.CreatePostMigrationSnapshot(ctx, spec, preSnapshotID)
	}
	return "snapshot-post-" + spec.ID, nil
}
