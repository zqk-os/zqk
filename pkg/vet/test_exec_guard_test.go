package vet

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestProhibitNakedExecCommand verifies CRIT-TST-SUBPROCESS-STATIC-GUARD / TST-TST-SUBPROCESS-STATIC-GUARD
func TestProhibitNakedExecCommand(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "sample")
	require.NoError(t, fileutil.MkdirAll(pkgDir, 0o755))

	// 1. Write an unmanaged test file that calls exec.Command directly
	unmanagedTest := `package sample

import (
	"os/exec"
	"testing"
)

func TestUnmanaged(t *testing.T) {
	cmd := exec.Command("sleep", "10")
	_ = cmd.Run()
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "unmanaged_test.go"), []byte(unmanagedTest), 0o644))

	// 2. Write a managed test file that uses ManagedCommand
	managedTest := `package sample

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestManaged(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := testkit.ManagedCommand(t, ctx, "sleep", "1")
	_ = cmd.Run()
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "managed_test.go"), []byte(managedTest), 0o644))

	// 3. Non-test file with exec.Command (should NOT be flagged by test subprocess guard)
	prodSrc := `package sample

import (
	"os/exec"
)

func Helper() {
	_ = exec.Command("true")
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "sample.go"), []byte(prodSrc), 0o644))

	cfg := &GatesConfig{
		Hygiene: HygieneConfig{
			GoScanDirs: []string{"pkg"},
		},
	}

	findings, err := CheckTestSubprocessHygiene(tempDir, cfg)
	require.NoError(t, err)
	require.Len(t, findings, 1)

	for _, f := range findings {
		assert.Equal(t, "hygiene/unmanaged_test_exec", f.CheckID)
		assert.Equal(t, SeverityError, f.Severity)
		assert.Equal(t, "hygiene", f.Suite)
	}
}

func TestCheckTestSubprocessHygiene_Boundaries(t *testing.T) {
	t.Run("NonExistentDirectory", func(t *testing.T) {
		cfg := &GatesConfig{
			Hygiene: HygieneConfig{
				GoScanDirs: []string{"non_existent_dir_xyz"},
			},
		}
		findings, err := CheckTestSubprocessHygiene(t.TempDir(), cfg)
		require.NoError(t, err)
		assert.Empty(t, findings)
	})

	t.Run("SyntaxErrorIgnoredGracefully", func(t *testing.T) {
		tempDir := t.TempDir()
		pkgDir := filepath.Join(tempDir, "pkg", "sample")
		require.NoError(t, fileutil.MkdirAll(pkgDir, 0o755))
		badGo := `package sample broken syntax {`
		require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "bad_test.go"), []byte(badGo), 0o644))

		cfg := &GatesConfig{
			Hygiene: HygieneConfig{
				GoScanDirs: []string{"pkg"},
			},
		}
		findings, err := CheckTestSubprocessHygiene(tempDir, cfg)
		require.NoError(t, err)
		assert.Empty(t, findings)
	})

	t.Run("StartProcessDetected", func(t *testing.T) {
		tempDir := t.TempDir()
		pkgDir := filepath.Join(tempDir, "pkg", "sample")
		require.NoError(t, fileutil.MkdirAll(pkgDir, 0o755))
		startProcessSrc := `package sample

import (
	"os"
	"testing"
)

func TestStart(t *testing.T) {
	_, _ = os.StartProcess("/bin/ls", nil, nil)
}
`
		require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "start_test.go"), []byte(startProcessSrc), 0o644))

		cfg := &GatesConfig{
			Hygiene: HygieneConfig{
				GoScanDirs: []string{"pkg"},
			},
		}
		findings, err := CheckTestSubprocessHygiene(tempDir, cfg)
		require.NoError(t, err)
		require.Len(t, findings, 1)
		assert.Equal(t, "hygiene/unmanaged_test_exec", findings[0].CheckID)
		assert.Equal(t, SeverityError, findings[0].Severity)
	})

	t.Run("CommandContextDetected", func(t *testing.T) {
		tempDir := t.TempDir()
		pkgDir := filepath.Join(tempDir, "pkg", "sample")
		require.NoError(t, fileutil.MkdirAll(pkgDir, 0o755))
		cmdContextSrc := `package sample

import (
	"context"
	"os/exec"
	"testing"
)

func TestCmdContext(t *testing.T) {
	_ = exec.CommandContext(context.Background(), "/bin/ls")
}
`
		require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "cmd_context_test.go"), []byte(cmdContextSrc), 0o644))

		cfg := &GatesConfig{
			Hygiene: HygieneConfig{
				GoScanDirs: []string{"pkg"},
			},
		}
		findings, err := CheckTestSubprocessHygiene(tempDir, cfg)
		require.NoError(t, err)
		require.Len(t, findings, 1)
		assert.Equal(t, "hygiene/unmanaged_test_exec", findings[0].CheckID)
		assert.Equal(t, SeverityError, findings[0].Severity)
		assert.Contains(t, findings[0].Message, "CommandContext")
	})
}

func TestCheckTestFileutilHygiene(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "sample")
	require.NoError(t, fileutil.MkdirAll(pkgDir, 0o755))

	rawFileutilTest := `package sample

import (
	"os"
	"testing"
)

func TestRawFiles(t *testing.T) {
	_, _ = ` + "os." + `ReadFile("test.txt")
	_ = ` + "os." + `WriteFile("test.txt", []byte("data"), 0o644)
	_, _ = os.Open("test.txt")
	_ = ` + "os." + `RemoveAll("test.txt")
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "raw_test.go"), []byte(rawFileutilTest), 0o644))

	cleanFileutilTest := `package sample

import (
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCleanFiles(t *testing.T) {
	_, _ = fileutil.ReadFile("test.txt")
	_ = fileutil.WriteFile("test.txt", []byte("data"), 0o644)
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "clean_test.go"), []byte(cleanFileutilTest), 0o644))

	cfg := &GatesConfig{
		Hygiene: HygieneConfig{
			GoScanDirs: []string{"pkg"},
		},
	}

	findings, err := CheckTestFileutilHygiene(tempDir, cfg)
	require.NoError(t, err)
	require.Len(t, findings, 4)

	for _, f := range findings {
		assert.Equal(t, "hygiene/unmanaged_test_fileutil", f.CheckID)
		assert.Equal(t, SeverityError, f.Severity)
		assert.Equal(t, "hygiene", f.Suite)
	}
}
