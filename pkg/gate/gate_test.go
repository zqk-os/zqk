package gate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// mockStorageProvider is a lightweight in-memory ObjectStorageProvider for testing.
type mockStorageProvider struct {
	mu      sync.RWMutex
	objects map[string]map[string]any
}

func newMockStorage() *mockStorageProvider {
	return &mockStorageProvider{
		objects: make(map[string]map[string]any),
	}
}

func (m *mockStorageProvider) Create(_ context.Context, _ *pkgctx.SecurityContext, obj map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, _ := obj[objects.FieldKeyID].(string)
	if id == "" {
		id = GenerateApprovalID()
		obj[objects.FieldKeyID] = id
	}
	if _, exists := m.objects[id]; exists {
		return fmt.Errorf("object %s already exists", id)
	}
	cp := make(map[string]any)
	for k, v := range obj {
		cp[k] = v
	}
	m.objects[id] = cp
	return nil
}

func (m *mockStorageProvider) Read(_ context.Context, _ *pkgctx.SecurityContext, id string) (map[string]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	obj, exists := m.objects[id]
	if !exists {
		return nil, fmt.Errorf("object %s not found", id)
	}
	cp := make(map[string]any)
	for k, v := range obj {
		cp[k] = v
	}
	return cp, nil
}

func (m *mockStorageProvider) Update(_ context.Context, _ *pkgctx.SecurityContext, id string, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	obj, exists := m.objects[id]
	if !exists {
		return fmt.Errorf("object %s not found", id)
	}
	for k, v := range updates {
		obj[k] = v
	}
	return nil
}

func (m *mockStorageProvider) Delete(_ context.Context, _ *pkgctx.SecurityContext, id string, _ bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, id)
	return nil
}

func (m *mockStorageProvider) List(_ context.Context, _ *pkgctx.SecurityContext, _ *storage.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []map[string]any
	for _, obj := range m.objects {
		if filter.Kind != "" && obj[objects.FieldKeyKind] != filter.Kind {
			continue
		}
		match := true
		for fk, fv := range filter.Filters {
			if obj[fk] != fv {
				match = false
				break
			}
		}
		if match {
			cp := make(map[string]any)
			for k, v := range obj {
				cp[k] = v
			}
			list = append(list, cp)
		}
	}
	return &storage.QueryResult{Objects: list}, nil
}

func (m *mockStorageProvider) Query(_ context.Context, _ *pkgctx.SecurityContext, _ *storage.StorageContext, _ storage.Query) (*storage.QueryResult, error) {
	return nil, nil
}
func (m *mockStorageProvider) Search(_ context.Context, _ *pkgctx.SecurityContext, _ *storage.StorageContext, _ storage.SearchQuery) (*storage.SearchResult, error) {
	return nil, nil
}
func (m *mockStorageProvider) BeginTransaction(_ context.Context) (storage.ObjectTransaction, error) {
	return nil, nil
}
func (m *mockStorageProvider) BulkCreate(_ context.Context, _ *pkgctx.SecurityContext, _ []map[string]any) (*storage.BulkResult, error) {
	return nil, nil
}
func (m *mockStorageProvider) BulkUpdate(_ context.Context, _ *pkgctx.SecurityContext, _ []storage.BulkUpdateItem) (*storage.BulkResult, error) {
	return nil, nil
}
func (m *mockStorageProvider) BulkGet(_ context.Context, _ *pkgctx.SecurityContext, _ []string) (*storage.BulkResult, error) {
	return nil, nil
}
func (m *mockStorageProvider) BulkDelete(_ context.Context, _ *pkgctx.SecurityContext, _ []string, _ bool) (*storage.BulkResult, error) {
	return nil, nil
}
func (m *mockStorageProvider) Exists(_ context.Context, _ *pkgctx.SecurityContext, id string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.objects[id]
	return exists, nil
}
func (m *mockStorageProvider) Count(_ context.Context, _ *pkgctx.SecurityContext, _ storage.ListFilter) (int, error) {
	return 0, nil
}
func (m *mockStorageProvider) Aggregate(_ context.Context, _ *pkgctx.SecurityContext, _ *storage.StorageContext, _ storage.ListFilter, _ []storage.Aggregation) (*storage.AggregateResult, error) {
	return nil, nil
}
func (m *mockStorageProvider) GetRelated(_ context.Context, _ *pkgctx.SecurityContext, _ string, _ string, _ int) ([]map[string]any, error) {
	return nil, nil
}
func (m *mockStorageProvider) GetPath(_ context.Context, _ *pkgctx.SecurityContext, _, _ string) ([]map[string]any, error) {
	return nil, nil
}
func (m *mockStorageProvider) GetNeighbors(_ context.Context, _ *pkgctx.SecurityContext, _ string, _ string) ([]map[string]any, error) {
	return nil, nil
}
func (m *mockStorageProvider) Move(_ context.Context, _ *pkgctx.SecurityContext, _, _ string, _ bool) error {
	return nil
}
func (m *mockStorageProvider) Rename(_ context.Context, _ *pkgctx.SecurityContext, _, _ string, _ bool) error {
	return nil
}
func (m *mockStorageProvider) Shutdown(_ context.Context) error {
	return nil
}

