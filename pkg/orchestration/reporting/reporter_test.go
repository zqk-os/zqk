package reporting

import (
	"context"
	"testing"
)

func TestConsoleReporter(t *testing.T) {
	reporter := NewConsoleReporter()
	err := reporter.Report(context.Background(), ProgressReport{
		PlanID:  "test-plan",
		Summary: "Test summary",
		Impact:  "High",
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}
