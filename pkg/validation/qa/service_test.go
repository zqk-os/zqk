package qa

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/lifecycle"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/policyinterrupt"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/validation"
)

func newTraceabilityAuditorTest(
	t *testing.T,
) (*AuditorService, storage.ObjectStorageProvider, context.Context, *pkgctx.SecurityContext, string) {
	t.Helper()

	tmpDir := t.TempDir()
	wal, err := lifecycle.GetOrCreateLifecycleWAL(tmpDir)
	if err != nil {
		t.Fatalf("create lifecycle WAL: %v", err)
	}
	pool := testkit.PrepareGraphConnectionForTest(t)
	objectStorage := storage.NewPoolAwareGraphStorage(pool, tmpDir)
	signer, err := NewAuditorSigner("")
	if err != nil {
		t.Fatalf("create auditor signer: %v", err)
	}
	service := NewAuditorService(
		wal,
		objectStorage,
		signer,
		NewInterruptEmitter(tmpDir),
		NewGuidanceEngine(),
		NewAuditorGate(objectStorage),
	)
	return service, objectStorage, pkgctx.NewSystemContext(), pkgctx.NewSystemSecurityContext(), tmpDir
}

func latestCriticalInterrupt(t *testing.T, projectRoot string) *policyinterrupt.InterruptRecord {
	t.Helper()

	acks, err := policyinterrupt.LoadAcksIncremental(projectRoot)
	if err != nil {
		t.Fatalf("load interrupt acknowledgements: %v", err)
	}
	latest, err := policyinterrupt.LoadLatestCriticalUnacked(projectRoot, acks)
	if err != nil {
		t.Fatalf("load policy interrupts: %v", err)
	}
	return latest
}

func qaFixtureID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func qaForceStatus(t *testing.T, store storage.ObjectStorageProvider, ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) {
	t.Helper()
	id, _ := obj[objects.FieldKeyID].(string)
	leave, _ := obj[objects.FieldKeyStatus].(string)
	force := pkgctx.WithLifecycleBreakGlass(ctx, "qa auditor fixture")
	force = pkgctx.WithAllowCoreObjectDelete(force)
	create := make(map[string]any, len(obj))
	for k, v := range obj {
		create[k] = v
	}
	if kind, _ := create[objects.FieldKeyKind].(string); kind == objects.KindBacklogItem {
		if _, ok := create[objects.FieldKeyEstimatedEffort]; !ok {
			create[objects.FieldKeyEstimatedEffort] = "1h"
		}
	} else if kind == objects.KindCriteria {
		if _, ok := create[objects.FieldKeyCategory]; !ok {
			create[objects.FieldKeyCategory] = "acceptance"
		}
	}
	if leave != "" {
		create[objects.FieldKeyStatus] = leave
	}
	_ = store.Delete(storage.WithTestHardDelete(force), secCtx, id, true) //nolint:errcheck // shared Memgraph fixture reset
	if err := store.Create(force, secCtx, create); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
	// Create coerces non-preliminary status to lifecycle origin (draft-first).
	// The auditor's complete-evidence check only runs when Read sees `complete`.
	if leave == "" {
		return
	}
	got, err := store.Read(force, secCtx, id)
	if err != nil {
		t.Fatalf("read back %s: %v", id, err)
	}
	if objects.GetString(got, objects.FieldKeyStatus) == leave {
		return
	}
	if err := store.Update(force, secCtx, id, map[string]any{objects.FieldKeyStatus: leave}); err != nil {
		t.Fatalf("force status %s → %s: %v", id, leave, err)
	}
	got, err = store.Read(force, secCtx, id)
	if err != nil {
		t.Fatalf("read back after force %s: %v", id, err)
	}
	if objects.GetString(got, objects.FieldKeyStatus) != leave {
		t.Fatalf("qaForceStatus %s: want status %s, got %s", id, leave, objects.GetString(got, objects.FieldKeyStatus))
	}
}

