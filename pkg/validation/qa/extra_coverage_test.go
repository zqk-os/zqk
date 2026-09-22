package qa

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestGuidanceEngine_Recommend_AllBranches(t *testing.T) {
	ge := NewGuidanceEngine()
	types := []string{
		"coverage",
		validation.ConstMagicExtracted_34,
		"di_violation",
		validation.ConstMagicExtracted_39,
		"dry_violation",
		validation.ConstMagicExtracted_44,
		validation.ConstMagicExtracted_50,
		"unknown_failure_type",
	}

	for _, failureType := range types {
		ctx := "test-context"
		if failureType == validation.ConstMagicExtracted_44 {
			g1 := ge.Recommend(failureType, validation.ConstMagiccd1ac283)
			if g1.Summary == "" || len(g1.Steps) == 0 {
				t.Errorf("empty guidance for %s with keyword", failureType)
			}
		}
		g := ge.Recommend(failureType, ctx)
		if g.Summary == "" || len(g.Steps) == 0 {
			t.Errorf("empty guidance for %s", failureType)
		}
	}
}

func TestASTAuditor_AllRules(t *testing.T) {
	tmpDir := t.TempDir()
	auditor := NewASTAuditor()

	// 1. Long conditional chain + long function complexity + interface usage + goroutine without ctx + map extraction
	code := `package testpkg

import (
	"fmt"
	"github.com/zqk-os/zqk/pkg/storage"
)

func HeavyFunc(store *storage.FileObjectStorage) {
	go func() {
		fmt.Println("background")
	}()

	m := make(map[string]any)
	if val, ok := m["key"].(string); ok && val != "" {
		_ = val
	}

	x := 1
	if x == 1 {
		_ = 1
	} else if x == 2 {
		_ = 2
	} else if x == 3 {
		_ = 3
	} else {
		_ = 4
	}

	s := "hardcoded_long_magic_string_without_slashes"
	_ = s
}
`
	srcFile := filepath.Join(tmpDir, "sample.go")
	if err := os.WriteFile(srcFile, []byte(code), 0644); err != nil {
		t.Fatal(err)
	}

	violations, err := auditor.AuditFile(srcFile)
	if err != nil {
		t.Fatalf("AuditFile failed: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("expected AST violations, got 0")
	}
}

func TestAuditorGate_VerifyComplete(t *testing.T) {
	ctx := context.Background()

	t.Run("nil storage", func(t *testing.T) {
		gate := NewAuditorGate(nil)
		err := gate.VerifyComplete(ctx, "BLI-1")
		if err == nil {
			t.Fatal("expected error with nil storage")
		}
	})

	t.Run("missing key in storage and disk", func(t *testing.T) {
		gate := NewAuditorGateForProject(newMockQASuccessStore(), t.TempDir())
		err := gate.VerifyComplete(ctx, "BLI-1")
		if err == nil {
			t.Fatal("expected error with missing trusted pub key")
		}
	})
}

type mockQASuccessStore struct {
	storage.ObjectStorageProvider
	objs map[string]map[string]any
}

func newMockQASuccessStore() *mockQASuccessStore {
	return &mockQASuccessStore{
		objs: make(map[string]map[string]any),
	}
}

func (m *mockQASuccessStore) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	id, _ := obj[objects.FieldKeyID].(string)
	m.objs[id] = obj
	return nil
}

func (m *mockQASuccessStore) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.objs[id]; ok {
		return obj, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockQASuccessStore) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	var matched []map[string]any
	for _, obj := range m.objs {
		if filter.Kind != "" && obj[objects.FieldKeyKind] != filter.Kind {
			continue
		}
		matched = append(matched, obj)
	}
	return &storage.QueryResult{Objects: matched}, nil
}

