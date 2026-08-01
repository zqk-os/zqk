package qa

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/lifecycle"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/policyinterrupt"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/validation"
)

func TestAuditorService_PerformAudit_Success(t *testing.T) {
	tmpDir := t.TempDir()
	wal, _ := lifecycle.GetOrCreateLifecycleWAL(tmpDir)

	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, tmpDir)
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	_ = realStorage.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID: "ITEM-SUCCESS", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Good Item",
	})

	signer, _ := NewAuditorSigner("")
	emitter := NewInterruptEmitter(tmpDir)
	engine := NewGuidanceEngine()
	gate := NewAuditorGate(realStorage)

	svc := NewAuditorService(wal, realStorage, signer, emitter, engine, gate)

	// Run audit
	svc.performAudit(context.Background(), "ITEM-SUCCESS", "backlog_item")

	// Verify QASuccess created in storage
}

func TestAuditorService_PerformAudit_HITLTrigger(t *testing.T) {
	tmpDir := t.TempDir()
	wal, _ := lifecycle.GetOrCreateLifecycleWAL(tmpDir)

	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, tmpDir)
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	_ = realStorage.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID: "ITEM-SMOKE", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: validation.ConstMagic6183a95d,
	})

	signer, _ := NewAuditorSigner("")
	emitter := NewInterruptEmitter(tmpDir)
	engine := NewGuidanceEngine()
	gate := NewAuditorGate(realStorage)

	svc := NewAuditorService(wal, realStorage, signer, emitter, engine, gate)

	// Run audit
	svc.performAudit(context.Background(), "ITEM-SMOKE", "backlog_item")

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

	_ = realStorage.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:        "ITEM-AST-FAIL",
		objects.FieldKeyKind:      "backlog_item",
		objects.FieldKeyTitle:     "Broken Item",
		objects.FieldKeyArtifacts: []any{violationFile},
	})

	signer, _ := NewAuditorSigner("")
	emitter := NewInterruptEmitter(tmpDir)
	engine := NewGuidanceEngine()
	gate := NewAuditorGate(realStorage)

	svc := NewAuditorService(wal, realStorage, signer, emitter, engine, gate)

	// Run audit
	svc.performAudit(context.Background(), "ITEM-AST-FAIL", "backlog_item")

	// Verify HITL emitted for AST violation
	acks, _ := policyinterrupt.LoadAcksIncremental(tmpDir)
	latest, _ := policyinterrupt.LoadLatestCriticalUnacked(tmpDir, acks)

	if latest == nil || !strings.Contains(latest.Message, "AST violations") {
		t.Errorf(validation.ConstMagicc1e81cb3, latest)
	}
}
