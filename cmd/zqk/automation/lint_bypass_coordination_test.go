package automation

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1" // Register instance builders
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2"          // Register spec builders
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestLintBypassAuditEventViaCoordinator verifies that lint bypass audit events are created and
// persisted. It uses StorageAuditRouter directly (sync) for deterministic verification; production
// uses emitLintBypassAuditEventViaCoordinator which emits via the coordinator (async).
func TestLintBypassAuditEventViaCoordinator(t *testing.T) {
	// Not t.Parallel(): t.Setenv(ZQK_TEST_ROOT) is invalid after Parallel.
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "automation.lint_bypass",
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			return []testkit.NamedTestStep{
				{Name: "ensure_path_alias_cache", Fn: func() error { return storage.EnsurePathAliasCacheReady(root) }},
				{Name: "lint_bypass_generate_specs", Fn: func() error {
					specsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
					if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
						return err
					}
					return builders.NewSpecGenerator(specsDir).GenerateAllSpecs()
				}},
			}
		},
	})
	testDir := proj.Root
	fileStorage := proj.FileStorage

	// Disable global audit event buffer for tests
	buffer := storage.GetGlobalAuditEventBuffer()
	buffer.SetEnabled(false)
	if err := buffer.Flush(); err != nil {
		t.Logf("Warning: Failed to flush buffer: %v", err)
	}

	// Test data (matches production lint bypass payload)
	commitMessage := "test: bypass lint checks"

	// Emit lint bypass audit event via storage audit router (sync) so test is deterministic
	ctx := pkgctx.NewSystemContext()
	auditRouter := coordination.NewStorageAuditRouter(testDir, fileStorage)
	operation := "Lint checks bypassed with --no-verify flag"
	if commitMessage != emptyValue {
		msg := commitMessage
		if len(msg) > 100 {
			msg = msg[:100] + "..."
		}
		operation = fmt.Sprintf("Lint checks bypassed: %s", msg)
	}
	eventCtx := coordination.NewEventContext("lint_bypass_test", automationEventTypeLintBypass, automationStatusComplete).
		WithEventData(&coordination.EventData{
			AuditMetadata: map[string]any{
				automationAuditKeyEventType:   automationAuditCodeQualityBypass,
				automationAuditKeyOperation:   operation,
				automationAuditKeySeverity:    automationSeverityMedium,
				automationAuditKeyTargetKind:  automationTargetKindLint,
				automationAuditKeySource:      automationSourceGitHook,
				automationAuditKeyProjectRoot: testDir,
			},
		})
	if err := auditRouter.Emit(ctx, eventCtx); err != nil {
		t.Fatalf("Audit router emit: %v", err)
	}

	// Flush buffer and verify event in storage (router.Emit is synchronous)
	if err := buffer.Flush(); err != nil {
		t.Logf("Warning: Failed to flush buffer: %v", err)
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{
		Kind:  objects.KindAuditEvent,
		Limit: 10,
	}
	listResult, err := fileStorage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Fatalf("Failed to list audit events: %v", err)
	}
	var found bool
	for _, event := range listResult.Objects {
		eventType, _ := event[automationAuditKeyEventType].(string)
		op, _ := event[automationAuditKeyOperation].(string)
		targetKind, _ := event[automationAuditKeyTargetKind].(string)
		if eventType == automationAuditCodeQualityBypass &&
			targetKind == automationTargetKindLint &&
			(op != emptyValue && strings.Contains(op, "Lint checks bypassed")) {
			found = true
			t.Logf("Found lint bypass audit event: id=%v, event_type=%s, operation=%s",
				event[objects.FieldKeyID], eventType, op)
			break
		}
	}
	if !found {
		t.Error("Lint bypass audit event was not created in storage")
		for i, event := range listResult.Objects {
			t.Logf("Event %d: %+v", i, event)
		}
	}

	// Check audit metrics
	metricsCollector := storage.GetGlobalAuditMetricsCollector()
	snapshot := metricsCollector.GetSnapshot()
	t.Logf("Audit metrics: created=%d, failed=%d, validated=%d, skipped=%d",
		snapshot.EventsCreated, snapshot.EventsFailed, snapshot.EventsValidated, snapshot.EventsSkipped)
}
