package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestContinuationHook_AntigravityActivePlan(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:            "hook_test",
		SeedSchemaPlane: true,
	})
	root, fs := proj.Root, proj.FileStorage
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Seed active priority plan
	pri := map[string]any{
		objects.FieldKeyID:            "PRI-HOOK-TEST-001",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Cycle 40: Autonomous Loop Advancement",
		objects.FieldKeyDescription:   "Advance autonomous progression",
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	require.NoError(t, fs.Create(context.Background(), secCtx, pri))

	// 2. Touch cap_loop_active marker
	stateDir := filepath.Join(root, paths.ProjectDataDir, "state")
	require.NoError(t, fileutil.MkdirAll(stateDir, paths.DirPerm755))
	markerPath := filepath.Join(stateDir, "cap_loop_active")
	require.NoError(t, fileutil.WriteFile(markerPath, []byte("active"), 0644))

	// 3. Run hook with Antigravity vendor
	inReader := strings.NewReader(`{"executionNum":1,"terminationReason":"model_stop"}`)
	var outBuf bytes.Buffer

	err := runContinuationHookWithDeps(context.Background(), secCtx, fs, root, "antigravity", markerPath, "", "", inReader, &outBuf)
	require.NoError(t, err)

	var hookOut continuationHookOutput
	require.NoError(t, json.Unmarshal(outBuf.Bytes(), &hookOut))

	assert.Equal(t, "continue", hookOut.Decision)
	assert.Contains(t, hookOut.Reason, "PRI-HOOK-TEST-001")
	assert.Contains(t, hookOut.Reason, "Cycle 40: Autonomous Loop Advancement")
	assert.Contains(t, hookOut.Reason, "CONTINUOUS AUTONOMOUS PROGRESSION")
}

func TestContinuationHook_Inactive_AllowsStop(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:            "hook_test",
		SeedSchemaPlane: true,
	})
	root, fs := proj.Root, proj.FileStorage
	secCtx := pkgctx.NewSystemSecurityContext()

	// Ensure marker file does NOT exist
	markerPath := filepath.Join(root, paths.ProjectDataDir, "state", "cap_loop_active")

	inReader := strings.NewReader(`{"executionNum":1,"terminationReason":"model_stop"}`)
	var outBuf bytes.Buffer

	err := runContinuationHookWithDeps(context.Background(), secCtx, fs, root, "antigravity", markerPath, "", "", inReader, &outBuf)
	require.NoError(t, err)

	var hookOut continuationHookOutput
	require.NoError(t, json.Unmarshal(outBuf.Bytes(), &hookOut))

	assert.Equal(t, "allow", hookOut.Decision)
}

func TestContinuationHook_BoundaryValidationAndErrorRecovery(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:            "hook_test",
		SeedSchemaPlane: true,
	})
	root, fs := proj.Root, proj.FileStorage
	secCtx := pkgctx.NewSystemSecurityContext()

	stateDir := filepath.Join(root, paths.ProjectDataDir, "state")
	require.NoError(t, fileutil.MkdirAll(stateDir, paths.DirPerm755))
	markerPath := filepath.Join(stateDir, "cap_loop_active")
	require.NoError(t, fileutil.WriteFile(markerPath, []byte("active"), 0644))

	// Subtest 1: Empty stdin
	{
		var outBuf bytes.Buffer
		err := runContinuationHookWithDeps(context.Background(), secCtx, fs, root, "antigravity", markerPath, "", "", strings.NewReader(""), &outBuf)
		require.NoError(t, err)
		var hookOut continuationHookOutput
		require.NoError(t, json.Unmarshal(outBuf.Bytes(), &hookOut))
		assert.Equal(t, "continue", hookOut.Decision)
	}

	// Subtest 2: Malformed JSON stdin
	{
		var outBuf bytes.Buffer
		err := runContinuationHookWithDeps(context.Background(), secCtx, fs, root, "antigravity", markerPath, "", "", strings.NewReader("{invalid-json"), &outBuf)
		require.NoError(t, err)
		var hookOut continuationHookOutput
		require.NoError(t, json.Unmarshal(outBuf.Bytes(), &hookOut))
		assert.Equal(t, "continue", hookOut.Decision)
	}

	// Subtest 3: Nil storage provider falls back safely without panic
	{
		var outBuf bytes.Buffer
		err := runContinuationHookWithDeps(context.Background(), secCtx, nil, root, "antigravity", markerPath, "", "", strings.NewReader("{}"), &outBuf)
		require.NoError(t, err)
		var hookOut continuationHookOutput
		require.NoError(t, json.Unmarshal(outBuf.Bytes(), &hookOut))
		assert.Equal(t, "continue", hookOut.Decision)
	}

	// Subtest 4: Generic vendor format
	{
		var outBuf bytes.Buffer
		err := runContinuationHookWithDeps(context.Background(), secCtx, fs, root, "generic", markerPath, "", "", strings.NewReader("{}"), &outBuf)
		require.NoError(t, err)
		var hookOut continuationHookOutput
		require.NoError(t, json.Unmarshal(outBuf.Bytes(), &hookOut))
		assert.Equal(t, "continue", hookOut.Decision)
	}

	// Subtest 5: Claude vendor format (decision is "block")
	{
		var outBuf bytes.Buffer
		err := runContinuationHookWithDeps(context.Background(), secCtx, fs, root, "claude", markerPath, "", "", strings.NewReader("{}"), &outBuf)
		require.NoError(t, err)
		var rawMap map[string]any
		require.NoError(t, json.Unmarshal(outBuf.Bytes(), &rawMap))
		assert.Equal(t, "block", rawMap["decision"])
		assert.NotEmpty(t, rawMap["reason"])
	}

	// Subtest 6: Cursor vendor format (decision is "continue" with message & reason)
	{
		var outBuf bytes.Buffer
		err := runContinuationHookWithDeps(context.Background(), secCtx, fs, root, "cursor", markerPath, "", "", strings.NewReader("{}"), &outBuf)
		require.NoError(t, err)
		var rawMap map[string]any
		require.NoError(t, json.Unmarshal(outBuf.Bytes(), &rawMap))
		assert.Equal(t, "continue", rawMap["decision"])
		assert.NotEmpty(t, rawMap["reason"])
		assert.Equal(t, rawMap["reason"], rawMap["message"])
	}
}

