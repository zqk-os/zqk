package agentprompt

import (
	"context"
	"errors"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/observer"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type onboardingMockStorage struct {
	storage.ObjectStorageProvider
	readErr error
	listErr error
}

func (m *onboardingMockStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	if id == OnboardingPromptTemplateID {
		return map[string]any{
			objects.FieldKeyID:         OnboardingPromptTemplateID,
			objects.FieldKeyPromptBody: "Welcome to ZQK Onboarding",
		}, nil
	}
	return nil, storage.ErrObjectNotFound
}

func (m *onboardingMockStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return &storage.QueryResult{
		Objects: []map[string]any{
			{
				objects.FieldKeyID:          "POL-001",
				objects.FieldKeyTitle:       "Test Policy",
				objects.FieldKeyDescription: "Always write clean tests",
				objects.FieldKeyStatus:      "active",
			},
		},
	}, nil
}

func TestBuildOnboardingPrompt(t *testing.T) {
	ctx := context.Background()

	// 1. Successful build
	sp := &onboardingMockStorage{}
	out, err := BuildOnboardingPrompt(ctx, sp, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Welcome to ZQK Onboarding") {
		t.Fatalf("expected onboarding body in prompt, got: %s", out)
	}
	if !strings.Contains(out, "Test Policy") {
		t.Fatalf("expected policy in prompt, got: %s", out)
	}

	// 2. Read error
	failReadSp := &onboardingMockStorage{readErr: storage.ErrObjectNotFound}
	if _, err := BuildOnboardingPrompt(ctx, failReadSp, 200); err == nil {
		t.Fatal("expected error on template read failure")
	}

	// 3. List error
	failListSp := &onboardingMockStorage{listErr: errors.New("storage error")}
	if _, err := BuildOnboardingPrompt(ctx, failListSp, 200); err == nil {
		t.Fatal("expected error on policies list failure")
	}
}

func TestObserverContext_PromptSections(t *testing.T) {
	obs := &ObserverContext{}
	sec := obs.GeneratePromptSection()
	if !strings.Contains(sec, "Codebase Structural Context") {
		t.Fatalf("unexpected prompt section: %s", sec)
	}

	// Empty result census
	censusEmpty := obs.GeneratePromptSectionCensus()
	if !strings.Contains(censusEmpty, "No significant AST structures") {
		t.Fatalf("unexpected empty census: %s", censusEmpty)
	}

	// Populated census
	obsWithEntities := &ObserverContext{
		Result: &observer.ExtractResult{
			Entities: []observer.Entity{
				{
					Name:       "RunWorker",
					Kind:       "func",
					File:       "pkg/worker/worker.go",
					Signature:  "func RunWorker() error",
					Complexity: 15,
				},
				{
					Name:       "Init",
					Kind:       "func",
					File:       "pkg/worker/worker.go",
					Signature:  "func Init()",
					Complexity: 2,
				},
				{
					Name:       "Config",
					Kind:       "struct",
					File:       "pkg/config/config.go",
					Signature:  "type Config struct",
					Complexity: 0,
				},
			},
		},
	}
	censusPopulated := obsWithEntities.GeneratePromptSectionCensus()
	if !strings.Contains(censusPopulated, "HIGH COMPLEXITY") {
		t.Fatalf("expected high complexity marker in census, got: %s", censusPopulated)
	}
	if !strings.Contains(censusPopulated, "RunWorker") {
		t.Fatalf("expected RunWorker in census, got: %s", censusPopulated)
	}
}

func TestAutonomousFeedback_Populated(t *testing.T) {
	fb := &AutonomousFeedback{
		FeedbackItems: []map[string]any{
			{
				objects.FieldKeyTitle:            "Fix flaky test",
				objects.FieldKeyID:               "FB-001",
				objects.FieldKeyTargetReport:     "reports/ci-flakiness.json",
				objects.FieldKeySuggestedActions: []any{"Add synchronization barrier", "Avoid tight polling loops"},
			},
		},
	}

	prompt := fb.GeneratePromptSection()
	if !strings.Contains(prompt, "Fix flaky test") {
		t.Fatalf("expected title in prompt: %s", prompt)
	}
	if !strings.Contains(prompt, "reports/ci-flakiness.json") {
		t.Fatalf("expected target report in prompt: %s", prompt)
	}
	if !strings.Contains(prompt, "synchronization barrier") {
		t.Fatalf("expected action in prompt: %s", prompt)
	}
}

func TestWorkClass_IsCoding(t *testing.T) {
	if !WorkClassCoding.IsCoding() {
		t.Fatal("WorkClassCoding should return true for IsCoding")
	}
	if WorkClassDocsEval.IsCoding() {
		t.Fatal("WorkClassDocsEval should return false for IsCoding")
	}
}

func TestLoadRelevantSkills(t *testing.T) {
	ctx := context.Background()
	sp := &onboardingMockStorage{}
	secCtx := pkgctx.NewSystemSecurityContext()
	skills, err := LoadRelevantSkills(ctx, sp, secCtx, "test query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skills == nil {
		t.Fatal("expected non-nil skills environment")
	}
}
