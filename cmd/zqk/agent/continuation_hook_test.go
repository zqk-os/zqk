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

	err := runContinuationHookWithDeps(context.Background(), secCtx, fs, root, "antigravity", markerPath, inReader, &outBuf)
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

	err := runContinuationHookWithDeps(context.Background(), secCtx, fs, root, "antigravity", markerPath, inReader, &outBuf)
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
		err := runContinuationHookWithDeps(context.Background(), secCtx, fs, root, "antigravity", markerPath, strings.NewReader(""), &outBuf)
		require.NoError(t, err)
		var hookOut continuationHookOutput
		require.NoError(t, json.Unmarshal(outBuf.Bytes(), &hookOut))
		assert.Equal(t, "continue", hookOut.Decision)
	}

	// Subtest 2: Malformed JSON stdin
	{
		var outBuf bytes.Buffer
		err := runContinuationHookWithDeps(context.Background(), secCtx, fs, root, "antigravity", markerPath, strings.NewReader("{invalid-json"), &outBuf)
		require.NoError(t, err)
		var hookOut continuationHookOutput
		require.NoError(t, json.Unmarshal(outBuf.Bytes(), &hookOut))
		assert.Equal(t, "continue", hookOut.Decision)
	}

	// Subtest 3: Nil storage provider falls back safely without panic
	{
		var outBuf bytes.Buffer
		err := runContinuationHookWithDeps(context.Background(), secCtx, nil, root, "antigravity", markerPath, strings.NewReader("{}"), &outBuf)
		require.NoError(t, err)
		var hookOut continuationHookOutput
		require.NoError(t, json.Unmarshal(outBuf.Bytes(), &hookOut))
		assert.Equal(t, "continue", hookOut.Decision)
	}

	// Subtest 4: Generic vendor format
	{
		var outBuf bytes.Buffer
		err := runContinuationHookWithDeps(context.Background(), secCtx, fs, root, "generic", markerPath, strings.NewReader("{}"), &outBuf)
		require.NoError(t, err)
		var hookOut continuationHookOutput
		require.NoError(t, json.Unmarshal(outBuf.Bytes(), &hookOut))
		assert.Equal(t, "continue", hookOut.Decision)
	}
}
