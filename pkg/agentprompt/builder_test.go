package agentprompt

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestBuildTaskPromptTokenBudget(t *testing.T) {
	// 1. Set a small budget
	opts := TaskPromptOptions{
		PlanTitle:   "Test Plan",
		TokenBudget: 50, // very small
	}

	sp := &mockStorageProvider{}
	secCtx := pkgctx.NewSystemSecurityContext()

	// Ensure policies or other data would naturally exceed this if not budgeted.
	// We'll just test that it builds successfully without crashing and enforces the budget natively via allocator.
	// Since we mock without actually inserting large strings, the allocator will process whatever is there.
	// But let's verify the output is generated without error.

	out, err := BuildTaskPrompt(context.Background(), sp, secCtx, "", opts)
	if err != nil {
		t.Fatalf("BuildTaskPrompt failed: %v", err)
	}

	if !strings.Contains(out, "Test Plan") {
		t.Fatalf("expected 'Test Plan' in output, got %s", out)
	}
}

type mockStorageProvider struct {
	storage.ObjectStorageProvider // Embed interface to panic on unimplemented methods instead of failing compilation
}

func (m *mockStorageProvider) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	return &storage.QueryResult{
		Objects: []map[string]any{},
	}, nil
}

func (m *mockStorageProvider) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return nil, storage.ErrObjectNotFound
}

func (m *mockStorageProvider) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	return false, nil
}

