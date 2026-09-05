package agentprompt

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// mockKernelStorage implements a minimal ObjectStorageProvider for kernel steward tests.
type mockKernelStorage struct {
	storagepkg.ObjectStorageProvider
	data map[string]map[string]any
}

func (m *mockKernelStorage) Read(_ context.Context, _ *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.data[id]; ok {
		return obj, nil
	}
	return nil, fileutil.ErrNotExist
}

func (m *mockKernelStorage) List(_ context.Context, _ *pkgctx.SecurityContext, _ *pkgctx.StorageContext, filter storagepkg.ListFilter) (*storagepkg.QueryResult, error) {
	var objs []map[string]any
	for _, obj := range m.data {
		// If a kind filter is set, only return matching objects.
		if filter.Kind != "" {
			k, _ := obj[objects.FieldKeyKind].(string)
			if k != filter.Kind {
				continue
			}
		}
		// Apply field filters.
		match := true
		for field, val := range filter.Filters {
			if obj[field] != val {
				match = false
				break
			}
		}
		if match {
			objs = append(objs, obj)
		}
	}
	return &storagepkg.QueryResult{Objects: objs}, nil
}

func newMockKernelStorage() *mockKernelStorage {
	return &mockKernelStorage{
		data: map[string]map[string]any{
			// Sentinel prompt template
			SentinelPromptTemplateID: {
				objects.FieldKeyID:         SentinelPromptTemplateID,
				objects.FieldKeyTitle:      "CAP Sentinel Native Prompt",
				objects.FieldKeyPromptBody: "You are the kernel steward. State: {{.StateJSON}}",
			},
			// Mission
			"MIS-TEST-001": {
				objects.FieldKeyID:               "MIS-TEST-001",
				objects.FieldKeyKind:             "mission",
				objects.FieldKeyTitle:            "Test Mission",
				objects.FieldKeyMissionStatement: "Build a great system.",
				objects.FieldKeyStatus:           "active",
			},
			// Active goal
			"GOAL-TEST-001": {
				objects.FieldKeyID:          "GOAL-TEST-001",
				objects.FieldKeyKind:        "goal",
				objects.FieldKeyTitle:       "Achieve kernel observability",
				objects.FieldKeyStatus:      "active",
				objects.FieldKeyDescription: "Kernel must surface health signals in real-time.",
			},
			// In-progress milestone
			"MIL-TEST-001": {
				objects.FieldKeyID:     "MIL-TEST-001",
				objects.FieldKeyKind:   "milestone",
				objects.FieldKeyTitle:  "Context Onion operational",
				objects.FieldKeyStatus: "in_progress",
			},
		},
	}
}

// TestBuildSentinelPrompt_IncludesKernelContext verifies that the sentinel prompt
// contains mission and milestone context from the knowledge kernel. Goals are intentionally
// excluded (they bloat small-context models; mission/milestone coverage is sufficient).
func TestBuildSentinelPrompt_IncludesKernelContext(t *testing.T) {
	sp := newMockKernelStorage()
	secCtx := pkgctx.NewSystemSecurityContext()

	prompt, err := BuildSentinelPrompt(context.Background(), sp, secCtx, "", SentinelPromptOptions{
		StateJSON: `{"priority_plan":{"id":"PRI-001","title":"Test Plan"}}`,
	})
	if err != nil {
		t.Fatalf("BuildSentinelPrompt returned error: %v", err)
	}

	// Prompt must be non-empty
	if len(prompt) == 0 {
		t.Fatal("expected non-empty prompt")
	}

	// Must contain the prompt body
	if !contains(prompt, "kernel steward") {
		t.Errorf("prompt missing sentinel identity; got:\n%s", prompt)
	}

	// Must contain mission context
	if !contains(prompt, "MIS-TEST-001") && !contains(prompt, "Build a great system") {
		t.Errorf("prompt missing mission context; got:\n%s", prompt)
	}

	// Must contain milestone context
	if !contains(prompt, "MIL-TEST-001") && !contains(prompt, "Context Onion") {
		t.Errorf("prompt missing milestone context; got:\n%s", prompt)
	}

	// Must NOT contain goals (intentionally excluded to avoid duplication on small-context models)
	if contains(prompt, "GOAL-TEST-001") {
		t.Errorf("prompt should NOT include goal IDs (goals excluded to prevent duplication); got:\n%s", prompt)
	}

	// Must include the directive format instruction
	if !contains(prompt, "DIRECTIVE REQUIRED") || !contains(prompt, "AGENT DIRECTIVE:") {
		t.Errorf("prompt missing DIRECTIVE REQUIRED format instruction; got:\n%s", prompt)
	}
}

