package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestStorageExtended_Wave1_DocumentationPolicy tests all branches in documentation_policy_validator.go
func TestStorageExtended_Wave1_DocumentationPolicy(t *testing.T) {
	tmpDir := t.TempDir()

	// Set up mock tree with various doc files and READMEs
	docsDir := filepath.Join(tmpDir, "docs")
	pkgDir := filepath.Join(tmpDir, "pkg", "sample")
	nodeModulesDir := filepath.Join(tmpDir, "node_modules")
	unallowedDir := filepath.Join(tmpDir, "random_folder")

	for _, d := range []string{docsDir, pkgDir, nodeModulesDir, unallowedDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("MkdirAll failed: %v", err)
		}
	}

	// 1. Allowed doc in docs/
	_ = fileutil.WriteFile(filepath.Join(docsDir, "guide.md"), []byte("# Guide\nSome doc content"), 0644)

	// 2. Excluded file in node_modules
	_ = fileutil.WriteFile(filepath.Join(nodeModulesDir, "ignore.md"), []byte("# Ignore"), 0644)

	// 3. Unallowed doc not in docs/ and not in code dir
	_ = fileutil.WriteFile(filepath.Join(unallowedDir, "stray.md"), []byte("# Stray"), 0644)

	// 4. Non-markdown file in pkg
	_ = fileutil.WriteFile(filepath.Join(pkgDir, "code.go"), []byte("package sample"), 0644)

	// 5. Unallowed markdown in code dir (not README.md)
	_ = fileutil.WriteFile(filepath.Join(pkgDir, "architecture.md"), []byte("# Arch"), 0644)

	// 6. README in code dir that is brief (<=200 lines, no detailed indicators)
	_ = fileutil.WriteFile(filepath.Join(pkgDir, "README.md"), []byte("# Package Sample\nIndex summary"), 0644)

	// 7. ValidateDocumentationPolicy on projectRoot
	violations, err := storagepkg.ValidateDocumentationPolicy(tmpDir)
	if err != nil {
		t.Fatalf("ValidateDocumentationPolicy failed: %v", err)
	}
	if len(violations) < 2 {
		t.Errorf("Expected at least 2 violations, got %d", len(violations))
	}

	// 8. Test checkReadmeIsIndexOnly with detailed content but NO doc links
	pkgDir2 := filepath.Join(tmpDir, "pkg", "detailed")
	_ = os.MkdirAll(pkgDir2, 0755)
	detailedReadmePath := filepath.Join(pkgDir2, "README.md")
	_ = fileutil.WriteFile(detailedReadmePath, []byte("# Detailed\n## Architecture\nSome detailed arch"), 0644)

	viols2, _ := storagepkg.ValidateDocumentationPolicyForFile(tmpDir, detailedReadmePath)
	if len(viols2) == 0 {
		t.Errorf("Expected violation for detailed README without doc links")
	}

	// 9. Test checkReadmeIsIndexOnly with detailed content AND doc links
	detailedWithLinkPath := filepath.Join(pkgDir2, "readme.md")
	_ = fileutil.WriteFile(detailedWithLinkPath, []byte("# Detailed\n## Architecture\nSee docs/architecture/arch.md and docs/onboarding/guide.md"), 0644)
	viols3, _ := storagepkg.ValidateDocumentationPolicyForFile(tmpDir, detailedWithLinkPath)
	if len(viols3) != 0 {
		t.Errorf("Expected no violation when doc link is present, got %v", viols3)
	}

	// 10. Test checkReadmeIsIndexOnly with > 200 lines
	longContent := "# Long Readme\n"
	for i := 0; i < 205; i++ {
		longContent += "line\n"
	}
	longReadmePath := filepath.Join(pkgDir2, "README.md")
	_ = fileutil.WriteFile(longReadmePath, []byte(longContent), 0644)
	viols4, _ := storagepkg.ValidateDocumentationPolicyForFile(tmpDir, longReadmePath)
	if len(viols4) == 0 {
		t.Errorf("Expected violation for README with > 200 lines")
	}

	// 11. ValidateDocumentationPolicyForFile edge cases
	// Non-markdown
	viols5, _ := storagepkg.ValidateDocumentationPolicyForFile(tmpDir, filepath.Join(pkgDir, "code.go"))
	if len(viols5) != 0 {
		t.Errorf("Expected 0 violations for non-md file")
	}
	// Excluded path (e.g. vendor/testdata)
	vendorDir := filepath.Join(tmpDir, "vendor")
	_ = os.MkdirAll(vendorDir, 0755)
	vendorFile := filepath.Join(vendorDir, "readme.md")
	_ = fileutil.WriteFile(vendorFile, []byte("vendor"), 0644)
	viols6, _ := storagepkg.ValidateDocumentationPolicyForFile(tmpDir, vendorFile)
	if len(viols6) != 0 {
		t.Errorf("Expected 0 violations for excluded vendor dir")
	}
	// Not in allowed and not in code dir
	viols7, _ := storagepkg.ValidateDocumentationPolicyForFile(tmpDir, filepath.Join(unallowedDir, "stray.md"))
	if len(viols7) == 0 {
		t.Errorf("Expected violation for stray markdown outside docs")
	}

	// 12. FindUnregisteredDocumentation
	archDir := filepath.Join(tmpDir, "docs", "architecture")
	_ = os.MkdirAll(archDir, 0755)
	registeredDoc := filepath.Join(archDir, "registered.md")
	unregisteredDoc := filepath.Join(archDir, "unregistered.md")
	_ = fileutil.WriteFile(registeredDoc, []byte("# Reg"), 0644)
	_ = fileutil.WriteFile(unregisteredDoc, []byte("# Unreg"), 0644)

	relReg, _ := filepath.Rel(tmpDir, registeredDoc)
	existingMap := map[string]bool{
		relReg: true,
	}
	unregistered, err := storagepkg.FindUnregisteredDocumentation(tmpDir, existingMap)
	if err != nil {
		t.Fatalf("FindUnregisteredDocumentation failed: %v", err)
	}
	foundUnreg := false
	for _, u := range unregistered {
		if filepath.Base(u) == "unregistered.md" {
			foundUnreg = true
		}
	}
	if !foundUnreg {
		t.Errorf("Expected unregistered.md to be found in unregistered list: %v", unregistered)
	}
}

