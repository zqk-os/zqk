package prime

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestPrimeCmd_Execution(t *testing.T) {
	tempDir := t.TempDir()

	// Write Go source file
	sourceFile := filepath.Join(tempDir, "sample.go")
	code := `package sample

type Config struct {
	Port int
}

func NewConfig() *Config {
	return &Config{Port: 8080}
}
`
	require.NoError(t, fileutil.WriteFile(sourceFile, []byte(code), paths.FilePerm644))

	cmd := NewPrimeCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cmd.SetArgs([]string{tempDir, "--verify=false"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := outBuf.String()
	assert.Contains(t, output, "ZQK Brownfield Fast-Prime Engine")
	assert.Contains(t, output, "Files Scanned:      1")
	assert.Contains(t, output, "AST Symbols:        2")
	assert.Contains(t, output, "Repository primed!")
}

func TestPrimeCmd_JSONFormat(t *testing.T) {
	tempDir := t.TempDir()

	sourceFile := filepath.Join(tempDir, "app.go")
	code := `package main
func main() {}
`
	require.NoError(t, fileutil.WriteFile(sourceFile, []byte(code), paths.FilePerm644))

	cmd := NewPrimeCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cmd.SetArgs([]string{tempDir, "-f", "json", "--verify=false"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := outBuf.String()
	assert.Contains(t, output, `"success": true`)
	assert.Contains(t, output, `"target_duration_met": true`)
	assert.Contains(t, output, `"project_root"`)
}

func TestPrimeCmd_QuietAndForce(t *testing.T) {
	tempDir := t.TempDir()

	sourceFile := filepath.Join(tempDir, "sample.go")
	code := `package sample
func Hello() string { return "world" }
`
	require.NoError(t, fileutil.WriteFile(sourceFile, []byte(code), paths.FilePerm644))

	cmd := NewPrimeCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cmd.SetArgs([]string{tempDir, "--quiet", "--force", "--verify=false"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Empty(t, outBuf.String())
}

func TestPrimeCmd_PathFlag(t *testing.T) {
	tempDir := t.TempDir()

	sourceFile := filepath.Join(tempDir, "sample.go")
	code := `package sample
func Hello() string { return "world" }
`
	require.NoError(t, fileutil.WriteFile(sourceFile, []byte(code), paths.FilePerm644))

	cmd := NewPrimeCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cmd.SetArgs([]string{"--path", tempDir, "--verify=false"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "ZQK Brownfield Fast-Prime Engine")
}

func TestPrimeCmd_DefaultDir(t *testing.T) {
	tempDir := t.TempDir()
	origDir, err := fileutil.Getwd()
	require.NoError(t, err)
	defer func() { _ = fileutil.Chdir(origDir) }()

	require.NoError(t, fileutil.Chdir(tempDir))

	cmd := NewPrimeCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cmd.SetArgs([]string{"--verify=false"})

	err = cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "ZQK Brownfield Fast-Prime Engine")
}

func TestPrimeCmd_ErrorOnInvalidDir(t *testing.T) {
	cmd := NewPrimeCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cmd.SetArgs([]string{"/nonexistent/path/xyz/123", "--verify=false"})

	err := cmd.Execute()
	assert.Error(t, err)
}
