package system

import (
	"context"
	"io"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
)

// executeSnapshotWorkflow executes the main snapshot workflow
func executeSnapshotWorkflow(ctx context.Context, underlyingStorage storage.ObjectStorageProvider, snapshotManager *storage.SnapshotManager, flags *SnapshotScenarioFlags, logger logging.Logger, out io.Writer) error {
	objectsToCapture, err := discoverObjects(ctx, underlyingStorage, flags.ObjectIDs, flags.Kinds, flags.QueryPreset, logger)
	if err != nil {
		return errfmt.Newf("failed to discover objects").Wrap(err)
	}

	if len(objectsToCapture) == 0 {
		return errfmt.Errorf("no objects found to capture")
	}

	if flags.Verbose {
		logging.Fluent(logger).Info("Discovered objects to capture").Count(len(objectsToCapture)).Log()
	}

	snapshotTimestamp, err := beginSnapshot(snapshotManager, flags.Verbose, logger)
	if err != nil {
		return err
	}

	metadata, err := captureSnapshotMetadata(ctx, snapshotManager, objectsToCapture, flags.Verbose, out)
	if err != nil {
		_ = snapshotManager.EndSnapshot(ctx) //nolint:errcheck
		return err
	}

	extractedObjects, err := extractObjectsForSnapshot(ctx, underlyingStorage, metadata, flags, logger)
	if err != nil {
		_ = snapshotManager.EndSnapshot(ctx) //nolint:errcheck
		return err
	}

	if err := applyAdaptorsIfNeeded(ctx, extractedObjects, flags, logger, out); err != nil {
		_ = snapshotManager.EndSnapshot(ctx) //nolint:errcheck
		return err
	}

	if err := snapshotManager.EndSnapshot(ctx); err != nil {
		return errfmt.Newf("failed to end snapshot").Wrap(err)
	}

	if flags.Verbose {
		logging.Fluent(logger).Info("Snapshot completed, operations replayed").Log()
	}

	compressedSnapshotPath, err := createCompressedSnapshot(extractedObjects, snapshotTimestamp, flags, logger, out)
	if err != nil {
		return err
	}

	scenarioObj, err := createScenarioObject(
		ctx,
		underlyingStorage,
		flags.Target,
		metadata,
		extractedObjects,
		flags.ChangePolicy,
		flags.Adaptors,
		nil, // adaptorConfig will be set if needed
		logger,
	)
	if err != nil {
		return errfmt.Newf("failed to create scenario object").Wrap(err)
	}

	if compressedSnapshotPath != emptyValue {
		scenarioObj["compressed_snapshot_path"] = compressedSnapshotPath
	}

	outputSnapshotSuccess(scenarioObj, flags.Target, extractedObjects, snapshotTimestamp, compressedSnapshotPath, out)

	return nil
}

// beginSnapshot begins a snapshot and returns the timestamp
func beginSnapshot(snapshotManager *storage.SnapshotManager, verbose bool, logger logging.Logger) (time.Time, error) {
	snapshotTimestamp, err := snapshotManager.BeginSnapshot()
	if err != nil {
		return time.Time{}, errfmt.Newf("failed to begin snapshot").Wrap(err)
	}

	if verbose && logger != nil {
		logging.Fluent(logger).Info("Snapshot initiated").TimestampRFC3339Nano(snapshotTimestamp.Format(time.RFC3339Nano)).Log()
	}

	return snapshotTimestamp, nil
}

// captureSnapshotMetadata captures metadata for the snapshot
func captureSnapshotMetadata(ctx context.Context, snapshotManager *storage.SnapshotManager, objectsToCapture []string, verbose bool, out io.Writer) (*storage.SnapshotMetadata, error) {
	metadata, err := snapshotManager.CaptureMetadata(ctx, objectsToCapture)
	if err != nil {
		return nil, errfmt.Newf("failed to capture metadata").Wrap(err)
	}

	outputMetadataDetails(metadata, verbose, out)
	return metadata, nil
}

// extractObjectsForSnapshot extracts objects for the snapshot
func extractObjectsForSnapshot(ctx context.Context, underlyingStorage storage.ObjectStorageProvider, metadata *storage.SnapshotMetadata, flags *SnapshotScenarioFlags, logger logging.Logger) ([]map[string]any, error) {
	extractedObjects, err := extractObjectsWithHandlers(ctx, underlyingStorage, metadata, flags.ChangePolicy, logger)
	if err != nil {
		return nil, errfmt.Newf("failed to extract objects").Wrap(err)
	}

	if flags.Verbose {
		logging.Fluent(logger).Info("Extracted objects").Count(len(extractedObjects)).Log()
	}

	return extractedObjects, nil
}

// applyAdaptorsIfNeeded applies adaptors to objects if needed
func applyAdaptorsIfNeeded(ctx context.Context, extractedObjects []map[string]any, flags *SnapshotScenarioFlags, logger logging.Logger, out io.Writer) error {
	if len(flags.Adaptors) == 0 {
		return nil
	}

	adaptorConfig := buildAdaptorConfig(flags)
	return applyAdaptorsToObjects(ctx, extractedObjects, flags.Adaptors, adaptorConfig, logger, flags.Verbose, out)
}
