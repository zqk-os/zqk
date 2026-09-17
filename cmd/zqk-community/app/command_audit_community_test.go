package app

import (
	"context"
	"testing"

	clitool "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestEmitCommandExecutionEventViaCoordinator_CommunitySkips(t *testing.T) {
	prev := zqkenv.IsCommunityEdition
	zqkenv.IsCommunityEdition = true
	t.Cleanup(func() { zqkenv.IsCommunityEdition = prev })

	// Nil storage would panic if the community skip did not return first.
	emitCommandExecutionEventViaCoordinator(
		context.Background(),
		"/tmp/zcom-community-audit-skip",
		nil,
		"Command execution: object list",
		auditSeverityLow,
		map[string]any{},
		&clitool.CommandMetric{Command: "object list"},
		profileHuman,
	)
}

func TestCreateCommandAuditEvent_CommunitySkips(t *testing.T) {
	prev := zqkenv.IsCommunityEdition
	zqkenv.IsCommunityEdition = true
	t.Cleanup(func() { zqkenv.IsCommunityEdition = prev })

	createCommandAuditEvent(nil, "/tmp/zcom-community-audit-skip", &clitool.CommandMetric{Command: "object list"}, profileHuman)
}