func TestStageApproval_Success(t *testing.T) {
	ctx := context.Background()
	store := newMockStorage()

	payload := []byte("mutation payload: update policy POL-123")
	expectedDigest := hex.EncodeToString(func() []byte {
		h := sha256.Sum256(payload)
		return h[:]
	}())

	req, err := StageApproval(ctx, store, "mutation", "agent-system", payload, "operator-alice")
	require.NoError(t, err)
	require.NotNil(t, req)

	assert.True(t, strings.HasPrefix(req.ID, "APR-"))
	assert.Equal(t, "mutation", req.Type)
	assert.Equal(t, "agent-system", req.Target)
	assert.Equal(t, string(payload), req.Payload)
	assert.Equal(t, expectedDigest, req.PayloadHash)
	assert.Equal(t, expectedDigest, req.Hash)
	assert.Equal(t, StatusStaged, req.Status)
	assert.Equal(t, "operator-alice", req.CreatedBy)

	// Verify persistence
	loaded, err := GetApproval(ctx, store, req.ID)
	require.NoError(t, err)
	assert.Equal(t, req.ID, loaded.ID)
	assert.Equal(t, expectedDigest, loaded.PayloadHash)
}

func TestStageApproval_ValidationFailures(t *testing.T) {
	ctx := context.Background()
	store := newMockStorage()

	_, err := StageApproval(ctx, nil, "mutation", "target", []byte("payload"), "user")
	assert.Error(t, err)

	_, err = StageApproval(ctx, store, "", "target", []byte("payload"), "user")
	assert.Error(t, err)

	_, err = StageApproval(ctx, store, "mutation", "", []byte("payload"), "user")
	assert.Error(t, err)

	_, err = StageApproval(ctx, store, "mutation", "target", nil, "user")
	assert.Error(t, err)
}

func TestApprove_SuccessAndTamperDetection(t *testing.T) {
	ctx := context.Background()
	store := newMockStorage()

	payload := []byte("execute migration step 1")
	req, err := StageApproval(ctx, store, "db_migration", "db-leader", payload, "operator-bob")
	require.NoError(t, err)

	// Approve successfully
	approved, err := Approve(ctx, store, "", req.ID, "reviewer-charlie")
	require.NoError(t, err)
	assert.Equal(t, StatusApproved, approved.Status)
	assert.Equal(t, "reviewer-charlie", approved.Reviewer)
	assert.NotEmpty(t, approved.ReviewedAt)

	// Verify fail-closed verification passes
	verified, err := VerifyApproval(ctx, store, req.ID, payload)
	require.NoError(t, err)
	assert.True(t, verified)

	// Verify tamper detection on approve
	req2, err := StageApproval(ctx, store, "mutation", "target", []byte("original payload"), "user")
	require.NoError(t, err)

	// Tamper payload in storage
	secCtx := pkgctx.NewSystemSecurityContext()
	err = store.Update(ctx, secCtx, req2.ID, map[string]any{"payload": "tampered payload"})
	require.NoError(t, err)

	// Approve must detect tampering and fail closed
	_, err = Approve(ctx, store, "", req2.ID, "reviewer")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tamper detected")
}

