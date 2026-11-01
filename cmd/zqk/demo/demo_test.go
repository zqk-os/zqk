package demo

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	demopkg "github.com/zqk-os/zqk/pkg/demo"
)

func TestDemoCmd_NonInteractive(t *testing.T) {
	cmd := NewDemoCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cmd.SetArgs([]string{"--non-interactive", "--speed=instant"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := outBuf.String()
	assert.Contains(t, output, "AUTONOMOUS ENGINEERING SHOWCASE")
	assert.Contains(t, output, demopkg.SceneNameAST)
	assert.Contains(t, output, demopkg.SceneNameVDS)
	assert.Contains(t, output, demopkg.SceneNameSwarm)
	assert.Contains(t, output, demopkg.SceneNameStar)
	assert.Contains(t, output, demopkg.GitHubRepoURL)
	assert.Contains(t, output, demopkg.GitHubForkURL)
}

func TestDemoCmd_StepSelection(t *testing.T) {
	cmd := NewDemoCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cmd.SetArgs([]string{"--step=ast", "--non-interactive", "--speed=instant"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := outBuf.String()
	assert.Contains(t, output, demopkg.SceneNameAST)
	assert.NotContains(t, output, demopkg.SceneNameVDS)
	assert.NotContains(t, output, demopkg.SceneNameSwarm)
}
