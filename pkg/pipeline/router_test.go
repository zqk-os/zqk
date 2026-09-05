package pipeline_test

import (
	"context"
	"io"
	"testing"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/pipeline"
)

type mockDS struct{}

func (m *mockDS) GetBacklogItemsForPlan(ctx context.Context, planID string) ([]string, error) {
	return []string{"BLI-1", "BLI-2", "BLI-3"}, nil
}

func (m *mockDS) GroupBacklogItems(ctx context.Context, backlogItems []string) ([][]string, error) {
	return [][]string{{"BLI-1", "BLI-2"}, {"BLI-3"}}, nil
}

func (m *mockDS) CreateAgentTask(ctx context.Context, planID string, backlogItems []string) (string, error) {
	return "TASK-" + backlogItems[0], nil
}

func (m *mockDS) AssignPersonaToTask(ctx context.Context, taskID string) (string, error) {
	if taskID == "TASK-BLI-1" {
		return "account:agent:coder", nil
	}
	return "account:agent:reviewer", nil
}

func (m *mockDS) SetTaskStatus(ctx context.Context, taskID string, status string) error {
	return nil
}

func (m *mockDS) WakeAgent(ctx context.Context, taskID string, persona string) error {
	return nil
}

func (m *mockDS) ScheduleHourglassFlip(ctx context.Context, taskID string) error {
	return nil
}

func TestRouter(t *testing.T) {
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(context.Background()))
	router := pipeline.NewRouter(logger, &mockDS{})

	res, err := router.Route(context.Background(), "PRI-123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(res.AgentTaskIDs) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(res.AgentTaskIDs))
	}

	if res.Assignments["TASK-BLI-1"] != "account:agent:coder" {
		t.Errorf("expected assignment to account:agent:coder, got %s", res.Assignments["TASK-BLI-1"])
	}

	if res.Assignments["TASK-BLI-3"] != "account:agent:reviewer" {
		t.Errorf("expected assignment to account:agent:reviewer, got %s", res.Assignments["TASK-BLI-3"])
	}
}

func BenchmarkRouter(b *testing.B) {
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(context.Background()))
	router := pipeline.NewRouter(logger, &mockDS{})
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = router.Route(ctx, "PRI-123")
	}
}
