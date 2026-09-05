package storage

import (
	"context"
	"errors"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

type mockProvider struct {
	ObjectStorageProvider
	calledRead  bool
	calledWrite bool
	failWrite   bool
	failRead    bool
	writeErr    error
	readErr     error
	lastID      string
	lastObj     map[string]any
	bulkResult  *BulkResult
	tx          *mockTransaction
}

func (m *mockProvider) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	if m.failRead {
		return false, m.readErr
	}
	return true, nil
}

func (m *mockProvider) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	m.calledWrite = true
	m.lastObj = obj
	if m.failWrite {
		return m.writeErr
	}
	return nil
}

func (m *mockProvider) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	m.calledRead = true
	m.lastID = id
	if m.failRead {
		return nil, m.readErr
	}
	return map[string]any{objects.FieldKeyID: id}, nil
}

func (m *mockProvider) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	m.calledWrite = true
	m.lastID = id
	if m.failWrite {
		return m.writeErr
	}
	return nil
}

func (m *mockProvider) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	m.calledWrite = true
	m.lastID = id
	if m.failWrite {
		return m.writeErr
	}
	return nil
}

func (m *mockProvider) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objects []map[string]any) (*BulkResult, error) {
	m.calledWrite = true
	if m.failWrite {
		return nil, m.writeErr
	}
	return m.bulkResult, nil
}

func (m *mockProvider) Move(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, newKind string, updateReferences bool) error {
	m.calledWrite = true
	m.lastID = id
	if m.failWrite {
		return m.writeErr
	}
	return nil
}

func (m *mockProvider) Rename(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID string, updateReferences bool) error {
	m.calledWrite = true
	m.lastID = oldID
	if m.failWrite {
		return m.writeErr
	}
	return nil
}

func TestHybridObjectStorage_Read(t *testing.T) {
	primary := &mockProvider{}
	secondary := &mockProvider{}
	hybrid := NewHybridObjectStorage(primary, secondary)

	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{}

	obj, err := hybrid.Read(ctx, secCtx, "test-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obj == nil {
		t.Fatal("expected object, got nil")
	}
	if !primary.calledRead {
		t.Error("expected primary.Read to be called")
	}
	if secondary.calledRead {
		t.Error("expected secondary.Read NOT to be called")
	}
	if primary.lastID != "test-id" {
		t.Errorf("expected ID 'test-id', got '%s'", primary.lastID)
	}
}

func TestHybridObjectStorage_Create(t *testing.T) {
	primary := &mockProvider{}
	secondary := &mockProvider{}
	hybrid := NewHybridObjectStorage(primary, secondary)

	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{}
	obj := map[string]any{objects.FieldKeyKind: "task"}

	err := hybrid.Create(ctx, secCtx, obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !primary.calledWrite {
		t.Error("expected primary.Create to be called")
	}
	if !secondary.calledWrite {
		t.Error("expected secondary.Create to be called")
	}
}

func TestHybridObjectStorage_Create_PrimaryFail(t *testing.T) {
	primary := &mockProvider{failWrite: true, writeErr: errors.New("primary fail")}
	secondary := &mockProvider{}
	hybrid := NewHybridObjectStorage(primary, secondary)

	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{}
	obj := map[string]any{objects.FieldKeyKind: "task"}

	err := hybrid.Create(ctx, secCtx, obj)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "primary fail") {
		t.Errorf("expected error to contain 'primary fail', got '%v'", err)
	}
	if !primary.calledWrite {
		t.Error("expected primary.Create to be called")
	}
	if secondary.calledWrite {
		t.Error("expected secondary.Create NOT to be called")
	}
}

func TestHybridObjectStorage_Create_SecondaryFail(t *testing.T) {
	primary := &mockProvider{}
	secondary := &mockProvider{failWrite: true, writeErr: errors.New("secondary fail")}
	hybrid := NewHybridObjectStorage(primary, secondary)

	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{}
	obj := map[string]any{objects.FieldKeyKind: "task"}

	err := hybrid.Create(ctx, secCtx, obj)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to mirror Create to secondary storage") {
		t.Errorf("expected error message mismatch, got '%v'", err)
	}
	if !strings.Contains(err.Error(), "secondary fail") {
		t.Errorf("expected error to contain 'secondary fail', got '%v'", err)
	}
	if !primary.calledWrite {
		t.Error("expected primary.Create to be called")
	}
	if !secondary.calledWrite {
		t.Error("expected secondary.Create to be called")
	}
}

