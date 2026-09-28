package system

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2" // Register spec builders
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testkit"
)

// TestHashMismatchFixEventViaCoordinator tests that hash mismatch fix audit events are created via coordinator
func TestHashMismatchFixEventViaCoordinator(t *testing.T) {
	// Serial: global audit buffer/metrics and coordinator paths are not reliable under t.Parallel in scheduler bundles.
	// Isolate CAS index queues per project root: the default global singleton keeps the previous test's
	// projectRoot/storage when another test runs after TestExecuteFixCommand, and getContentAddressableStorage
	// then skips SetProjectRoot — audit CAS writes may not flush or list correctly (bundle flake).
	caspkg.SetListingIndexWriteQueueFactoryToPerProjectRoot()
	t.Cleanup(func() {
		caspkg.SetListingIndexWriteQueueFactory(nil)
	})

	// Disable global audit event buffer for tests (before isolated storage init).
	buffer := storage.GetGlobalAuditEventBuffer()
	buffer.SetEnabled(false)
	if err := buffer.Flush(); err != nil {
		t.Logf("Warning: Failed to flush buffer: %v", err)
	}

	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "system.hash_mismatch_fix_coordination",
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			return []testkit.NamedTestStep{
				{
					Name: "ensure_path_alias_cache",
					Fn: func() error {
						return storage.EnsurePathAliasCacheReady(root)
					},
				},
				{
					Name: "generate_object_specs",
					Fn: func() error {
						specsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
						if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
							return err
						}
						return builders.NewSpecGenerator(specsDir).GenerateAllSpecs()
					},
				},
			}
		},
	})
	testDir := proj.Root
	fileStorage := proj.FileStorage

	// Test data
	objID := fmt.Sprintf("BLI-HASHMISMATCH-%d", time.Now().UnixNano())
	kind := "backlog_item"
	relPath := filepath.Join(paths.ProcessBacklogDir, fmt.Sprintf("%s.yaml", objID))
	originalHash := "abc123def456"
	newHash := "newhash789xyz"

	// Emit hash mismatch fix audit event via coordinator
	type contextKey string
	const timestampKey contextKey = "timestamp"
	ctx := context.WithValue(pkgctx.NewSystemContext(), timestampKey, time.Now().Unix())
	if err := emitHashMismatchFixEventViaCoordinator(
		ctx,
		testDir,
		fileStorage,
		objID,
		kind,
		relPath,
		originalHash,
		newHash,
		systemProfileSystem, // profile
	); err != nil {
		t.Fatalf("emitHashMismatchFixEventViaCoordinator: %v", err)
	}

	// Flush buffer to ensure events are written
	if err := buffer.Flush(); err != nil {
		t.Logf("Warning: Failed to flush buffer: %v", err)
	}

	casQueue := caspkg.GetListingIndexWriteQueueForProjectRoot(testDir)
	if casQueue != nil {
		_ = casQueue.FlushAll(2 * time.Second)
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{
		Kind: "audit_event",
		Filters: map[string]any{
			eventKeyTargetKind: "backlog_item",
		},
		Limit: 50,
	}

	var result *storage.QueryResult
	found := waitForConditionWithTimeoutSystem(
		ctx,
		func() bool {
			if err := buffer.Flush(); err != nil {
				t.Logf("Warning: Failed to flush buffer: %v", err)
			}
			if casQueue != nil {
				_ = casQueue.FlushAll(2 * time.Second)
			}
			var listErr error
			result, listErr = fileStorage.List(ctx, secCtx, storageCtx, filter)
			if listErr != nil {
				t.Logf("Warning: Failed to list audit events: %v", listErr)
				return false
			}
			for _, event := range result.Objects {
				eventType, _ := event[eventKeyEventType].(string)
				origType := ""
				if md, ok := event[objects.FieldKeyMetadata].(map[string]any); ok {
					if s, ok := md[storage.OriginalEventTypeMetadataKey].(string); ok {
						origType = s
					}
				}
				isHashMismatch := eventType == "hash_mismatch_fix" || origType == "hash_mismatch_fix"
				operation, _ := event[eventKeyOperation].(string)
				targetID, _ := event[eventKeyTargetID].(string)
				if !isHashMismatch || targetID != objID {
					continue
				}
				if operation == emptyValue || !strings.Contains(operation, "Regenerated integrity hash") {
					continue
				}
				var originalValue, newValue string
				if metadata, ok := event[objects.FieldKeyMetadata].(map[string]any); ok {
					if ov, ok := metadata[objects.FieldKeyOriginalValue].(string); ok {
						originalValue = ov
					}
					if nv, ok := metadata[objects.FieldKeyNewValue].(string); ok {
						newValue = nv
					}
				}
				if originalValue == emptyValue {
					if ov, ok := event[objects.FieldKeyOriginalValue].(string); ok {
						originalValue = ov
					}
				}
				if newValue == emptyValue {
					if nv, ok := event[objects.FieldKeyNewValue].(string); ok {
						newValue = nv
					}
				}
				if originalValue != emptyValue && originalValue != originalHash {
					t.Logf("Warning: Original hash mismatch: expected %s, got %s", originalHash, originalValue)
				}
				if newValue != emptyValue && newValue != newHash {
					t.Logf("Warning: New hash mismatch: expected %s, got %s", newHash, newValue)
				}
				t.Logf("Found hash mismatch fix audit event: id=%v, event_type=%s, operation=%s, target_id=%s, original_value=%s, new_value=%s",
					event[objects.FieldKeyID], eventType, operation, targetID, originalValue, newValue)
				return true
			}
			return false
		},
		20*time.Second,
		100*time.Millisecond,
	)

	if !found {
		t.Fatalf("Hash mismatch fix audit event was not created in storage")
	}

	// Check audit metrics
	metricsCollector := storage.GetGlobalAuditMetricsCollector()
	snapshot := metricsCollector.GetSnapshot()
	t.Logf("Audit metrics: created=%d, failed=%d, validated=%d, skipped=%d",
		snapshot.EventsCreated, snapshot.EventsFailed, snapshot.EventsValidated, snapshot.EventsSkipped)
}