type mockStorageProvider struct {
	storagepkg.ObjectStorageProvider
	data map[string]map[string]any
}

func (m *mockStorageProvider) Read(_ context.Context, _ *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.data[id]; ok {
		return obj, nil
	}
	return nil, os.ErrNotExist
}

// TestStorageExtended_Wave1_WorkflowConstraintValidator tests workflow_constraint_validator.go
func TestStorageExtended_Wave1_WorkflowConstraintValidator(t *testing.T) {
	mock := &mockStorageProvider{
		data: make(map[string]map[string]any),
	}
	ctx := context.Background()

	secCtx := &pkgctx.SecurityContext{
		AccountID:   "test-user",
		Roles:       []string{"developer", "contributor"},
		Permissions: []string{"read:*", "write:*"},
	}

	wcv := storagepkg.NewWorkflowConstraintValidator(mock)

	// Case 1: Priority plan does not exist
	err := wcv.ValidatePriorityPlanActivation(ctx, secCtx, "plan-nonexistent")
	if err == nil {
		t.Errorf("expected error reading nonexistent priority plan")
	}

	// Case 2: Priority plan with empty workflow_ref (should succeed)
	mock.data["plan-1"] = map[string]any{
		objects.FieldKeyID:   "plan-1",
		objects.FieldKeyKind: objects.KindPriorityPlan,
	}
	if err := wcv.ValidatePriorityPlanActivation(ctx, secCtx, "plan-1"); err != nil {
		t.Errorf("expected success when workflow_ref is empty: %v", err)
	}

	// Case 3: Priority plan referencing missing workflow
	mock.data["plan-2"] = map[string]any{
		objects.FieldKeyID:          "plan-2",
		objects.FieldKeyKind:        objects.KindPriorityPlan,
		objects.FieldKeyWorkflowRef: "wf-missing",
	}
	if err := wcv.ValidatePriorityPlanActivation(ctx, secCtx, "plan-2"); err == nil {
		t.Errorf("expected error reading missing workflow")
	}

	// Case 4: Workflow disabled
	mock.data["wf-disabled"] = map[string]any{
		objects.FieldKeyID:      "wf-disabled",
		objects.FieldKeyKind:    "workflow",
		objects.FieldKeyEnabled: false,
	}
	mock.data["plan-3"] = map[string]any{
		objects.FieldKeyID:          "plan-3",
		objects.FieldKeyKind:        objects.KindPriorityPlan,
		objects.FieldKeyWorkflowRef: "wf-disabled",
	}
	if err := wcv.ValidatePriorityPlanActivation(ctx, secCtx, "plan-3"); err == nil {
		t.Errorf("expected error for disabled workflow")
	}

	// Case 5: Workflow enabled with constraints
	mock.data["wf-valid"] = map[string]any{
		objects.FieldKeyID:      "wf-valid",
		objects.FieldKeyKind:    "workflow",
		objects.FieldKeyEnabled: true,
		objects.FieldKeyConstraints: map[string]any{
			"object_constraints": map[string]any{
				objects.KindPriorityPlan: map[string]any{
					"required_roles": []any{"lead", "developer"},
					"blocked_roles":  []any{"guest"},
				},
				"restricted_kind": map[string]any{
					"blocked_roles": []any{"developer"},
				},
			},
			"role_constraints": map[string]any{
				"developer": map[string]any{
					"allowed_operations": []any{"activate", "create"},
					"allowed_kinds":      []any{objects.KindPriorityPlan, "workstream"},
					"blocked_kinds":      []any{"secret"},
				},
			},
		},
	}
	mock.data["plan-4"] = map[string]any{
		objects.FieldKeyID:          "plan-4",
		objects.FieldKeyKind:        objects.KindPriorityPlan,
		objects.FieldKeyWorkflowRef: "wf-valid",
	}

	// Success case
	if err := wcv.ValidatePriorityPlanActivation(ctx, secCtx, "plan-4"); err != nil {
		t.Errorf("expected success for developer activating plan-4: %v", err)
	}

	// Blocked role test
	secGuest := &pkgctx.SecurityContext{Roles: []string{"guest"}}
	if err := wcv.ValidatePriorityPlanActivation(ctx, secGuest, "plan-4"); err == nil {
		t.Errorf("expected error for guest blocked role")
	}

	// Missing required role test
	secViewer := &pkgctx.SecurityContext{Roles: []string{"viewer"}}
	if err := wcv.ValidatePriorityPlanActivation(ctx, secViewer, "plan-4"); err == nil {
		t.Errorf("expected error for viewer missing required role")
	}

	// Workstream operation validation
	// Case 1: Workstream nonexistent
	if err := wcv.ValidateWorkstreamOperation(ctx, secCtx, "ws-missing", "create", "workstream"); err == nil {
		t.Errorf("expected error for missing workstream")
	}

	// Case 2: Workstream with no workflow_ref and no priority_plan_ref
	mock.data["ws-1"] = map[string]any{
		objects.FieldKeyID:   "ws-1",
		objects.FieldKeyKind: "workstream",
	}
	if err := wcv.ValidateWorkstreamOperation(ctx, secCtx, "ws-1", "create", "workstream"); err != nil {
		t.Errorf("expected success for unconstrained workstream: %v", err)
	}

	// Case 3: Workstream inheriting workflow from priority plan
	mock.data["ws-2"] = map[string]any{
		objects.FieldKeyID:              "ws-2",
		objects.FieldKeyKind:            "workstream",
		objects.FieldKeyPriorityPlanRef: "plan-4",
	}
	// Developer performing "create" on "workstream" -> allowed
	if err := wcv.ValidateWorkstreamOperation(ctx, secCtx, "ws-2", "create", "workstream"); err != nil {
		t.Errorf("expected success for ws2 create workstream: %v", err)
	}

	// Developer performing disallowed operation "delete" on "workstream"
	if err := wcv.ValidateWorkstreamOperation(ctx, secCtx, "ws-2", "delete", "workstream"); err == nil {
		t.Errorf("expected error for disallowed operation 'delete'")
	}

	// Developer operating on blocked kind "secret"
	if err := wcv.ValidateWorkstreamOperation(ctx, secCtx, "ws-2", "create", "secret"); err == nil {
		t.Errorf("expected error for blocked kind 'secret'")
	}

	// Developer operating on unlisted kind "other" (since allowed_kinds is restricted)
	if err := wcv.ValidateWorkstreamOperation(ctx, secCtx, "ws-2", "create", "other"); err == nil {
		t.Errorf("expected error for unlisted kind 'other'")
	}

	// Direct workflow_ref on workstream with disabled workflow
	mock.data["ws-3"] = map[string]any{
		objects.FieldKeyID:          "ws-3",
		objects.FieldKeyKind:        "workstream",
		objects.FieldKeyWorkflowRef: "wf-disabled",
	}
	if err := wcv.ValidateWorkstreamOperation(ctx, secCtx, "ws-3", "create", "workstream"); err == nil {
		t.Errorf("expected error on disabled workflow reference")
	}
}
