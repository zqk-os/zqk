package breeding_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/skills/breeding"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

type mockMutator struct {
	mutatedPath string
	err         error
	called      int
}

func (m *mockMutator) MutateSkill(ctx context.Context, originalFilePath string, feedback []string) (string, error) {
	m.called++
	return m.mutatedPath, m.err
}

func TestEngine_Breed(t *testing.T) {
	if os.Getenv(zqkenv.GraphEnabled()) != "true" {
		t.Skip("Graph backend not enabled (set ZQK_GRAPH_ENABLED=true)")
	}

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 2*time.Minute)
	t.Cleanup(cancel)
	ctx = storage.WithTestHardDelete(ctx)
	secCtx := pkgctx.NewSystemSecurityContext()

	pool := testkit.PrepareGraphConnectionForTest(t)
	conn, err := pool.GetConnection(ctx)
	if err != nil {
		t.Fatalf("failed to acquire connection: %v", err)
	}
	t.Cleanup(func() { pool.ReturnConnection(conn) })

	tempProject := testkit.PrepareIsolatedTempProject(t, nil)

	// Create Graph Storage
	graphStore, err := storage.NewGraphObjectStorage(conn, tempProject.Root)
	if err != nil {
		t.Fatalf("failed to create graph store: %v", err)
	}

	uniq := time.Now().UnixNano()
	skillID := fmt.Sprintf("ASK-%d", uniq)
	reportID := fmt.Sprintf("MAT-%d", uniq)
	t.Cleanup(func() {
		_ = graphStore.Delete(ctx, secCtx, skillID, true)
		_ = graphStore.Delete(ctx, secCtx, reportID, true)
	})

	// 1. Create a base AgentSkill
	skillObj := map[string]any{
		objects.FieldKeyKind:                objects.KindAgentSkill,
		objects.FieldKeyID:                  skillID,
		objects.FieldKeyProvider:            "gemini",
		objects.FieldKeyFilePath:            "skills/ask-100",
		objects.FieldKeyInstructionsSummary: "Initial skill",
	}
	if err := graphStore.Create(ctx, secCtx, skillObj); err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}

	// 2. Create a MaturationReport with low fitness
	reportObj := map[string]any{
		objects.FieldKeyKind:                objects.MaturationReport,
		objects.FieldKeyID:                  reportID,
		objects.FieldKeyComponentID:         skillID,
		objects.FieldKeyFitnessScore:        0.3,
		objects.FieldKeyObservationDuration: "24h",
		objects.FieldKeyGraduationStatus:    "maturation",
	}
	if err := graphStore.Create(ctx, secCtx, reportObj); err != nil {
		t.Fatalf("failed to create report: %v", err)
	}

	// 3. Initialize Breeding Engine
	mutator := &mockMutator{
		mutatedPath: "skills/ask-mutated",
	}
	engine := breeding.NewEngine(graphStore, mutator, 0.5)

	// 4. Run Breed
	if err := engine.Breed(ctx); err != nil {
		t.Fatalf("breed failed: %v", err)
	}

	// 5. Verify MutationProvider was called
	if mutator.called != 1 {
		t.Errorf("expected mutator to be called once, got %d", mutator.called)
	}

	// 6. Verify a new skill was created (scoped to this run — test DB may retain older skills)
	skillsResult, err := graphStore.List(ctx, secCtx, nil, storage.ListFilter{Kind: objects.KindAgentSkill})
	if err != nil {
		t.Fatalf("failed to list skills: %v", err)
	}
	foundMutated := false
	for _, obj := range skillsResult.Objects {
		if p, _ := obj[objects.FieldKeyFilePath].(string); p == "skills/ask-mutated" {
			foundMutated = true
			if id, ok := obj[objects.FieldKeyID].(string); ok && id != "" {
				t.Cleanup(func() { _ = graphStore.Delete(ctx, secCtx, id, true) })
			}
			break
		}
	}
	if !foundMutated {
		t.Errorf("expected mutated skill with file_path skills/ask-mutated; listed %d skill(s)", len(skillsResult.Objects))
	}
}
