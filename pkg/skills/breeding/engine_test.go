package breeding_test

import (
	"context"
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

	// 1. Create a base AgentSkill
	skillID := "ASK-100"
	skillObj := map[string]any{
		objects.FieldKeyKind:                objects.KindAgentSkill,
		objects.FieldKeyID:                  skillID,
		objects.FieldKeyProvider:            "gemini",
		objects.FieldKeyFilePath:            "skills/ask-100",
		objects.FieldKeyInstructionsSummary: "Initial skill",
	}
	err = graphStore.Create(ctx, nil, skillObj)
	if err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}

	// 2. Create a MaturationReport with low fitness
	reportID := "MAT-100"
	reportObj := map[string]any{
		objects.FieldKeyKind:                objects.MaturationReport,
		objects.FieldKeyID:                  reportID,
		objects.FieldKeyComponentID:         skillID,
		objects.FieldKeyFitnessScore:        0.3,
		objects.FieldKeyObservationDuration: "24h",
		objects.FieldKeyGraduationStatus:    "maturation",
	}
	err = graphStore.Create(ctx, nil, reportObj)
	if err != nil {
		t.Fatalf("failed to create report: %v", err)
	}

	// 3. Initialize Breeding Engine
	mutator := &mockMutator{
		mutatedPath: "skills/ask-mutated",
	}
	engine := breeding.NewEngine(graphStore, mutator, 0.5)

	// 4. Run Breed
	err = engine.Breed(ctx)
	if err != nil {
		t.Fatalf("breed failed: %v", err)
	}

	// 5. Verify MutationProvider was called
	if mutator.called != 1 {
		t.Errorf("expected mutator to be called once, got %d", mutator.called)
	}

	// 6. Verify new skill was created
	skillsResult, err := graphStore.List(ctx, nil, nil, storage.ListFilter{Kind: objects.KindAgentSkill})
	if err != nil {
		t.Fatalf("failed to list skills: %v", err)
	}

	if len(skillsResult.Objects) != 2 {
		t.Errorf("expected 2 skills, got %d", len(skillsResult.Objects))
	}
}