func TestContinuationHook_SteeringFeedIntegration(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:            "hook_test",
		SeedSchemaPlane: true,
	})
	root, fs := proj.Root, proj.FileStorage
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Seed active priority plan
	pri := map[string]any{
		objects.FieldKeyID:            "PRI-HOOK-TEST-002",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Cycle 41: Distributed Mesh Telemetry",
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	require.NoError(t, fs.Create(context.Background(), secCtx, pri))

	// 2. Touch cap_loop_active marker
	stateDir := filepath.Join(root, paths.ProjectDataDir, "state")
	require.NoError(t, fileutil.MkdirAll(stateDir, paths.DirPerm755))
	markerPath := filepath.Join(stateDir, "cap_loop_active")
	require.NoError(t, fileutil.WriteFile(markerPath, []byte("active"), 0644))

	// 3. Append steering message addressed to coordinator-1
	steerRes, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
		ProjectRoot:      root,
		Message:          "Focus on boundary verification and race safety",
		AgentID:          "human-tpm",
		ToAgentID:        "coordinator-1",
		EventType:        agentfeed.FeedEventTypeSteering,
		SkipEnabledCheck: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, steerRes.EventID)

	// 4. Run hook with explicit agent ID
	var outBuf bytes.Buffer
	err = runContinuationHookWithDeps(context.Background(), secCtx, fs, root, "antigravity", markerPath, "coordinator-1", "", strings.NewReader("{}"), &outBuf)
	require.NoError(t, err)

	var hookOut continuationHookOutput
	require.NoError(t, json.Unmarshal(outBuf.Bytes(), &hookOut))

	assert.Equal(t, "continue", hookOut.Decision)
	assert.Contains(t, hookOut.Reason, "Pending steering inbox (1 unacked message(s))")
	assert.Contains(t, hookOut.Reason, "Focus on boundary verification and race safety")
	assert.Contains(t, hookOut.Reason, "human-tpm")
}

func TestContinuationHook_SafeDrainInput(t *testing.T) {
	data, err := safeDrainInput(nil)
	assert.NoError(t, err)
	assert.Nil(t, data)

	data, err = safeDrainInput(strings.NewReader("test-payload"))
	assert.NoError(t, err)
	assert.Equal(t, "test-payload", string(data))
}

func TestContinuationHook_BroadcastSteeringInboxFallback(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:            "hook_test",
		SeedSchemaPlane: true,
	})
	root, fs := proj.Root, proj.FileStorage
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Touch cap_loop_active marker
	stateDir := filepath.Join(root, paths.ProjectDataDir, "state")
	require.NoError(t, fileutil.MkdirAll(stateDir, paths.DirPerm755))
	markerPath := filepath.Join(stateDir, "cap_loop_active")
	require.NoError(t, fileutil.WriteFile(markerPath, []byte("active"), 0644))

	// 2. Append broadcast steering message (no ToAgentID)
	steerRes, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
		ProjectRoot:      root,
		Message:          "General broadcast steering for active cycle",
		AgentID:          "human-tpm",
		EventType:        agentfeed.FeedEventTypeSteering,
		SkipEnabledCheck: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, steerRes.EventID)

	// 3. Run hook with empty agent ID (trigger fallback to coordinator)
	var outBuf bytes.Buffer
	err = runContinuationHookWithDeps(context.Background(), secCtx, fs, root, "antigravity", markerPath, "", "", strings.NewReader("{}"), &outBuf)
	require.NoError(t, err)

	var hookOut continuationHookOutput
	require.NoError(t, json.Unmarshal(outBuf.Bytes(), &hookOut))

	assert.Equal(t, "continue", hookOut.Decision)
	assert.Contains(t, hookOut.Reason, "Pending steering inbox (1 unacked message(s))")
	assert.Contains(t, hookOut.Reason, "General broadcast steering for active cycle")
}
