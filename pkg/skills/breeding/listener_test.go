package breeding_test

import (
	"context"
	"fmt"
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
	ctx = storage.WithTestHardDelete(ctx)
	secCtx := pkgctx.NewSystemSecurityContext()

	pool := testkit.PrepareGraphConnectionForTest(t)
	conn, err := pool.GetConnection(ctx)
	if err != nil {
		t.Fatalf("failed to acquire connection: %v", err)
	}
	t.Cleanup(func() { pool.ReturnConnection(conn) })

	tempProject := testkit.PrepareIsolatedTempProject(t, nil)

	graphStore, err := storage.NewGraphObjectStorage(conn, tempProject.Root)
	if err != nil {
		t.Fatalf("failed to create graph store: %v", err)
	}

	uniq := time.Now().UnixNano()
	skillID := fmt.Sprintf("ASK-listener-%d", uniq)
	reportID := fmt.Sprintf("MAT-listener-%d", uniq)
	t.Cleanup(func() {
		_ = graphStore.Delete(ctx, secCtx, skillID, true)
		_ = graphStore.Delete(ctx, secCtx, reportID, true)
	})

	skillObj := map[string]any{
		objects.FieldKeyKind:                objects.KindAgentSkill,
		objects.FieldKeyID:                  skillID,
		objects.FieldKeyProvider:            "gemini",
		objects.FieldKeyFilePath:            "skills/ask-listener",
		objects.FieldKeyInstructionsSummary: "Initial skill",
	}
	if err := graphStore.Create(ctx, secCtx, skillObj); err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}

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

	mutator := &mockMutator{
		mutatedPath: "skills/ask-mutated-listener",
	}

	engine := breeding.NewEngine(graphStore, mutator, 0.5)

	router := events.NewRouter()
	listener := breeding.NewListener(router, engine)

	listenerCtx, listenerCancel := context.WithCancel(ctx)
	defer listenerCancel()
	listener.Start(listenerCtx)

	// Unrelated event must not breed.
	router.Publish(events.Shape{
		Type:     events.EventTypeObjectMutated,
		Kind:     objects.KindAgentSkill,
		TargetID: "skill-unrelated",
	})
	time.Sleep(100 * time.Millisecond)
	if mutator.called != 0 {
		t.Fatalf("expected 0 mutator calls after unrelated event, got %d", mutator.called)
	}

	router.Publish(events.Shape{
		Type:     events.EventTypeObjectMutated,
		Kind:     objects.MaturationReport,
		TargetID: reportID,
	})

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if mutator.called == 1 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected 1 mutator call after maturation_report event, got %d", mutator.called)
}
