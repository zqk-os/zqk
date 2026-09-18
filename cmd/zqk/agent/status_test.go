package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/zqk-os/zqk/cmd/zqk/agent"
	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/stretchr/testify/assert"
)

func TestAgentStatusCmd(t *testing.T) {
	root, store := setupOrchestrateTest(t)

	// Save old budget and reset for tests
	oldBudget := goroutinelabels.DefaultBudget()
	t.Cleanup(func() {
		goroutinelabels.SetDefaultBudget(oldBudget)
	})
	goroutinelabels.SetDefaultBudget(nil)

	// Test 1: No active agents, no progress files
	t.Run("empty_status", func(t *testing.T) {
		goroutinelabels.SetDefaultBudget(nil) // Reset inside sub-test

		cmd := agent.NewStatusCmd()
		cli.SetContext(cmd, cli.ContextForProjectRoot(root))
		var buf bytes.Buffer
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.Execute()
		assert.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "ZQK Orchestrated Hive Status")
		assert.Contains(t, output, "No active swarm workers")
		assert.Regexp(t, `Active Workers: \d+ / 100`, output)
		assert.Contains(t, output, "Available:      ")
		assert.Contains(t, output, "Policy Compliance: no_activity")
	})

	// Test 2: Formatting output JSON
	t.Run("format_json", func(t *testing.T) {
		goroutinelabels.SetDefaultBudget(nil)

		cmd := agent.NewStatusCmd()
		cli.SetContext(cmd, cli.ContextForProjectRoot(root))
		cmd.SetArgs([]string{"--format", "json"})
		var buf bytes.Buffer
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.Execute()
		assert.NoError(t, err)

		var result map[string]any
		err = json.Unmarshal(buf.Bytes(), &result)
		assert.NoError(t, err)
		assert.Equal(t, "online", result["orchestrator_status"])
		assert.Equal(t, "no_activity", result["policy_compliance"])

		wb, ok := result["worker_budget"].(map[string]any)
		assert.True(t, ok)
		assert.Equal(t, float64(100), wb["total_capacity"])
		assert.Contains(t, wb, "active_count")
		assert.Contains(t, wb, "available")
	})

	// Test 3: Formatting output YAML
	t.Run("format_yaml", func(t *testing.T) {
		goroutinelabels.SetDefaultBudget(nil)

		cmd := agent.NewStatusCmd()
		cli.SetContext(cmd, cli.ContextForProjectRoot(root))
		cmd.SetArgs([]string{"--format", "yaml"})
		var buf bytes.Buffer
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.Execute()
		assert.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "orchestrator_status: online")
		assert.Contains(t, output, "policy_compliance: no_activity")
	})

	// Test 4: With active agent tasks
	t.Run("active_agents", func(t *testing.T) {
		goroutinelabels.SetDefaultBudget(nil)

		ctx := context.Background()
		secCtx := pkgctx.NewSystemSecurityContext()
		persona := map[string]any{
			objects.FieldKeyID:            "PER-STATUS-TEST",
			objects.FieldKeyKind:          objects.KindPersona,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyTitle:         "Status Test Persona",
			objects.FieldKeyName:          "Status Test Persona",
			objects.FieldKeyRole:          "agent",
			objects.FieldKeyStatus:        objects.ObjectStatusImplemented,
		}
		storage.CreateCASVisible(t, store, ctx, secCtx, persona, objects.ObjectStatusImplemented)

		task := map[string]any{
			objects.FieldKeyID:                 "ATK-1",
			objects.FieldKeyKind:               objects.KindAgentTask,
			objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
			objects.FieldKeyTitle:              "Testing worker budget",
			objects.FieldKeyAssigneePersonaRef: "PER-STATUS-TEST",
			objects.FieldKeyEstimatedEffort:    "1h",
			objects.FieldKeyUpdatedAt:          time.Now().Format(time.RFC3339),
		}
		storage.CreateCASVisible(t, store, ctx, secCtx, task, objects.ObjectStatusInProgress)

		// Flush storage
		flushCtx, flushCancel := storage.DurabilityFlushContext()
		err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, store, root, []string{objects.KindAgentTask})
		flushCancel()
		assert.NoError(t, err)

		cmd := agent.NewStatusCmd()
		cli.SetContext(cmd, cli.ContextForProjectRoot(root))
		var buf bytes.Buffer
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err = cmd.Execute()
		assert.NoError(t, err)

		output := buf.String()
		assert.Regexp(t, `Active Workers: \d+ / 100`, output)
		assert.Contains(t, output, "Available:      ")
		assert.Contains(t, output, "- ATK-1 [in_progress] - Testing worker budget")
	})

	// Test 5: Policy Compliance heartbeat (fresh)
	t.Run("policy_compliance_fresh", func(t *testing.T) {
		goroutinelabels.SetDefaultBudget(nil)

		ctx := context.Background()
		secCtx := pkgctx.NewSystemSecurityContext()

		persona := map[string]any{
			objects.FieldKeyID:            "PER-HEARTBEAT-FRESH",
			objects.FieldKeyKind:          objects.KindPersona,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyTitle:         "Heartbeat Fresh Persona",
			objects.FieldKeyName:          "Heartbeat Fresh Persona",
			objects.FieldKeyRole:          "agent",
			objects.FieldKeyStatus:        objects.ObjectStatusImplemented,
		}
		storage.CreateCASVisible(t, store, ctx, secCtx, persona, objects.ObjectStatusImplemented)

		taskFresh := map[string]any{
			objects.FieldKeyID:                 "ATK-FRESH",
			objects.FieldKeyKind:               objects.KindAgentTask,
			objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
			objects.FieldKeyTitle:              "Testing fresh heartbeat",
			objects.FieldKeyUpdatedAt:          time.Now().Format(time.RFC3339),
			objects.FieldKeyAssigneePersonaRef: "PER-HEARTBEAT-FRESH",
			objects.FieldKeyEstimatedEffort:    "1h",
		}
		storage.CreateCASVisible(t, store, ctx, secCtx, taskFresh, objects.ObjectStatusInProgress)

		flushCtx, flushCancel := storage.DurabilityFlushContext()
		err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, store, root, []string{objects.KindAgentTask})
		flushCancel()
		assert.NoError(t, err)

		cmd := agent.NewStatusCmd()
		cli.SetContext(cmd, cli.ContextForProjectRoot(root))
		var buf bytes.Buffer
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err = cmd.Execute()
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Policy Compliance: heartbeat_ok")
	})

	// Test 6: Policy Compliance heartbeat (stale mod time → heartbeat_stale)
	t.Run("policy_compliance_stale", func(t *testing.T) {
		stale := time.Now().Add(-20 * time.Minute)
		assert.Equal(t, "heartbeat_stale", agent.HeartbeatComplianceFromModTime(stale))
	})
}
