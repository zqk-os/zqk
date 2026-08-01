// Stream stewardship: post-retention maintenance for stream-backed kinds (registry compaction,
// segment GC, runtime-delta backfill/overlay GC). REQ-STREAM-001; see STREAM_STORAGE.md,
// STREAM_KIND_STEWARDSHIP.md, and glossary object stream stewardship (GLS-EXAMPLE).
package storage

import (
	"context"
	"fmt"
	"sort"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/datacellregistry"
	"github.com/lanceman/zqk/pkg/logging"
)

// streamStewardPhase* are detail tokens for steward enqueue JSONL (after c=…|k=…|p=…).
const (
	streamStewardPhaseSegments     = "segments"
	streamStewardPhaseRuntimeDelta = "runtime_delta"
)

func streamStewardEnqueueDetail(cycleID, kind, phase string) string {
	return fmt.Sprintf(ConstStreamCStrKStrPStr, cycleID, kind, phase)
}

// RunPostRetentionStreamKindStewardship compacts the stream registry for one kind and runs orphan
// segment GC. Low-volume kinds use the same path; compact/GC are cheap when little or no data exists.
func RunPostRetentionStreamKindStewardship(projectRoot, kind string) (GCStreamSegmentResult, error) {
	if err := CompactStreamRegistryForKind(projectRoot, kind); err != nil {
		return GCStreamSegmentResult{Kind: kind}, err
	}
	gc := GCOrphanedStreamSegmentsForKind(projectRoot, kind)
	return gc, nil
}

// PostRetentionStreamStewardship compacts stream registries, runs segment GC, backfills runtime-delta
// overlays, and GCs stale overlay files. Call after a successful retention cycle (or from tests with
// an empty project root — operations are no-ops when paths are missing).
//
// Each stream-backed kind is processed independently; one steward enqueue record is written per kind
// per phase so envelope-tick drain and metrics stay per-kind (see STREAM_KIND_STEWARDSHIP.md).
func PostRetentionStreamStewardship(projectRoot string, cycleID string, logger logging.Logger) {
	if projectRoot == emptyValue {
		return
	}
	if logger == nil {
		return
	}
	streamKinds := StreamStorageEnabledKindsList()
	sort.Strings(streamKinds)

	mismatches, err := datacellregistry.HighVolumeStreamSpecProfileMismatches(projectRoot, streamKinds)
	if err != nil {
		StorageLog(logger).Warn(LogEventStorageStreamStewardshipSpecIndexUnavailableWarn).
			String("cycle_id", cycleID).
			WithError(err).
			Log()
	} else {
		for _, detail := range mismatches {
			StorageLog(logger).Warn(LogEventStorageStreamStewardshipSpecDriftWarn).
				String("cycle_id", cycleID).
				String("detail", detail).
				Log()
		}
	}

	var totalFilesRemoved int
	var totalBytesFreed int64
	for _, kind := range streamKinds {
		gc, compactErr := RunPostRetentionStreamKindStewardship(projectRoot, kind)
		if compactErr != nil {
			StorageLog(logger).Warn(LogEventStorageStreamStewardshipRegistryCompactWarn).
				String("cycle_id", cycleID).
				Kind(kind).
				WithError(compactErr).
				Log()
			detail := streamStewardEnqueueDetail(cycleID, kind, streamStewardPhaseSegments)
			if err := datacell.EnqueueStewardMaintenance(context.Background(), projectRoot, datacell.ProfileStream,
				datacell.MaintenanceOp{Name: datacell.MaintenanceOpStreamStewardKind, Detail: detail}, logger); err != nil {
				StorageLog(logger).Warn(ConstStreamStewardEnqueueFailedAfterRegistryCompactError).
					String("cycle_id", cycleID).Kind(kind).WithError(err).Log()
			}
			continue
		}
		totalFilesRemoved += gc.FilesRemoved
		totalBytesFreed += gc.BytesFreed
		if gc.FilesRemoved > 0 || gc.Errors > 0 {
			StorageLog(logger).Info(LogEventStorageStreamStewardshipSegmentGCInfo).
				String("cycle_id", cycleID).
				Kind(kind).
				Int("files_removed", gc.FilesRemoved).
				Int("bytes_freed", int(gc.BytesFreed)).
				Int("errors", gc.Errors).
				Log()
		}
		detail := streamStewardEnqueueDetail(cycleID, kind, streamStewardPhaseSegments)
		if err := datacell.EnqueueStewardMaintenance(context.Background(), projectRoot, datacell.ProfileStream,
			datacell.MaintenanceOp{Name: datacell.MaintenanceOpStreamStewardKind, Detail: detail}, logger); err != nil {
			StorageLog(logger).Warn(ConstStreamStewardEnqueueFailed).
				String("cycle_id", cycleID).Kind(kind).WithError(err).Log()
		}
	}

	if totalFilesRemoved > 0 {
		StorageLog(logger).Info(LogEventStorageStreamStewardshipSegmentGCSummaryInfo).
			String("cycle_id", cycleID).
			Int(ConstStreamTotalFilesRemoved, totalFilesRemoved).
			Int(ConstStreamTotalBytesFreed, int(totalBytesFreed)).
			Log()
	}

	rtKinds := RuntimeDeltaEnabledKindsList(projectRoot)
	sort.Strings(rtKinds)
	for _, kind := range rtKinds {
		backfill := BackfillRuntimeDeltaCurrentForKind(projectRoot, kind)
		if backfill.Created > 0 || backfill.Failures > 0 {
			StorageLog(logger).Info(LogEventStorageStreamStewardshipRuntimeDeltaBackfillInfo).
				String("cycle_id", cycleID).
				Kind(kind).
				Int("scanned", backfill.Scanned).
				Int("created", backfill.Created).
				Int("skipped", backfill.Skipped).
				Int("failures", backfill.Failures).
				Log()
		}
		gc := GCRuntimeDeltaCurrentForKind(projectRoot, kind)
		if gc.FilesRemoved > 0 || gc.Errors > 0 {
			StorageLog(logger).Info(LogEventStorageStreamStewardshipRuntimeDeltaOverlayGCInfo).
				String("cycle_id", cycleID).
				Kind(kind).
				Int("files_removed", gc.FilesRemoved).
				Int("errors", gc.Errors).
				Log()
		}
		detail := streamStewardEnqueueDetail(cycleID, kind, streamStewardPhaseRuntimeDelta)
		if err := datacell.EnqueueStewardMaintenance(context.Background(), projectRoot, datacell.ProfileStream,
			datacell.MaintenanceOp{Name: datacell.MaintenanceOpStreamStewardKind, Detail: detail}, logger); err != nil {
			StorageLog(logger).Warn(ConstStreamStewardEnqueueFailedRuntimeDeltaPhase).
				String("cycle_id", cycleID).Kind(kind).WithError(err).Log()
		}
	}
}