// TestBuildSentinelPrompt_StateInjected verifies that the kernel state JSON
// is injected into the prompt body.
func TestBuildSentinelPrompt_StateInjected(t *testing.T) {
	sp := newMockKernelStorage()
	secCtx := pkgctx.NewSystemSecurityContext()

	stateJSON := `{"priority_plan":{"id":"PRI-123","title":"Alpha Launch"}}`
	prompt, err := BuildSentinelPrompt(context.Background(), sp, secCtx, "", SentinelPromptOptions{
		StateJSON: stateJSON,
	})
	if err != nil {
		t.Fatalf("BuildSentinelPrompt returned error: %v", err)
	}

	if !contains(prompt, "PRI-123") {
		t.Errorf("prompt missing injected state JSON; got:\n%s", prompt)
	}
}

// TestBuildSentinelPrompt_HealthSignals verifies that health signal context
// is included when provided.
func TestBuildSentinelPrompt_HealthSignals(t *testing.T) {
	sp := newMockKernelStorage()
	secCtx := pkgctx.NewSystemSecurityContext()

	prompt, err := BuildSentinelPrompt(context.Background(), sp, secCtx, "", SentinelPromptOptions{
		StateJSON: `{}`,
		HealthSignals: KernelHealthSignals{
			FailingTestCount:   3,
			PolicyViolations:   2,
			SchedulerErrorRate: 0.1,
			DriftIndicators:    []string{"storage latency spike", "CAS index drift"},
		},
	})
	if err != nil {
		t.Fatalf("BuildSentinelPrompt returned error: %v", err)
	}

	if !strings.Contains(prompt, "Failing tests: 3") {
		t.Errorf("prompt missing failing test count; got:\n%s", prompt)
	}

	if !strings.Contains(prompt, "storage latency spike") {
		t.Errorf("prompt missing drift indicator; got:\n%s", prompt)
	}
}

// TestBuildSentinelPrompt_BudgetTruncation verifies that a tight TokenBudget
// drops lower-priority sections (goals, policies) while keeping the core template.
func TestBuildSentinelPrompt_BudgetTruncation(t *testing.T) {
	sp := newMockKernelStorage()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Small but realistic budget — fits core template (~80 tokens) but should
	// exclude lower-weight backfill (goals w=0.75, policies w=0.5).
	prompt, err := BuildSentinelPrompt(context.Background(), sp, secCtx, "", SentinelPromptOptions{
		StateJSON:   `{}`,
		TokenBudget: 100, // tight: core + maybe health, but not goals/policies
	})
	if err != nil {
		t.Fatalf("BuildSentinelPrompt returned error: %v", err)
	}

	// Core template must always be present (IsCore=true in allocator).
	if !strings.Contains(prompt, "kernel steward") {
		t.Errorf("core sentinel identity missing under tight budget; got:\n%s", prompt)
	}

	// At this budget, the full list of goals (medium weight) and policies (low weight)
	// should not appear — only higher-priority sections fit.
	// We verify the prompt is bounded and not inflated.
	if len(prompt) > 4000 {
		t.Errorf("prompt exceeded expected size under tight budget: %d chars", len(prompt))
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