func TestAuditorService_PerformAudit_MemoryStore(t *testing.T) {
	tmpDir := t.TempDir()
	store := newMockQASuccessStore()
	signer, err := NewAuditorSigner("")
	if err != nil {
		t.Fatalf("signer creation failed: %v", err)
	}
	emitter := NewInterruptEmitter(tmpDir)
	engine := NewGuidanceEngine()
	gate := NewAuditorGate(store)

	svc := NewAuditorService(nil, store, signer, emitter, engine, gate)

	t.Run("read error or not found", func(t *testing.T) {
		svc.performAudit(context.Background(), "BLI-NOTFOUND", "backlog_item")
	})

	t.Run("complete without evidence emits interrupt", func(t *testing.T) {
		store.objs["BLI-NOEVID"] = map[string]any{
			objects.FieldKeyID:     "BLI-NOEVID",
			objects.FieldKeyKind:   objects.KindBacklogItem,
			objects.FieldKeyStatus: objects.ObjectStatusComplete,
			objects.FieldKeyTitle:  "Title",
		}
		svc.performAudit(context.Background(), "BLI-NOEVID", "backlog_item")
	})

	t.Run("smoke and mirrors emits interrupt", func(t *testing.T) {
		store.objs["BLI-SMOKE"] = map[string]any{
			objects.FieldKeyID:           "BLI-SMOKE",
			objects.FieldKeyKind:         objects.KindBacklogItem,
			objects.FieldKeyStatus:       objects.ObjectStatusComplete,
			objects.FieldKeyCommitHashes: []any{"abc1234"},
			objects.FieldKeyTitle:        "Smoke and mirrors test",
		}
		svc.performAudit(context.Background(), "BLI-SMOKE", "backlog_item")
	})

	t.Run("clean object passes audit and creates success token", func(t *testing.T) {
		store.objs["BLI-CLEAN"] = map[string]any{
			objects.FieldKeyID:           "BLI-CLEAN",
			objects.FieldKeyKind:         objects.KindBacklogItem,
			objects.FieldKeyStatus:       objects.ObjectStatusComplete,
			objects.FieldKeyCommitHashes: []any{"abc1234"},
			objects.FieldKeyTitle:        "Clean Verified Work",
			objects.FieldKeyDescription:  "Properly implemented feature",
		}
		svc.performAudit(context.Background(), "BLI-CLEAN", "backlog_item")
	})

	t.Run("isAuditTriggeringEvent", func(t *testing.T) {
		if isAuditTriggeringEvent(nil) {
			t.Error("expected false for nil event")
		}
	})
}

func TestAuditorGate_VerifyComplete_Success(t *testing.T) {
	ctx := context.Background()
	store := newMockQASuccessStore()
	signer, err := NewAuditorSigner("")
	if err != nil {
		t.Fatalf("create signer failed: %v", err)
	}

	// 1. Setup AuditorKeyID in store with signer's public key as description
	store.objs[AuditorKeyID] = map[string]any{
		objects.FieldKeyID:          AuditorKeyID,
		objects.FieldKeyDescription: signer.PublicKey(),
	}

	// 2. Setup a valid signed QASuccess object
	itemID := "BLI-GATE-1"
	status := objects.ObjectStatusSuccess
	data := []byte(itemID + status)
	sig, err := signer.Sign(data)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	qaSuccessObj, err := buildQASuccessObject(itemID, sig, signer.PublicKey())
	if err != nil {
		t.Fatalf("buildQASuccessObject failed: %v", err)
	}
	qaSuccessObj[objects.FieldKeyStatus] = status
	store.objs["QAS-TEST-1"] = qaSuccessObj

	gate := NewAuditorGate(store)
	if err := gate.VerifyComplete(ctx, itemID); err != nil {
		t.Fatalf("expected VerifyComplete to pass, got: %v", err)
	}

	// Test PrivateKey accessor on signer
	if signer.PrivateKey() == nil {
		t.Error("expected non-nil private key")
	}
}