func TestHybridObjectStorage_Update(t *testing.T) {
	primary := &mockProvider{}
	secondary := &mockProvider{}
	hybrid := NewHybridObjectStorage(primary, secondary)

	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{}
	updates := map[string]any{objects.FieldKeyStatus: objects.ObjectStatusCompleted}

	err := hybrid.Update(ctx, secCtx, "test-id", updates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !primary.calledWrite {
		t.Error("expected primary.Update to be called")
	}
	if !secondary.calledWrite {
		t.Error("expected secondary.Update to be called")
	}
	if primary.lastID != "test-id" {
		t.Errorf("expected ID 'test-id' for primary, got '%s'", primary.lastID)
	}
	if secondary.lastID != "test-id" {
		t.Errorf("expected ID 'test-id' for secondary, got '%s'", secondary.lastID)
	}
}

func TestHybridObjectStorage_Delete(t *testing.T) {
	primary := &mockProvider{}
	secondary := &mockProvider{}
	hybrid := NewHybridObjectStorage(primary, secondary)

	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{}

	err := hybrid.Delete(ctx, secCtx, "test-id", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !primary.calledWrite {
		t.Error("expected primary.Delete to be called")
	}
	if !secondary.calledWrite {
		t.Error("expected secondary.Delete to be called")
	}
	if primary.lastID != "test-id" {
		t.Errorf("expected ID 'test-id' for primary, got '%s'", primary.lastID)
	}
	if secondary.lastID != "test-id" {
		t.Errorf("expected ID 'test-id' for secondary, got '%s'", secondary.lastID)
	}
}

func TestHybridObjectStorage_Delete_PrimaryNotFound(t *testing.T) {
	primary := &mockProvider{
		failWrite: true,
		writeErr:  ErrObjectNotFound,
	}
	secondary := &mockProvider{}
	hybrid := NewHybridObjectStorage(primary, secondary)

	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{}

	err := hybrid.Delete(ctx, secCtx, "test-id", false)
	if err != nil {
		t.Fatalf("unexpected error when primary is not found: %v", err)
	}
	if !primary.calledWrite {
		t.Error("expected primary.Delete to be called")
	}
	if !secondary.calledWrite {
		t.Error("expected secondary.Delete to be called even if primary is not found")
	}
}

func TestHybridObjectStorage_BulkCreate(t *testing.T) {
	primary := &mockProvider{bulkResult: &BulkResult{SuccessCount: 1}}
	secondary := &mockProvider{bulkResult: &BulkResult{SuccessCount: 1}}
	hybrid := NewHybridObjectStorage(primary, secondary)

	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{}
	objs := []map[string]any{{objects.FieldKeyKind: "task"}}

	result, err := hybrid.BulkCreate(ctx, secCtx, objs)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SuccessCount != 1 {
		t.Errorf("expected success count 1, got %d", result.SuccessCount)
	}
	if !primary.calledWrite {
		t.Error("expected primary.BulkCreate to be called")
	}
	if !secondary.calledWrite {
		t.Error("expected secondary.BulkCreate to be called")
	}
}

func TestHybridObjectStorage_Move(t *testing.T) {
	primary := &mockProvider{}
	secondary := &mockProvider{}
	hybrid := NewHybridObjectStorage(primary, secondary)

	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{}

	err := hybrid.Move(ctx, secCtx, "test-id", "new-kind", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !primary.calledWrite {
		t.Error("expected primary.Move to be called")
	}
	if !secondary.calledWrite {
		t.Error("expected secondary.Move to be called")
	}
}

func TestHybridObjectStorage_Rename(t *testing.T) {
	primary := &mockProvider{}
	secondary := &mockProvider{}
	hybrid := NewHybridObjectStorage(primary, secondary)

	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{}

	err := hybrid.Rename(ctx, secCtx, "old-id", "new-id", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !primary.calledWrite {
		t.Error("expected primary.Rename to be called")
	}
	if !secondary.calledWrite {
		t.Error("expected secondary.Rename to be called")
	}
}

type mockTransaction struct {
	ObjectTransaction
	calledCreate bool
	calledRead   bool
	calledUpdate bool
	calledDelete bool
	calledCommit bool
	failCreate   bool
	failRead     bool
	failUpdate   bool
	failDelete   bool
	failCommit   bool
	createErr    error
	readErr      error
	updateErr    error
	deleteErr    error
	lastID       string
	lastObj      map[string]any
	lastUpdates  map[string]any
	readObj      map[string]any
}

func (m *mockTransaction) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	m.calledCreate = true
	m.lastObj = obj
	if m.failCreate {
		return m.createErr
	}
	return nil
}

func (m *mockTransaction) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	m.calledRead = true
	m.lastID = id
	if m.failRead {
		return nil, m.readErr
	}
	if m.readObj != nil {
		return m.readObj, nil
	}
	return map[string]any{objects.FieldKeyID: id}, nil
}

func (m *mockTransaction) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	m.calledUpdate = true
	m.lastID = id
	m.lastUpdates = updates
	if m.failUpdate {
		return m.updateErr
	}
	return nil
}

func (m *mockTransaction) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	m.calledDelete = true
	m.lastID = id
	if m.failDelete {
		return m.deleteErr
	}
	return nil
}

func (m *mockTransaction) Commit(ctx context.Context) error {
	m.calledCommit = true
	if m.failCommit {
		return errors.New("commit fail")
	}
	return nil
}

func (m *mockTransaction) Rollback(ctx context.Context) error {
	return nil
}

