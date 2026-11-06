package test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestBatchCriteriaExecution_CLI_Success(t *testing.T) {
	testRoot, sp := setupCycle140TestProject(t)
	var buf bytes.Buffer
	stdctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create shared script
	scriptRel := filepath.Join("scripts", "cli_batch_runner.sh")
	scriptAbs := filepath.Join(testRoot, scriptRel)
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(scriptAbs), 0755))

	logFile := filepath.Join(testRoot, "cli_invocations.log")
	scriptBody := fmt.Sprintf(`#!/usr/bin/env bash
echo "$@" >> '%s'
exit 0
`, logFile)
	require.NoError(t, fileutil.WriteFile(scriptAbs, []byte(scriptBody), 0755))

	crit1 := "CRIT-CLI-001"
	crit2 := "CRIT-CLI-002"
	for _, cid := range []string{crit1, crit2} {
		require.NoError(t, sp.Create(stdctx, secCtx, map[string]any{
			objects.FieldKeyID:          cid,
			objects.FieldKeyKind:        objects.KindCriteria,
			objects.FieldKeyTitle:       cid,
			objects.FieldKeyDescription: cid + " Description",
			objects.FieldKeyCategory:    "functional",
			objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
		}))
	}

	tcID := "TST-CLI-BATCH-001"
	require.NoError(t, sp.Create(stdctx, secCtx, map[string]any{
		objects.FieldKeyID:           tcID,
		objects.FieldKeyKind:         objects.KindTestCase,
		objects.FieldKeyTitle:        "CLI Batch Test",
		objects.FieldKeyDescription:  "CLI Batch Test Description",
		objects.FieldKeyStatus:       objects.ObjectStatusActive,
		objects.FieldKeyPathOrID:     scriptRel,
		objects.FieldKeyCriteriaRefs: []string{crit1, crit2},
	}))

	cmd := NewRunCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{"--batch=true", tcID})

	err := cmd.Execute()
	require.NoError(t, err)

	outStr := buf.String()
	assert.Contains(t, outStr, "[PASS]")
	assert.Contains(t, outStr, "2/2 passed")

	// Verify only 1 batch invocation was logged
	data, err := fileutil.ReadFile(logFile)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%s %s\n", crit1, crit2), string(data))
}

func TestBatchCriteriaExecution_CLI_FallbackOnFailure(t *testing.T) {
	testRoot, sp := setupCycle140TestProject(t)
	var buf bytes.Buffer
	stdctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	// Script that fails when given multiple arguments (batch),
	// but passes for CRIT-FB-001 and fails for CRIT-FB-002 when called individually.
	scriptRel := filepath.Join("scripts", "cli_fallback_runner.sh")
	scriptAbs := filepath.Join(testRoot, scriptRel)
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(scriptAbs), 0755))

	scriptBody := `#!/usr/bin/env bash
if [ "$#" -gt 1 ]; then
    exit 1
fi
if [ "$1" = "CRIT-FB-001" ]; then
    exit 0
fi
exit 2
`
	require.NoError(t, fileutil.WriteFile(scriptAbs, []byte(scriptBody), 0755))

	crit1 := "CRIT-FB-001"
	crit2 := "CRIT-FB-002"
	for _, cid := range []string{crit1, crit2} {
		require.NoError(t, sp.Create(stdctx, secCtx, map[string]any{
			objects.FieldKeyID:          cid,
			objects.FieldKeyKind:        objects.KindCriteria,
			objects.FieldKeyTitle:       cid,
			objects.FieldKeyDescription: cid + " Description",
			objects.FieldKeyCategory:    "functional",
			objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
		}))
	}

	tcID := "TST-CLI-FALLBACK-001"
	require.NoError(t, sp.Create(stdctx, secCtx, map[string]any{
		objects.FieldKeyID:           tcID,
		objects.FieldKeyKind:         objects.KindTestCase,
		objects.FieldKeyTitle:        "CLI Fallback Test",
		objects.FieldKeyDescription:  "CLI Fallback Test Description",
		objects.FieldKeyStatus:       objects.ObjectStatusActive,
		objects.FieldKeyPathOrID:     scriptRel,
		objects.FieldKeyCriteriaRefs: []string{crit1, crit2},
	}))

	cmd := NewRunCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{"--batch=true", tcID})

	err := cmd.Execute()
	require.Error(t, err)

	outStr := buf.String()
	// Should show fallback message
	assert.Contains(t, outStr, "Batch execution failed; fell back to isolated single-criterion execution")
	// Should show CRIT-FB-001 passed and CRIT-FB-002 failed
	assert.Contains(t, outStr, "[PASS] CRIT-FB-001")
	assert.Contains(t, outStr, "[FAIL] CRIT-FB-002")
	assert.Contains(t, outStr, "Criteria: 1/2 passed")
}