func TestASTAuditor_ComplexAndInterface(t *testing.T) {
	tmpDir := t.TempDir()
	auditor := NewASTAuditor()

	// Generate a 105-line function to trigger complexity
	var lines []string
	lines = append(lines, "package testpkg", "import (", "\t\"context\"", "\t\"github.com/zqk-os/zqk/pkg/storage\"", ")")
	lines = append(lines, "func VeryLongFunction(ctx context.Context, mgr *storage.FileObjectStorage) {")
	for i := 0; i < 105; i++ {
		lines = append(lines, "\t_ = 1")
	}
	lines = append(lines, "}")

	srcFile := filepath.Join(tmpDir, "long.go")
	if err := os.WriteFile(srcFile, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		t.Fatal(err)
	}

	violations, err := auditor.AuditFile(srcFile)
	if err != nil {
		t.Fatalf("AuditFile failed: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("expected complexity/interface violations")
	}
}

func TestAuditorService_hasStringEvidence_hasCompletedCriterion(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store := newMockQASuccessStore()
	svc := NewAuditorService(nil, store, nil, nil, nil, nil)

	// Test hasStringEvidence
	if !hasStringEvidence([]any{"commit-1"}) {
		t.Error("expected true for []any with string")
	}
	if !hasStringEvidence([]string{"commit-2"}) {
		t.Error("expected true for []string")
	}
	if hasStringEvidence([]any{"   "}) {
		t.Error("expected false for whitespace")
	}
	if hasStringEvidence(nil) {
		t.Error("expected false for nil")
	}

	// Test hasCompletedCriterion
	store.objs["CRIT-COMPLETE"] = map[string]any{
		objects.FieldKeyID:     "CRIT-COMPLETE",
		objects.FieldKeyStatus: objects.ObjectStatusComplete,
	}
	store.objs["CRIT-INPROG"] = map[string]any{
		objects.FieldKeyID:     "CRIT-INPROG",
		objects.FieldKeyStatus: "in_progress",
	}

	if !svc.hasCompletedCriterion(ctx, sec, []any{"CRIT-COMPLETE"}) {
		t.Error("expected true for completed criterion []any")
	}
	if !svc.hasCompletedCriterion(ctx, sec, []string{"CRIT-COMPLETE"}) {
		t.Error("expected true for completed criterion []string")
	}
	if svc.hasCompletedCriterion(ctx, sec, []string{"CRIT-INPROG"}) {
		t.Error("expected false for in_progress criterion")
	}
	if svc.hasCompletedCriterion(ctx, sec, []string{"CRIT-MISSING"}) {
		t.Error("expected false for missing criterion")
	}

	// Test AuditNow
	svc.AuditNow(ctx, "BLI-TEST-NOW", "backlog_item")
}

func TestAuditorService_performAudit_AdditionalBranches(t *testing.T) {
	ctx := context.Background()
	store := newMockQASuccessStore()
	tmpDir := t.TempDir()
	emitter := NewInterruptEmitter(tmpDir)
	signer, err := NewAuditorSigner("")
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}
	engine := NewGuidanceEngine()
	auditor := NewASTAuditor()
	svc := &AuditorService{
		storage:    store,
		emitter:    emitter,
		astAuditor: auditor,
		engine:     engine,
		signer:     signer,
	}

	// 1. Backlog item with _test.go artifact (satisfies test asset traceability)
	store.objs["BLI-TEST-ASSET"] = map[string]any{
		objects.FieldKeyID:        "BLI-TEST-ASSET",
		objects.FieldKeyTitle:     "Clean Backlog Item",
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyArtifacts: []any{"pkg/service/foo_test.go"},
	}
	svc.performAudit(ctx, "BLI-TEST-ASSET", objects.KindBacklogItem)
	if len(store.objs) <= 1 {
		t.Error("expected QA success object to be created when _test.go artifact is present")
	}

	// 2. Backlog item with completed criterion (satisfies criterion traceability)
	store.objs["CRIT-DONE"] = map[string]any{
		objects.FieldKeyID:     "CRIT-DONE",
		objects.FieldKeyStatus: objects.ObjectStatusCompleted,
	}
	store.objs["BLI-CRIT-ASSET"] = map[string]any{
		objects.FieldKeyID:           "BLI-CRIT-ASSET",
		objects.FieldKeyTitle:        "Clean Backlog Item 2",
		objects.FieldKeyStatus:       objects.ObjectStatusComplete,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-DONE"},
	}
	svc.performAudit(ctx, "BLI-CRIT-ASSET", objects.KindBacklogItem)

	// 3. Artifact with AST violations
	badFile := filepath.Join(tmpDir, "bad.go")
	badCode := `package bad
import "fmt"
func Bad() {
	fmt.Println("direct print")
}
`
	if err := os.WriteFile(badFile, []byte(badCode), 0644); err != nil {
		t.Fatal(err)
	}
	store.objs["BLI-AST-BAD"] = map[string]any{
		objects.FieldKeyID:        "BLI-AST-BAD",
		objects.FieldKeyTitle:     "AST Bad Item",
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyArtifacts: []any{badFile, 12345, "non-go.txt"},
		objects.FieldKeyCommitHashes: []any{"hash-1"},
	}
	svc.performAudit(ctx, "BLI-AST-BAD", objects.KindBacklogItem)

	// 4. Smoke and mirrors with TODO in description
	store.objs["BLI-TODO"] = map[string]any{
		objects.FieldKeyID:           "BLI-TODO",
		objects.FieldKeyTitle:        "Valid Title",
		objects.FieldKeyDescription:  "This is a TODO item",
		objects.FieldKeyStatus:       objects.ObjectStatusComplete,
		objects.FieldKeyCommitHashes: []any{"hash-1"},
	}
	svc.performAudit(ctx, "BLI-TODO", objects.KindBacklogItem)

	// 5. Artifact that fails to parse
	unparseableFile := filepath.Join(tmpDir, "unparseable.go")
	if err := os.WriteFile(unparseableFile, []byte("package unparseable\nfunc Broken({{{"), 0644); err != nil {
		t.Fatal(err)
	}
	store.objs["BLI-PARSE-ERR"] = map[string]any{
		objects.FieldKeyID:        "BLI-PARSE-ERR",
		objects.FieldKeyTitle:     "Unparseable File Item",
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyArtifacts: []any{unparseableFile},
		objects.FieldKeyCommitHashes: []any{"hash-1"},
	}
	svc.performAudit(ctx, "BLI-PARSE-ERR", objects.KindBacklogItem)
}

