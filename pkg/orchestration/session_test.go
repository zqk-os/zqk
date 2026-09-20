package orchestration

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSession(t *testing.T) {
	ctx := context.Background()

	s := NewSession(nil, nil)

	if s.Status() != "pending" {
		t.Errorf("expected status 'pending', got '%s'", s.Status())
	}

	err := s.AddAgent(ctx, AgentID("agent-1"))
	if err != nil {
		t.Fatalf("failed to add agent: %v", err)
	}

	err = s.Start(ctx, "architecture_design", nil)
	if err != nil {
		t.Fatalf("failed to start session: %v", err)
	}

	if s.Status() != "completed" {
		t.Errorf("expected status 'completed', got '%s'", s.Status())
	}
}

func TestSession_Cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := NewSession(nil, nil)
	err := s.Start(ctx, "architecture_design", nil)
	if err == nil {
		t.Fatalf("expected error from canceled context, got nil")
	}
	if s.Status() != "failed" {
		t.Errorf("expected status 'failed', got '%s'", s.Status())
	}
}

type MockStorage struct {
	storage.ObjectStorageProvider
}

func (m *MockStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if id == "POL-GATE-001" {
		return map[string]any{objects.FieldKeyID: "POL-GATE-001"}, nil
	}
	return nil, fileutil.ErrNotExist
}

func TestSession_TruthSentinel(t *testing.T) {
	ctx := context.Background()
	mockStore := &MockStorage{}

	s := NewSession(nil, mockStore)

	// Local merge without PR link should fail
	err := s.Start(ctx, "local_merge", map[string]any{
		"local_merge_to_main": true,
	})
	if err == nil {
		t.Errorf("expected error for local merge without PR link, got nil")
	}

	// Local merge with PR link should succeed
	s2 := NewSession(nil, mockStore)
	err = s2.Start(ctx, "local_merge_pr", map[string]any{
		"local_merge_to_main": true,
		"pr_link":             "https://github.com/zqk-os/zqk/pull/123",
	})
	if err != nil {
		t.Errorf("expected no error for local merge with PR link, got %v", err)
	}
}
