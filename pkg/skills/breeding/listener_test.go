package breeding_test

import (
	"context"
	"os"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/events"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/skills/breeding"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestListenerTriggersBreeding(t *testing.T) {
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

	// Create a base AgentSkill so Breed has something to evaluate
	skillID := "ASK-100"
	skillObj := map[string]any{
		objects.FieldKeyKind:                objects.KindAgentSkill,
		objects.FieldKeyID:                  skillID,
		objects.FieldKeyProvider:            "gemini",
		objects.FieldKeyFilePath:            "skills/ask-100",
		objects.FieldKeyInstructionsSummary: "Initial skill",
	}
	_ = graphStore.Create(ctx, nil, skillObj)

	reportID := "MAT-100"
	reportObj := map[string]any{
		objects.FieldKeyKind:                objects.MaturationReport,
		objects.FieldKeyID:                  reportID,
		objects.FieldKeyComponentID:         skillID,
		objects.FieldKeyFitnessScore:        0.3,
		objects.FieldKeyObservationDuration: "24h",
		objects.FieldKeyGraduationStatus:    "maturation",
	}
	_ = graphStore.Create(ctx, nil, reportObj)

	mutator := &mockMutator{
		mutatedPath: "skills/ask-mutated-listener",
	}

	engine := breeding.NewEngine(graphStore, mutator, 0.5)

	router := events.NewRouter()
	listener := breeding.NewListener(router, engine)

	listenerCtx, listenerCancel := context.WithCancel(ctx)
	defer listenerCancel()
	listener.Start(listenerCtx)

	// Publish an unrelated event
	router.Publish(events.Shape{
		Type:     events.EventTypeObjectMutated,
		Kind:     objects.KindAgentSkill,
		TargetID: "skill-1",
	})

	// Ensure it has time to process (should be ignored)
	time.Sleep(100 * time.Millisecond)

	if mutator.called != 0 {
		t.Errorf("expected 0 mutator calls, got %d", mutator.called)
	}

	// Publish a matching event
	router.Publish(events.Shape{
		Type:     events.EventTypeObjectMutated,
		Kind:     objects.MaturationReport,
		TargetID: reportID,
	})

	// Wait briefly for async processing
	time.Sleep(200 * time.Millisecond)

	// Since we published a MaturationReport, the engine.Breed should have been called,
	if mutator.called != 1 {
		t.Errorf("expected 1 mutator call, got %d", mutator.called)
	}
}
