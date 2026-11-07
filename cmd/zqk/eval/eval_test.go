package eval_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/app"
	evalcmd "github.com/zqk-os/zqk/cmd/zqk/eval"
	cli "github.com/zqk-os/zqk/pkg/cliapp"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestEvalCmd_ThroughRoot(t *testing.T) {
	rootCmd := app.NewRootCommand()
	var outBuf bytes.Buffer
	rootCmd.SetOut(&outBuf)
	rootCmd.SetErr(&outBuf)
	rootCmd.SetArgs([]string{"eval", "validate", "--help"})
	err := rootCmd.Execute()
	require.NoError(t, err, "out: %s", outBuf.String())
}

func TestEvalCmd_Structure(t *testing.T) {
	cmd := evalcmd.NewEvalCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "eval", cmd.Name())

	subCommands := cmd.Commands()
	subNames := make(map[string]bool)
	for _, sub := range subCommands {
		subNames[sub.Name()] = true
	}

	assert.True(t, subNames["run"], "missing eval run")
	assert.True(t, subNames["validate"], "missing eval validate")
	assert.True(t, subNames["matrix"], "missing eval matrix")
	assert.True(t, subNames["llm-scrutiny"], "missing eval llm-scrutiny")
}

func TestEvalCmd_ValidateCommand(t *testing.T) {
	projectRoot := cli.ResolveProjectRoot(".")
	require.NotEmpty(t, projectRoot)

	templatePath := filepath.Join(projectRoot, ".zqk", "eval", "templates", "go-ast-hygiene.yaml")

	cmd := evalcmd.NewEvalCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cmd.SetArgs([]string{"validate", "--file", templatePath})

	err := cmd.Execute()
	require.NoError(t, err, "eval validate should succeed, out: %s", outBuf.String())
	assert.Contains(t, outBuf.String(), "is valid")
}

func TestEvalCmd_RunCommand(t *testing.T) {
	projectRoot := cli.ResolveProjectRoot(".")
	require.NotEmpty(t, projectRoot)

	templatePath := filepath.Join(projectRoot, ".zqk", "eval", "templates", "go-ast-hygiene.yaml")

	cmd := evalcmd.NewEvalCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cmd.SetArgs([]string{"run", "--template", templatePath, "--target", "pkg/eval", "--auto-mint=false"})

	err := cmd.Execute()
	require.NoError(t, err, "eval run should succeed, out: %s", outBuf.String())
	assert.Contains(t, outBuf.String(), "Scenario Evaluation")
	assert.Contains(t, outBuf.String(), "Composite Score")
}

func TestEvalCmd_RunCommand_JSON(t *testing.T) {
	projectRoot := cli.ResolveProjectRoot(".")
	require.NotEmpty(t, projectRoot)

	templatePath := filepath.Join(projectRoot, ".zqk", "eval", "templates", "go-ast-hygiene.yaml")

	cmd := evalcmd.NewEvalCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cmd.SetArgs([]string{"run", "--template", templatePath, "--target", "pkg/eval", "--auto-mint=false", "--json"})

	err := cmd.Execute()
	require.NoError(t, err, "eval run --json should succeed, out: %s", outBuf.String())
	assert.True(t, strings.HasPrefix(strings.TrimSpace(outBuf.String()), "{"))
}

func TestEvalCmd_LLMScrutiny(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "transcript.jsonl")
	logData := `
{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","content":"Verify status"}
{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-11-07T12:00:00Z","input_tokens":1000,"output_tokens":50,"tool_calls":[{"name":"run_command","args":{"CommandLine":"zqk status"}}]}
{"step_index":2,"source":"MODEL","type":"GENERIC","content":"Command exited with code 0."}
`
	require.NoError(t, fileutil.WriteFile(logFile, []byte(logData), 0644))

	t.Run("Table format", func(t *testing.T) {
		cmd := evalcmd.NewEvalCmd()
		cli.SetContext(cmd, cli.ContextForProjectRoot(tempDir))
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)
		cmd.SetArgs([]string{"llm-scrutiny", "--logs", logFile, "--format", "table"})

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "LLM INTERACTION SCRUTINY")
		assert.Contains(t, outBuf.String(), "RANK")
	})

	t.Run("JSON format", func(t *testing.T) {
		cmd := evalcmd.NewEvalCmd()
		cli.SetContext(cmd, cli.ContextForProjectRoot(tempDir))
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)
		cmd.SetArgs([]string{"llm-scrutiny", "--logs", logFile, "--format", "json"})

		err := cmd.Execute()
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(strings.TrimSpace(outBuf.String()), "{"))
		assert.Contains(t, outBuf.String(), "models_scrutinized")
	})

	t.Run("Markdown format", func(t *testing.T) {
		cmd := evalcmd.NewEvalCmd()
		cli.SetContext(cmd, cli.ContextForProjectRoot(tempDir))
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)
		cmd.SetArgs([]string{"llm-scrutiny", "--logs", logFile, "--format", "markdown"})

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "# LLM Interaction Scrutiny & Model Correctness Ranking")
	})
}
