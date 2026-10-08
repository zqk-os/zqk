package app

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clitool "github.com/zqk-os/zqk/pkg/cli"
)

func TestBuildOperationDescription(t *testing.T) {
	// Without args
	m1 := &clitool.CommandMetric{Command: "status"}
	assert.Equal(t, "Command execution: status", buildOperationDescription(m1))

	// With args
	m2 := &clitool.CommandMetric{Command: "get", Args: []string{"task", "ATK-1"}}
	assert.Equal(t, "Command execution: get task ATK-1", buildOperationDescription(m2))
}

func TestDetermineSeverity(t *testing.T) {
	// 1. Failure -> High
	assert.Equal(t, auditSeverityHigh, determineSeverity(&clitool.CommandMetric{Success: false}))

	// 2. Deleted objects -> High
	assert.Equal(t, auditSeverityHigh, determineSeverity(&clitool.CommandMetric{
		Success:        true,
		ObjectsDeleted: []string{"OBJ-1"},
	}))

	// 3. Created objects -> Medium
	assert.Equal(t, auditSeverityMedium, determineSeverity(&clitool.CommandMetric{
		Success:        true,
		ObjectsCreated: []string{"OBJ-2"},
	}))

	// 4. Updated objects -> Medium
	assert.Equal(t, auditSeverityMedium, determineSeverity(&clitool.CommandMetric{
		Success:        true,
		ObjectsUpdated: []string{"OBJ-3"},
	}))

	// 5. Read-only clean success -> Low
	assert.Equal(t, auditSeverityLow, determineSeverity(&clitool.CommandMetric{
		Success: true,
	}))
}

func TestBuildAuditMetadata_AllFields(t *testing.T) {
	now := time.Now()
	m := &clitool.CommandMetric{
		Command:        "create",
		NormalizedCmd:  "create",
		Args:           []string{"task"},
		Flags:          map[string]any{"dry-run": false},
		Duration:       150 * time.Millisecond,
		StartTime:      now,
		EndTime:        now.Add(150 * time.Millisecond),
		ExitCode:       0,
		TimedOut:       false,
		ObjectsCreated: []string{"ATK-100"},
		ObjectsUpdated: []string{"PRI-1"},
		ObjectsDeleted: []string{"ATK-OLD"},
		PriorityPlan:   "PRI-PLAN-1",
		Workstream:     "WS-CORE",
		Milestone:      "MLS-V1",
		ActorID:        "ACC-DEV-1",
		ActorRoles:     []string{"developer"},
		Error:          "none",
	}

	meta := buildAuditMetadata("/tmp/test-project", m)
	require.NotNil(t, meta)
	assert.Equal(t, "cli", meta[auditMetaSource])
	assert.Equal(t, "/tmp/test-project", meta[auditMetaProjectRoot])
	assert.Equal(t, "create", meta[auditMetaCommand])
	assert.Equal(t, int64(150), meta[auditMetaDurationMs])
	assert.Equal(t, "PRI-PLAN-1", meta[auditMetaPriorityPlan])
	assert.Equal(t, "WS-CORE", meta[auditMetaWorkstream])
	assert.Equal(t, "MLS-V1", meta[auditMetaMilestone])
	assert.Equal(t, "ACC-DEV-1", meta[auditMetaActorID])
	assert.Equal(t, []string{"developer"}, meta[auditMetaActorRoles])
	assert.Equal(t, "none", meta[auditMetaError])
}

func TestValidateAndWriteAuditEvent_NoStorage(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	m := &clitool.CommandMetric{Command: "test"}
	err := validateAndWriteAuditEvent(cmd, "/tmp", "op", "low", nil, m, "profile")
	assert.NoError(t, err)
}

func TestEmitCommandExecutionEventViaCoordinator_EarlyReturns(t *testing.T) {
	ctx := context.Background()
	m := &clitool.CommandMetric{Command: "test"}
	// Empty project root
	emitCommandExecutionEventViaCoordinator(ctx, "", nil, "op", "low", nil, m, "profile")
	// Dot project root
	emitCommandExecutionEventViaCoordinator(ctx, ".", nil, "op", "low", nil, m, "profile")
}