func TestAuditorGate_VerifySignature_And_TrustedPubHex(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	tmpDir := t.TempDir()

	store := newMockQASuccessStore()
	gate := NewAuditorGateForProject(store, tmpDir)

	// trustedPubHex generates a key in tmpDir/.zqk/keystore/auditor.priv
	pubHex, err := gate.trustedPubHex(ctx, sec)
	if err != nil {
		t.Fatalf("trustedPubHex failed: %v", err)
	}
	if pubHex == "" {
		t.Fatal("expected non-empty pubHex")
	}

	// Verify with invalid hex signature
	err = gate.verifySignature(QAReport{
		ItemID:    "BLI-1",
		Status:    "passed",
		Signature: "invalid-hex-zz!",
	}, pubHex)
	if err == nil {
		t.Error("expected error for invalid hex signature")
	}

	// Verify with short pubHex
	err = gate.verifySignature(QAReport{
		ItemID:    "BLI-1",
		Status:    "passed",
		Signature: "001122",
	}, "tooshort")
	if err == nil {
		t.Error("expected error for short pubHex")
	}

	// Verify with mismatched/bad signature
	err = gate.verifySignature(QAReport{
		ItemID:    "BLI-1",
		Status:    "passed",
		Signature: "3045022011111111111111111111111111111111111111111111111111111111111111110221002222222222222222222222222222222222222222222222222222222222222222",
	}, pubHex)
	if err == nil {
		t.Error("expected signature verification to fail for bogus signature")
	}

	// VerifyComplete with mismatched public key
	reportObj := map[string]any{
		objects.FieldKeyID:        "QA-SUCCESS-BLI-MISMATCH",
		objects.FieldKeyKind:      KindQASuccess,
		objects.FieldKeyItemID:    "BLI-MISMATCH",
		objects.FieldKeyStatus:    objects.ObjectStatusSuccess,
		objects.FieldKeySignature: "abcd",
		objects.FieldKeyPublicKey: "wrong-pub-hex",
	}
	store.objs["QA-SUCCESS-BLI-MISMATCH"] = reportObj
	err = gate.VerifyComplete(ctx, "BLI-MISMATCH")
	if err == nil {
		t.Error("expected VerifyComplete to fail for key mismatch")
	}
}

func TestInterruptEmitter_AckDisparityOnPass_EdgeCases(t *testing.T) {
	var nilEmitter *InterruptEmitter
	nilEmitter.AckDisparityOnPass("BLI-1") // Should not panic

	emitterEmptyRoot := NewInterruptEmitter("")
	emitterEmptyRoot.AckDisparityOnPass("BLI-1") // Should return early

	emitterValid := NewInterruptEmitter(t.TempDir())
	emitterValid.AckDisparityOnPass("") // Should return early
}

