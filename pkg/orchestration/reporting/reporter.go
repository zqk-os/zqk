package reporting

import (
	"context"
	"fmt"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/orchestration"
)

// ProgressReport represents a human-readable update synthesized from system events.
type ProgressReport struct {
	PlanID  string
	Summary string
	Impact  string
}

// Reporter defines the interface for surfacing autonomous activity.
type Reporter interface {
	Report(ctx context.Context, report ProgressReport) error
}

// ConsoleReporter outputs reports to the human-facing audit stream.
type ConsoleReporter struct{}

func NewConsoleReporter() *ConsoleReporter {
	return &ConsoleReporter{}
}

func (r *ConsoleReporter) Report(ctx context.Context, report ProgressReport) error {
	logging.GetLoggerFromContext(ctx).LogInfo(fmt.Sprintf(orchestration.LogFmtAutonomousProgress, report.PlanID, report.Impact, report.Summary))
	return nil
}
