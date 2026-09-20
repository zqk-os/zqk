package storage

import (
	"strings"
	"testing"
)

// Ensures newer POL-CODE-007 families keep prefix + "_" wire shape (spot-check high-churn groups).
func TestStorageRuntimeLogEvents_moreWirePrefixes(t *testing.T) {
	cases := []struct {
		prefix string
		want   []string
	}{
		{
			storageMetricInstanceWirePrefix + "_",
			[]string{
				LogEventStorageMetricInstanceSkipHashMismatch,
				LogEventStorageMetricInstanceSkipStaleCASIndex,
				LogEventStorageMetricInstanceCleanupStaleCASFailed,
			},
		},
		{
			storageBundledObjectSpecWirePrefix + "_",
			[]string{
				LogEventStorageBundledSpecReadFailed,
				LogEventStorageBundledSpecFinishedInfo,
			},
		},
		{
			storageObjectUpdateWirePrefix + "_",
			[]string{
				LogEventStorageObjectUpdateMutationClassifiedDebug,
				LogEventStorageObjectUpdateTouchProcessDirFailedDebug,
			},
		},
		{
			storageSnapshotWirePrefix + "_",
			[]string{
				LogEventStorageSnapshotInitiatedInfo,
				LogEventStorageSnapshotOpReplayCompletedInfo,
			},
		},
		{
			storageStreamStewardshipWirePrefix + "_",
			[]string{
				LogEventStorageStreamStewardshipSpecIndexUnavailableWarn,
				LogEventStorageStreamStewardshipRuntimeDeltaOverlayGCInfo,
			},
		},
		{
			storageSchedulerIntegrationWirePrefix + "_",
			[]string{
				LogEventStorageSchedulerIntegrationCascadeUpdateDirectInfo,
				LogEventStorageSchedulerIntegrationCacheInvalidationScheduleFallbackWarn,
			},
		},
		{
			storageObjectIDCachePendingWirePrefix + "_",
			[]string{
				LogEventStorageObjectIDCachePendingLoadFailedWarn,
				LogEventStorageObjectIDCachePendingPersistFailedWarn,
				LogEventStorageObjectIDCachePendingPersistSkippedLoadErrWarn,
			},
		},
	}
	for _, tc := range cases {
		for _, evt := range tc.want {
			if !strings.HasPrefix(evt, tc.prefix) {
				t.Fatalf("event %q must start with wire prefix %q", evt, tc.prefix)
			}
		}
	}
}