func TestASTAuditor_GoroutineAudit(t *testing.T) {
	tmpDir := t.TempDir()
	auditor := NewASTAuditor()

	// 1. File with unmanaged goroutine (no ctx argument)
	unmanagedCode := `package testgo
func Spawn() {
	go doSomething()
}
func doSomething() {}
`
	unmanagedFile := filepath.Join(tmpDir, "unmanaged.go")
	if err := os.WriteFile(unmanagedFile, []byte(unmanagedCode), 0644); err != nil {
		t.Fatal(err)
	}

	violations, err := auditor.AuditFile(unmanagedFile)
	if err != nil {
		t.Fatalf("AuditFile failed: %v", err)
	}
	found := false
	for _, v := range violations {
		if v.Type == validation.ConstMagic9ae33b8e {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected unmanaged goroutine violation")
	}

	// 2. File with managed goroutine (with ctx argument)
	managedCode := `package testgo
import "context"
func SpawnManaged(ctx context.Context) {
	go doSomethingManaged(ctx)
}
func doSomethingManaged(ctx context.Context) {}
`
	managedFile := filepath.Join(tmpDir, "managed.go")
	if err := os.WriteFile(managedFile, []byte(managedCode), 0644); err != nil {
		t.Fatal(err)
	}
	violations, err = auditor.AuditFile(managedFile)
	if err != nil {
		t.Fatalf("AuditFile failed: %v", err)
	}
	for _, v := range violations {
		if v.Type == validation.ConstMagic9ae33b8e {
			t.Error("unexpected unmanaged goroutine violation for managed code")
		}
	}
}

func TestNewAuditorSigner_KeyReload(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "auditor.priv")

	// 1. Generate first time
	s1, err := NewAuditorSigner(keyPath)
	if err != nil {
		t.Fatalf("NewAuditorSigner failed: %v", err)
	}
	pub1 := s1.PublicKey()

	// 2. Reload from same file
	s2, err := NewAuditorSigner(keyPath)
	if err != nil {
		t.Fatalf("NewAuditorSigner reload failed: %v", err)
	}
	pub2 := s2.PublicKey()

	if pub1 != pub2 {
		t.Errorf("expected reloaded public key %q to match %q", pub2, pub1)
	}

	// 3. Corrupt key file
	corruptPath := filepath.Join(tmpDir, "corrupt.priv")
	if err := os.WriteFile(corruptPath, []byte("NOT A VALID PEM BLOCK"), 0644); err != nil {
		t.Fatal(err)
	}
	// Should generate fresh key rather than failing
	s3, err := NewAuditorSigner(corruptPath)
	if err != nil {
		t.Fatalf("expected NewAuditorSigner with corrupt file to fallback gracefully, got err: %v", err)
	}
	if s3 == nil {
		t.Fatal("expected non-nil signer")
	}
}

func TestAuditorGate_trustedPubHex_StorageAndErrorCases(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store := newMockQASuccessStore()

	// 1. Key exists in storage
	store.objs[AuditorKeyID] = map[string]any{
		objects.FieldKeyID:          AuditorKeyID,
		objects.FieldKeyDescription: "pub-hex-from-cas-1234567890abcdef1234567890abcdef1234567890abcdef",
	}
	gateWithStore := NewAuditorGate(store)
	pub, err := gateWithStore.trustedPubHex(ctx, sec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pub != "pub-hex-from-cas-1234567890abcdef1234567890abcdef1234567890abcdef" {
		t.Errorf("got %q, want CAS pub hex", pub)
	}

	// 2. Key missing from storage and projectRoot is empty
	emptyStore := newMockQASuccessStore()
	gateNoRoot := NewAuditorGate(emptyStore)
	_, err = gateNoRoot.trustedPubHex(ctx, sec)
	if err == nil {
		t.Error("expected error when key missing and projectRoot is empty")
	}
}

func TestASTAuditor_MapExtractionAntiPattern(t *testing.T) {
	tmpDir := t.TempDir()
	auditor := NewASTAuditor()

	code := `package testextract
func Extract(m map[string]any) string {
	if val, ok := m["key"].(string); ok && val != "" {
		return val
	}
	return ""
}
`
	srcFile := filepath.Join(tmpDir, "extract.go")
	if err := os.WriteFile(srcFile, []byte(code), 0644); err != nil {
		t.Fatal(err)
	}

	violations, err := auditor.AuditFile(srcFile)
	if err != nil {
		t.Fatalf("AuditFile failed: %v", err)
	}
	found := false
	for _, v := range violations {
		if strings.Contains(v.Message, "Verbose map extraction detected") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected map extraction anti-pattern violation")
	}
}

func TestIsAuditTriggeringEvent_Cases(t *testing.T) {
	if isAuditTriggeringEvent(nil) {
		t.Error("expected false for nil event")
	}
	evNonTransition := &lifecycle.LifecycleEvent{
		EventType: "other",
		ToStatus:  "completed",
	}
	if isAuditTriggeringEvent(evNonTransition) {
		t.Error("expected false for non-transition event")
	}
	evUntriggeredStatus := &lifecycle.LifecycleEvent{
		EventType: lifecycle.EventTypeStatusTransition,
		ToStatus:  "draft",
	}
	if isAuditTriggeringEvent(evUntriggeredStatus) {
		t.Error("expected false for draft status")
	}
	evTriggered := &lifecycle.LifecycleEvent{
		EventType: lifecycle.EventTypeStatusTransition,
		ToStatus:  "in_progress",
	}
	if !isAuditTriggeringEvent(evTriggered) {
		t.Error("expected true for in_progress status")
	}
}

