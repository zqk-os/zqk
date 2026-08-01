package scheduler

import (
	"context"
	"github.com/lanceman/zqk/pkg/logging"
	"testing"
)

func TestIdleCleanupHandler(t *testing.T) {
	logger := logging.GetLoggerFromProfile("test")
	handler := NewIdleCleanupHandler(".", logger, nil)

	job := &ScheduledJob{
		ID: "SCH-IDLE-TEST-1",
	}

	err := handler.Execute(context.Background(), job)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
}
