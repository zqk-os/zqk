package test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestBatchCriteriaExecution_CLI_Success(t *testing.T) {
	scriptTmpl := "#!/usr/bin/env bash\necho \"$@\" >> '%s'\nexit 0\n"
	critIDs := []string{"CRIT-CLI-001", "CRIT-CLI-002"}
	tcID := "TST-CLI-BATCH-001"
	testRoot, logPath := setupCLIBatchScenario(t, "cli_batch_runner.sh", scriptTmpl, tcID, critIDs)
	_ = testRoot

	var buf bytes.Buffer
	cmd := NewRunCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{"--batch=true", tcID})

	err := cmd.Execute()
	require.NoError(t, err)

	outStr := buf.String()
	assert.Contains(t, outStr, "[PASS]")
	assert.Contains(t, outStr, "2/2 passed")

	// Verify only 1 batch invocation was logged
	data, err := fileutil.ReadFile(logPath)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%s %s\n", critIDs[0], critIDs[1]), string(data))
}

func TestBatchCriteriaExecution_CLI_FallbackOnFailure(t *testing.T) {
	scriptTmpl := `#!/usr/bin/env bash
if [ "$#" -gt 1 ]; then
    exit 1
fi
if [ "$1" = "CRIT-FB-001" ]; then
    exit 0
fi
exit 2
`
	critIDs := []string{"CRIT-FB-001", "CRIT-FB-002"}
	tcID := "TST-CLI-FALLBACK-001"
	setupCLIBatchScenario(t, "cli_fallback_runner.sh", scriptTmpl, tcID, critIDs)

	var buf bytes.Buffer
	cmd := NewRunCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{"--batch=true", tcID})

	err := cmd.Execute()
	require.Error(t, err)

	outStr := buf.String()
	assert.Contains(t, outStr, "Batch execution failed; fell back to isolated single-criterion execution")
	assert.Contains(t, outStr, "[PASS] CRIT-FB-001")
	assert.Contains(t, outStr, "[FAIL] CRIT-FB-002")
	assert.Contains(t, outStr, "Criteria: 1/2 passed")
}

func setupCLIBatchScenario(
	t *testing.T,
	scriptName, scriptContent, tcID string,
	critIDs []string,
) (string, string) {
	t.Helper()
	testRoot, sp := setupCycle140TestProject(t)
	stdctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	scriptRel := filepath.Join("scripts", scriptName)
	scriptAbs := filepath.Join(testRoot, scriptRel)
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(scriptAbs), 0755))

	logPath := filepath.Join(testRoot, scriptName+".log")
	body := scriptContent
	if strings.Contains(scriptContent, "%s") {
		body = fmt.Sprintf(scriptContent, logPath)
	}
	require.NoError(t, fileutil.WriteFile(scriptAbs, []byte(body), 0755))

	for _, cid := range critIDs {
		require.NoError(t, sp.Create(stdctx, secCtx, map[string]any{
			objects.FieldKeyID:          cid,
			objects.FieldKeyKind:        objects.KindCriteria,
			objects.FieldKeyTitle:       cid,
			objects.FieldKeyDescription: cid + " Description",
			objects.FieldKeyCategory:    "functional",
			objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
		}))
	}

	require.NoError(t, sp.Create(stdctx, secCtx, map[string]any{
		objects.FieldKeyID:           tcID,
		objects.FieldKeyKind:         objects.KindTestCase,
		objects.FieldKeyTitle:        tcID + " Title",
		objects.FieldKeyDescription:  tcID + " Description",
		objects.FieldKeyStatus:       objects.ObjectStatusActive,
		objects.FieldKeyPathOrID:     scriptRel,
		objects.FieldKeyCriteriaRefs: critIDs,
	}))

	return testRoot, logPath
}