func TestAuditorService_PerformAudit_CompleteUsesSupportedEvidence(t *testing.T) {
	t.Run("commit reference", func(t *testing.T) {
		service, objectStorage, ctx, secCtx, tmpDir := newTraceabilityAuditorTest(t)
		bliID := qaFixtureID("BLI-COMMIT-EVIDENCE")
		qaForceStatus(t, objectStorage, ctx, secCtx, map[string]any{
			objects.FieldKeyID:         bliID,
			objects.FieldKeyKind:       "backlog_item",
			objects.FieldKeyTitle:      "Complete with traceability",
			objects.FieldKeyStatus:     objects.ObjectStatusComplete,
			objects.FieldKeyCommitRefs: []any{"abc123"},
		})

		service.performAudit(context.Background(), bliID, "backlog_item")

		if latest := latestCriticalInterrupt(t, tmpDir); latest != nil {
			t.Fatalf("commit_refs should satisfy traceability, got interrupt: %+v", latest)
		}
	})

	t.Run("completed criterion", func(t *testing.T) {
		service, objectStorage, ctx, secCtx, tmpDir := newTraceabilityAuditorTest(t)
		critID := qaFixtureID("CRIT-COMPLETE-EVIDENCE")
		bliID := qaFixtureID("BLI-CRITERION-EVIDENCE")
		qaForceStatus(t, objectStorage, ctx, secCtx, map[string]any{
			objects.FieldKeyID:     critID,
			objects.FieldKeyKind:   "criteria",
			objects.FieldKeyTitle:  "Verified criterion",
			objects.FieldKeyStatus: objects.ObjectStatusComplete,
		})
		qaForceStatus(t, objectStorage, ctx, secCtx, map[string]any{
			objects.FieldKeyID:           bliID,
			objects.FieldKeyKind:         "backlog_item",
			objects.FieldKeyTitle:        "Complete with verified criterion",
			objects.FieldKeyStatus:       objects.ObjectStatusComplete,
			objects.FieldKeyCriteriaRefs: []any{critID},
		})

		service.performAudit(context.Background(), bliID, "backlog_item")

		if latest := latestCriticalInterrupt(t, tmpDir); latest != nil {
			t.Fatalf("completed criteria_refs should satisfy verification, got interrupt: %+v", latest)
		}
	})

	t.Run("missing evidence", func(t *testing.T) {
		service, objectStorage, ctx, secCtx, tmpDir := newTraceabilityAuditorTest(t)
		bliID := qaFixtureID("BLI-MISSING-EVIDENCE")
		qaForceStatus(t, objectStorage, ctx, secCtx, map[string]any{
			objects.FieldKeyID:     bliID,
			objects.FieldKeyKind:   "backlog_item",
			objects.FieldKeyTitle:  "Complete without evidence",
			objects.FieldKeyStatus: objects.ObjectStatusComplete,
		})

		service.performAudit(context.Background(), bliID, "backlog_item")

		latest := latestCriticalInterrupt(t, tmpDir)
		if latest == nil || latest.DedupeKey != "qa-disparity-"+bliID {
			t.Fatalf("missing evidence should emit the disparity interrupt, got: %+v", latest)
		}
	})

	t.Run("non backlog kind", func(t *testing.T) {
		service, objectStorage, ctx, secCtx, tmpDir := newTraceabilityAuditorTest(t)
		priID := qaFixtureID("PRI-NON-BACKLOG")
		qaForceStatus(t, objectStorage, ctx, secCtx, map[string]any{
			objects.FieldKeyID:     priID,
			objects.FieldKeyKind:   "priority_plan",
			objects.FieldKeyTitle:  "Completed plan",
			objects.FieldKeyStatus: objects.ObjectStatusComplete,
		})

		service.performAudit(context.Background(), priID, "priority_plan")

		if latest := latestCriticalInterrupt(t, tmpDir); latest != nil {
			t.Fatalf("backlog traceability rules must not apply to other kinds, got: %+v", latest)
		}
	})
}

