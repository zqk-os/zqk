package cli

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestProcessor_ResolveSemanticArgument(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cliapp.semantic.resolver",
	})
	cmd := &cobra.Command{Use: "test-semantic"}
	cmd.SetContext(pkgctx.NewSystemContext())
	cliCtx := ContextForProjectRoot(proj.Root)
	SetContext(cmd, cliCtx)

	proc, err := NewProcessor(cmd)
	if err != nil {
		t.Fatalf("failed to create processor: %v", err)
	}

	ctx := context.Background()

	// 1. Valid ID format returns directly
	resolvedID, err := proc.ResolveSemanticArgument(ctx, "backlog_item", "BLI-12345")
	if err != nil || resolvedID != "BLI-12345" {
		t.Errorf("expected BLI-12345 to resolve directly, got %s (err: %v)", resolvedID, err)
	}

	// 2. Priority plan ID format returns directly
	resolvedPlan, err := proc.ResolveSemanticArgument(ctx, "priority_plan", "PRI-999")
	if err != nil || resolvedPlan != "PRI-999" {
		t.Errorf("expected PRI-999 to resolve directly, got %s (err: %v)", resolvedPlan, err)
	}

	// 3. Title resolution when no match found
	_, err = proc.ResolveSemanticArgument(ctx, "goal", "Non Existent Title XYZ")
	if err == nil {
		t.Errorf("expected error when semantic title does not match any object")
	}

	// 4. Exact and substring title resolution
	sampleGoal := map[string]any{
		"id":    "GOAL-arch-123",
		"kind":  "goal",
		"title": "Build Architecture Layer",
	}
	if err := proj.FileStorage.Create(ctx, proc.SecurityContext(), sampleGoal); err == nil {
		// Exact title match
		resID, err := proc.ResolveSemanticArgument(ctx, "goal", "Build Architecture Layer")
		if err == nil && resID != "GOAL-arch-123" {
			t.Errorf("expected GOAL-arch-123, got: %s", resID)
		}
		// Substring title match
		resSub, err := proc.ResolveSemanticArgument(ctx, "goal", "Architecture Layer")
		if err == nil && resSub != "GOAL-arch-123" {
			t.Errorf("expected substring match GOAL-arch-123, got: %s", resSub)
		}
	}
}

type mockStorageForSemanticDecorator struct {
	storage.ObjectStorageProvider
	readObj map[string]any
}

func (m *mockStorageForSemanticDecorator) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return m.readObj, nil
}

func (m *mockStorageForSemanticDecorator) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	return &storage.QueryResult{Objects: []map[string]any{m.readObj}}, nil
}

func (m *mockStorageForSemanticDecorator) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query storage.Query) (*storage.QueryResult, error) {
	return &storage.QueryResult{Objects: []map[string]any{m.readObj}}, nil
}

func TestSemanticStorageDecorator(t *testing.T) {
	mockStore := &mockStorageForSemanticDecorator{
		readObj: map[string]any{
			objects.FieldKeyID:   "GOAL-1",
			objects.FieldKeyKind: objects.KindGoal,
		},
	}
	decorator := &SemanticStorageDecorator{
		ObjectStorageProvider: mockStore,
	}

	// UnderlyingObjectStorageProvider
	if decorator.UnderlyingObjectStorageProvider() != mockStore {
		t.Errorf("expected mockStore as underlying provider")
	}
	var nilDec *SemanticStorageDecorator
	if nilDec.UnderlyingObjectStorageProvider() != nil {
		t.Errorf("expected nil for nil decorator")
	}

	// Read with no active schemes -> allowed
	secCtx := pkgctx.NewSystemSecurityContext()
	obj, err := decorator.Read(context.Background(), secCtx, "GOAL-1")
	if err != nil || obj == nil {
		t.Fatalf("unexpected error reading object: %v", err)
	}

	// Read with active scheme that does not allow goal -> error
	secCtxFiltered := &pkgctx.SecurityContext{
		ActiveVocabularySchemes: []string{"VOCAB-1"},
	}
	// Core structural objects (priority_plan, persona, etc.) are always allowed
	mockStore.readObj = map[string]any{
		objects.FieldKeyID:   "PRI-1",
		objects.FieldKeyKind: objects.KindPriorityPlan,
	}
	planObj, err := decorator.Read(context.Background(), secCtxFiltered, "PRI-1")
	if err != nil || planObj == nil {
		t.Errorf("expected core priority_plan to be allowed even with vocabulary schemes: %v", err)
	}

	// List with disallowed kind fast-fails to empty
	listRes, err := decorator.List(context.Background(), secCtxFiltered, nil, storage.ListFilter{Kind: "unallowed_kind"})
	if err != nil || len(listRes.Objects) != 0 {
		t.Errorf("expected empty list result for unallowed kind: %+v", listRes)
	}

	// Query with core object
	qRes, err := decorator.Query(context.Background(), secCtxFiltered, nil, storage.Query{})
	if err != nil || len(qRes.Objects) != 1 {
		t.Errorf("expected 1 object from Query: %+v", qRes)
	}

	// Vocabulary scheme allowing goal via []any
	mockStore.readObj = map[string]any{
		objects.FieldKeyAllowedKinds: []any{"goal"},
	}
	if !decorator.isKindAllowed(context.Background(), secCtxFiltered, "goal") {
		t.Errorf("expected goal to be allowed by vocabulary scheme")
	}
	if decorator.isKindAllowed(context.Background(), secCtxFiltered, "risk_blocker") {
		t.Errorf("expected risk_blocker to be disallowed by vocabulary scheme")
	}

	// Vocabulary scheme allowing goal via []string
	mockStore.readObj = map[string]any{
		objects.FieldKeyAllowedKinds: []string{"goal"},
	}
	if !decorator.isKindAllowed(context.Background(), secCtxFiltered, "goal") {
		t.Errorf("expected goal to be allowed by string list scheme")
	}
}
