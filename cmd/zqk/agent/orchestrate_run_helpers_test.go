package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/objects/koi"
	"github.com/zqk-os/zqk/pkg/storage"
)

type mockHelperStore struct {
	storage.ObjectStorageProvider
	objs map[string]map[string]any
}

func newMockHelperStore() *mockHelperStore {
	return &mockHelperStore{objs: make(map[string]map[string]any)}
}

func (m *mockHelperStore) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	id := koi.ID(obj)
	m.objs[id] = obj
	return nil
}

func (m *mockHelperStore) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.objs[id]; ok {
		return obj, nil
	}
	return nil, errfmt.Errorf("not found: %s", id)
}

func (m *mockHelperStore) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, obj map[string]any) error {
	m.objs[id] = obj
	return nil
}

func (m *mockHelperStore) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	var res []map[string]any
	for _, obj := range m.objs {
		if filter.Kind != "" && koi.Kind(obj) != filter.Kind {
			continue
		}
		res = append(res, obj)
	}
	return &storage.QueryResult{Objects: res}, nil
}

func TestIsTestCaseReadyStatus(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"active", true},
		{"ACTIVE", true},
		{"draft", true},
		{"metrics_captured", true},
		{"complete", true},
		{"in_progress", false},
		{"originated", false},
		{"unknown", false},
	}
	for _, tc := range cases {
		if got := isTestCaseReadyStatus(tc.status); got != tc.want {
			t.Errorf("isTestCaseReadyStatus(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestExtractItemUpstreamIDs(t *testing.T) {
	if ids := extractItemUpstreamIDs(nil); ids != nil {
		t.Errorf("expected nil for nil item, got %v", ids)
	}

	item := map[string]any{
		"depends_on":         []any{"ATK-1", "ATK-2"},
		"upstream_task_refs": []any{"ATK-3"},
	}
	ids := extractItemUpstreamIDs(item)
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}
	if ids[0] != "ATK-1" || ids[1] != "ATK-2" || ids[2] != "ATK-3" {
		t.Errorf("unexpected IDs order: %v", ids)
	}
}

func TestResolveVerifiedUpstreamDeliverables(t *testing.T) {
	sp := newMockHelperStore()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Upstream task complete with artifacts and summary
	upTask := map[string]any{
		objects.FieldKeyID:        "ATK-UPSTREAM-1",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyTitle:     "Build Parser",
		objects.FieldKeyArtifacts: []any{"pkg/parser/ast.go"},
		"summary":                 "Parser constructed cleanly",
	}
	if err := sp.Create(ctx, secCtx, upTask); err != nil {
		t.Fatalf("failed to create upstream task: %v", err)
	}

	downstreamItem := map[string]any{
		objects.FieldKeyID:   "ATK-DOWNSTREAM-1",
		"upstream_task_refs": []any{"ATK-UPSTREAM-1"},
	}

	out := resolveVerifiedUpstreamDeliverables(ctx, sp, secCtx, downstreamItem)
	if !strings.Contains(out, "Upstream Verified Deliverables") {
		t.Errorf("expected header, got: %s", out)
	}
	if !strings.Contains(out, "pkg/parser/ast.go") {
		t.Errorf("expected artifact, got: %s", out)
	}
	if !strings.Contains(out, "Parser constructed cleanly") {
		t.Errorf("expected summary, got: %s", out)
	}
}

func TestIsBlockedByUnverifiedUpstream(t *testing.T) {
	sp := newMockHelperStore()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Missing upstream item blocks
	blockedItem := map[string]any{
		"upstream_task_refs": []any{"ATK-NONEXISTENT"},
	}
	if !isBlockedByUnverifiedUpstream(ctx, sp, secCtx, blockedItem) {
		t.Errorf("expected blocked by missing upstream")
	}

	// Incomplete upstream blocks
	upIncomplete := map[string]any{
		objects.FieldKeyID:        "ATK-INCOMPLETE",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusInProgress,
		objects.FieldKeyArtifacts: []any{"pkg/lib/lib.go"},
	}
	if err := sp.Create(ctx, secCtx, upIncomplete); err != nil {
		t.Fatalf("failed to create incomplete task: %v", err)
	}

	item2 := map[string]any{
		"upstream_task_refs": []any{"ATK-INCOMPLETE"},
	}
	if !isBlockedByUnverifiedUpstream(ctx, sp, secCtx, item2) {
		t.Errorf("expected blocked by in-progress upstream")
	}

	// Complete upstream with empty artifacts blocks
	upNoArtifacts := map[string]any{
		objects.FieldKeyID:        "ATK-NO-ARTS",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyArtifacts: []any{},
	}
	if err := sp.Create(ctx, secCtx, upNoArtifacts); err != nil {
		t.Fatalf("failed to create no artifacts task: %v", err)
	}

	item3 := map[string]any{
		"upstream_task_refs": []any{"ATK-NO-ARTS"},
	}
	if !isBlockedByUnverifiedUpstream(ctx, sp, secCtx, item3) {
		t.Errorf("expected blocked by empty artifacts upstream")
	}

	// Complete upstream with artifacts does not block
	upGood := map[string]any{
		objects.FieldKeyID:        "ATK-GOOD",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyArtifacts: []any{"pkg/lib/lib.go"},
	}
	if err := sp.Create(ctx, secCtx, upGood); err != nil {
		t.Fatalf("failed to create good task: %v", err)
	}

	itemGood := map[string]any{
		"upstream_task_refs": []any{"ATK-GOOD"},
	}
	if isBlockedByUnverifiedUpstream(ctx, sp, secCtx, itemGood) {
		t.Errorf("expected unblocked for good upstream")
	}
}

func TestVerifyBLITDDReady_And_LocalSkill(t *testing.T) {
	sp := newMockHelperStore()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	skill := map[string]any{
		objects.FieldKeyID:    "ASK-GO-CRAFT",
		objects.FieldKeyKind:  objects.KindAgentSkill,
		objects.FieldKeyTitle: "Go Code Craftsmanship",
	}
	if err := sp.Create(ctx, secCtx, skill); err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}

	if id := localSkillIDByTitle(ctx, sp, secCtx, "go code craftsmanship"); id != "ASK-GO-CRAFT" {
		t.Errorf("expected ASK-GO-CRAFT, got: %s", id)
	}
	if id := localSkillIDByTitle(ctx, sp, secCtx, "nonexistent"); id != "" {
		t.Errorf("expected empty string for nonexistent, got: %s", id)
	}

	tc := map[string]any{
		objects.FieldKeyID:     "TC-1",
		objects.FieldKeyKind:   objects.KindTestCase,
		objects.FieldKeyStatus: "complete",
	}
	if err := sp.Create(ctx, secCtx, tc); err != nil {
		t.Fatalf("failed to create test case: %v", err)
	}

	bli := map[string]any{
		objects.FieldKeyTestCaseRefs: []any{"TC-1"},
	}
	if !verifyBLITDDReady(ctx, sp, secCtx, bli) {
		t.Errorf("expected bli with direct complete test case to be TDD ready")
	}
}

func TestRunOrchestrate_ValidationBranches(t *testing.T) {
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()
	t.Setenv("ZQK_PROJECT_ROOT", tempDir)

	// 1. Deny doer / guest
	cmd := NewOrchestrateCmd()
	cmd.SetContext(pkgctx.WithSecurityContext(context.Background(), pkgctx.NewGuestSecurityContext()))
	err := runOrchestrate(cmd, "PRI-1", OrchestrateOptions{})
	assert.ErrorContains(t, err, "cannot orchestrate")

	// 2. Semantic routing failure
	adminCtx := pkgctx.WithSecurityContext(context.Background(), pkgctx.NewSystemSecurityContext())
	cmdAdmin := NewOrchestrateCmd()
	cmdAdmin.SetContext(adminCtx)
	err = runOrchestrate(cmdAdmin, "NONEXISTENT-PLAN-XYZ", OrchestrateOptions{})
	assert.ErrorContains(t, err, "semantic routing failed")

	// 3. No active plan via whats-next
	cmdEmpty := NewOrchestrateCmd()
	cmdEmpty.SetContext(adminCtx)
	err = runOrchestrate(cmdEmpty, "", OrchestrateOptions{})
	assert.ErrorContains(t, err, "no active priority plan")
}