func (m *mockProvider) BeginTransaction(ctx context.Context) (ObjectTransaction, error) {
	if m.tx != nil {
		return m.tx, nil
	}
	return &mockTransaction{}, nil
}

func TestHybridObjectStorage_Transaction(t *testing.T) {
	primary := &mockProvider{}
	secondary := &mockProvider{}
	hybrid := NewHybridObjectStorage(primary, secondary)

	ctx := context.Background()
	tx, err := hybrid.BeginTransaction(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	secCtx := &pkgctx.SecurityContext{}
	err = tx.Create(ctx, secCtx, map[string]any{objects.FieldKeyID: "tx-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	htx := tx.(*HybridObjectTransaction)
	ptx := htx.primaryTx.(*mockTransaction)
	stx := htx.secondaryTx.(*mockTransaction)

	if !ptx.calledCreate || !ptx.calledCommit {
		t.Error("primary transaction not called correctly")
	}
	if !stx.calledCreate || !stx.calledCommit {
		t.Error("secondary transaction not called correctly")
	}
}

func TestHybridObjectTransaction_Read(t *testing.T) {
	ptx := &mockTransaction{}
	stx := &mockTransaction{}

	primary := &mockProvider{tx: ptx}
	secondary := &mockProvider{tx: stx}
	hybrid := NewHybridObjectStorage(primary, secondary)

	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{}

	tx, err := hybrid.BeginTransaction(ctx)
	if err != nil {
		t.Fatalf("BeginTransaction failed: %v", err)
	}

	// 1. Success from primary
	ptx.readObj = map[string]any{objects.FieldKeyID: "obj-1", "val": "primary"}
	obj, err := tx.Read(ctx, secCtx, "obj-1")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if obj["val"] != "primary" {
		t.Errorf("expected primary val, got %v", obj["val"])
	}
	if !ptx.calledRead || stx.calledRead {
		t.Error("primary read should be called, secondary not")
	}

	// 2. ErrObjectNotFound fallback to secondary
	ptx.calledRead = false
	stx.calledRead = false
	ptx.failRead = true
	ptx.readErr = ErrObjectNotFound
	stx.readObj = map[string]any{objects.FieldKeyID: "obj-1", "val": "secondary"}

	obj, err = tx.Read(ctx, secCtx, "obj-1")
	if err != nil {
		t.Fatalf("Read fallback failed: %v", err)
	}
	if obj["val"] != "secondary" {
		t.Errorf("expected secondary val, got %v", obj["val"])
	}
	if !ptx.calledRead || !stx.calledRead {
		t.Error("both primary and secondary reads should be called on fallback")
	}

	// 3. Other error from primary does not fall back
	ptx.calledRead = false
	stx.calledRead = false
	ptx.readErr = errors.New("severe database error")

	_, err = tx.Read(ctx, secCtx, "obj-1")
	if err == nil {
		t.Fatal("expected severe read error to propagate, got nil")
	}
	if !strings.Contains(err.Error(), "severe database error") {
		t.Errorf("unexpected error: %v", err)
	}
	if !ptx.calledRead || stx.calledRead {
		t.Error("only primary read should have been attempted")
	}
}

func TestHybridObjectTransaction_Update(t *testing.T) {
	ptx := &mockTransaction{}
	stx := &mockTransaction{}

	primary := &mockProvider{tx: ptx}
	secondary := &mockProvider{tx: stx}
	hybrid := NewHybridObjectStorage(primary, secondary)

	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{}

	tx, err := hybrid.BeginTransaction(ctx)
	if err != nil {
		t.Fatalf("BeginTransaction failed: %v", err)
	}

	// 1. Success on normal update (exists in primary)
	ptx.readObj = map[string]any{objects.FieldKeyID: "obj-1"}
	err = tx.Update(ctx, secCtx, "obj-1", map[string]any{objects.FieldKeyStatus: objects.ObjectStatusInProgress})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if !ptx.calledRead || !ptx.calledUpdate || !stx.calledUpdate {
		t.Error("expected normal update to read and update primary, and update secondary")
	}

	// 2. Lazy migration (not found in primary, exists in secondary)
	ptx.calledRead = false
	ptx.calledCreate = false
	ptx.calledUpdate = false
	stx.calledRead = false
	stx.calledUpdate = false

	ptx.failRead = true
	ptx.readErr = ErrObjectNotFound
	stx.readObj = map[string]any{objects.FieldKeyID: "obj-2", objects.FieldKeyTitle: "Old Title"}

	err = tx.Update(ctx, secCtx, "obj-2", map[string]any{objects.FieldKeyTitle: "New Title"})
	if err != nil {
		t.Fatalf("Lazy migration update failed: %v", err)
	}

	if !ptx.calledCreate || !stx.calledUpdate {
		t.Error("expected primary.Create and secondary.Update for lazy migration")
	}
	if ptx.lastObj[objects.FieldKeyTitle] != "New Title" {
		t.Errorf("expected merged updates, got %v", ptx.lastObj)
	}
}
