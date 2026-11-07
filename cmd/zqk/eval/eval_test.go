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