func TestAuditorService_PerformAudit_Success(t *testing.T) {
	tmpDir := t.TempDir()
	wal, _ := lifecycle.GetOrCreateLifecycleWAL(tmpDir)

	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, tmpDir)
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	_ = realStorage.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID: "BLI-SUCCESS", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Good Item",
	})

	signer, _ := NewAuditorSigner("")
	emitter := NewInterruptEmitter(tmpDir)
	engine := NewGuidanceEngine()
	gate := NewAuditorGate(realStorage)

	svc := NewAuditorService(wal, realStorage, signer, emitter, engine, gate)

	svc.performAudit(context.Background(), "BLI-SUCCESS", "backlog_item")

	keyObj := map[string]any{
		objects.FieldKeyID:          AuditorKeyID,
		objects.FieldKeyKind:        objects.KindKeystoreEntry,
		objects.FieldKeyDescription: signer.PublicKey(),
		objects.FieldKeyAccountID:   "ACC-SYSTEM",
	}
	if existing, err := realStorage.Read(ctx, secCtx, AuditorKeyID); err == nil && existing != nil {
		keyObj[objects.FieldKeyUpdatedAt] = existing[objects.FieldKeyUpdatedAt]
		if err := realStorage.Update(ctx, secCtx, AuditorKeyID, keyObj); err != nil {
			t.Fatalf("update auditor key: %v", err)
		}
	} else if err := realStorage.Create(ctx, secCtx, keyObj); err != nil {
		t.Fatalf("create auditor key: %v", err)
	}

	res, err := realStorage.List(ctx, secCtx, nil, storage.ListFilter{
		Kind: KindQASuccess,
		Filters: map[string]any{
			objects.FieldKeyItemID: "BLI-SUCCESS",
			objects.FieldKeyStatus: objects.ObjectStatusSuccess,
		},
	})
	if err != nil {
		t.Fatalf("list qa_success: %v", err)
	}
	if len(res.Objects) == 0 {
		t.Fatal("expected qa_success with status=success")
	}
	if st, _ := res.Objects[0][objects.FieldKeyStatus].(string); st != objects.ObjectStatusSuccess {
		t.Fatalf("qa_success status=%q want %q", st, objects.ObjectStatusSuccess)
	}
	wantTitle := QASuccessTitle("BLI-SUCCESS")
	if got, _ := res.Objects[0][objects.FieldKeyTitle].(string); got != wantTitle {
		t.Fatalf("qa_success title=%q want %q", got, wantTitle)
	}
	if err := gate.VerifyComplete(ctx, "BLI-SUCCESS"); err != nil {
		t.Fatalf("VerifyComplete: %v", err)
	}
}

func TestAuditorService_PerformAudit_HITLTrigger(t *testing.T) {
	tmpDir := t.TempDir()
	wal, _ := lifecycle.GetOrCreateLifecycleWAL(tmpDir)

	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, tmpDir)
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	_ = realStorage.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID: "BLI-SMOKE", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: validation.ConstMagic6183a95d,
	})

	signer, _ := NewAuditorSigner("")
	emitter := NewInterruptEmitter(tmpDir)
	engine := NewGuidanceEngine()
	gate := NewAuditorGate(realStorage)

	svc := NewAuditorService(wal, realStorage, signer, emitter, engine, gate)

	// Run audit
	svc.performAudit(context.Background(), "BLI-SMOKE", "backlog_item")

	// Verify HITL emitted
	acks, _ := policyinterrupt.LoadAcksIncremental(tmpDir)
	latest, _ := policyinterrupt.LoadLatestCriticalUnacked(tmpDir, acks)

	if latest == nil || latest.DedupeKey != validation.ConstMagic59265a57 {
		t.Errorf(validation.ConstMagic40b1c9a4)
	}
}

func TestAuditorService_PerformAudit_ASTViolation(t *testing.T) {
	tmpDir := t.TempDir()
	wal, _ := lifecycle.GetOrCreateLifecycleWAL(tmpDir)

	violationFile := filepath.Join(tmpDir, "violation_example.go")
	_ = fileutil.WriteSecureFile(violationFile, []byte("package main\nimport \"fmt\"\nfunc do() { fmt.Println(\"hi\") }"))

	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, tmpDir)
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	testID := fmt.Sprintf("BLI-AST-FAIL-%d", time.Now().UnixNano())

	err := realStorage.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:        testID,
		objects.FieldKeyKind:      "backlog_item",
		objects.FieldKeyTitle:     "Broken Item",
		objects.FieldKeyArtifacts: []any{violationFile},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	signer, _ := NewAuditorSigner("")
	emitter := NewInterruptEmitter(tmpDir)
	engine := NewGuidanceEngine()
	gate := NewAuditorGate(realStorage)

	svc := NewAuditorService(wal, realStorage, signer, emitter, engine, gate)

	// Run audit
	svc.performAudit(context.Background(), testID, "backlog_item")

	// Verify HITL emitted for AST violation
	acks, _ := policyinterrupt.LoadAcksIncremental(tmpDir)
	latest, _ := policyinterrupt.LoadLatestCriticalUnacked(tmpDir, acks)

	if latest == nil || !strings.Contains(latest.Message, "AST violations") {
		t.Errorf(validation.ConstMagicc1e81cb3, latest)
	}
}