func TestReject_Success(t *testing.T) {
	ctx := context.Background()
	store := newMockStorage()

	req, err := StageApproval(ctx, store, "risky_mutation", "pod-worker", []byte("dangerous operation"), "agent-1")
	require.NoError(t, err)

	rejected, err := Reject(ctx, store, "", req.ID, "operator-alice", "High security risk detected")
	require.NoError(t, err)
	assert.Equal(t, StatusRejected, rejected.Status)
	assert.Equal(t, "operator-alice", rejected.Reviewer)
	assert.Equal(t, "High security risk detected", rejected.Reason)

	// Cannot approve rejected request
	_, err = Approve(ctx, store, "", req.ID, "reviewer-bob")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot approve rejected request")

	// Verification must fail closed
	verified, err := VerifyApproval(ctx, store, req.ID, []byte("dangerous operation"))
	assert.Error(t, err)
	assert.False(t, verified)
}

func TestVerifyApproval_FailClosedScenarios(t *testing.T) {
	ctx := context.Background()
	store := newMockStorage()

	payload := []byte("critical action")
	req, err := StageApproval(ctx, store, "action", "dest", payload, "author")
	require.NoError(t, err)

	// 1. Nil storage
	ok, err := VerifyApproval(ctx, nil, req.ID, payload)
	assert.False(t, ok)
	assert.Error(t, err)

	// 2. Empty ID
	ok, err = VerifyApproval(ctx, store, "", payload)
	assert.False(t, ok)
	assert.Error(t, err)

	// 3. Empty payload
	ok, err = VerifyApproval(ctx, store, req.ID, nil)
	assert.False(t, ok)
	assert.Error(t, err)

	// 4. Staged (not approved yet)
	ok, err = VerifyApproval(ctx, store, req.ID, payload)
	assert.False(t, ok)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "has status \"staged\"")

	// 5. Payload hash mismatch
	_, err = Approve(ctx, store, "", req.ID, "reviewer")
	require.NoError(t, err)

	ok, err = VerifyApproval(ctx, store, req.ID, []byte("different payload"))
	assert.False(t, ok)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cryptographic digest mismatch")

	// 6. Non-existent approval ID
	ok, err = VerifyApproval(ctx, store, "APR-NONEXISTENT", payload)
	assert.False(t, ok)
	assert.Error(t, err)
}

func TestListApprovals(t *testing.T) {
	ctx := context.Background()
	store := newMockStorage()

	req1, _ := StageApproval(ctx, store, "type1", "target1", []byte("p1"), "user1")
	req2, _ := StageApproval(ctx, store, "type2", "target2", []byte("p2"), "user2")
	_, _ = Approve(ctx, store, "", req2.ID, "reviewer")

	all, err := ListApprovals(ctx, store, "")
	require.NoError(t, err)
	assert.Len(t, all, 2)

	staged, err := ListApprovals(ctx, store, StatusStaged)
	require.NoError(t, err)
	assert.Len(t, staged, 1)
	assert.Equal(t, req1.ID, staged[0].ID)

	approved, err := ListApprovals(ctx, store, StatusApproved)
	require.NoError(t, err)
	assert.Len(t, approved, 1)
	assert.Equal(t, req2.ID, approved[0].ID)
}
