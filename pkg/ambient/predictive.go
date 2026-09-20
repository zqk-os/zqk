package ambient

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// PredictiveTaskSpawner observes the ambient event hub and preemptively spawns tasks.
type PredictiveTaskSpawner struct {
	hub EventHub
}

// NewPredictiveTaskSpawner initializes a new PredictiveTaskSpawner and subscribes it to the hub.
func NewPredictiveTaskSpawner(hub EventHub) *PredictiveTaskSpawner {
	s := &PredictiveTaskSpawner{hub: hub}
	hub.Subscribe(EventTypeFilesystem, s.handleFilesystem)
	return s
}

func (s *PredictiveTaskSpawner) handleFilesystem(ctx context.Context, event Event) error {
	payload, ok := event.Payload.(map[string]any)
	if !ok {
		return nil
	}

	target, _ := payload[objects.FieldKeyTargetID].(string)
	op, _ := payload[objects.FieldKeyOperation].(string)
	source, _ := payload[objects.FieldKeySource].(string)

	if source == "fswatcher" && strings.Contains(op, "CREATE") && strings.HasSuffix(target, ".go") && !strings.HasSuffix(target, "_test.go") {
		testFile := strings.TrimSuffix(target, ".go") + "_test.go"

		// Generate test file stub if it doesn't exist
		if _, err := fileutil.Stat(testFile); fileutil.IsNotExist(err) {
			pkgName := filepath.Base(filepath.Dir(target))
			if pkgName == "." || pkgName == "" {
				pkgName = "main" // fallback
			}
			stub := "package " + pkgName + "\n\nimport \"testing\"\n"
			_ = fileutil.WriteFile(testFile, []byte(stub), 0644)
		}

		// Schedule a background code-quality vet
		_ = s.hub.Publish(ctx, Event{
			Type: EventTypeSession,
			Payload: map[string]any{
				objects.FieldKeySource:      "predictive_spawner",
				"heuristic_type":            "schedule_vet",
				objects.FieldKeyTargetID:    target,
				objects.FieldKeyDescription: "Scheduled background code-quality vet for " + target,
			},
			Timestamp: time.Now(),
		})
	}

	if source == "fswatcher" && strings.Contains(op, "WRITE") && (strings.Contains(target, "priority_plans/") || strings.Contains(target, "backlog_items/") || strings.Contains(target, "backlog/")) {
		_ = s.hub.Publish(ctx, Event{
			Type: EventTypeSession,
			Payload: map[string]any{
				objects.FieldKeySource:      "predictive_spawner",
				"heuristic_type":            "stage_convergence_session",
				objects.FieldKeyTargetID:    target,
				objects.FieldKeyDescription: "Autonomously staged preemptive convergence session for " + target,
			},
			Timestamp: time.Now(),
		})
	}
	return nil
}
