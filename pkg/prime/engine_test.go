package prime

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestPrimeBrownfieldRepo(t *testing.T) {
	tempDir := t.TempDir()

	// Create sample Go files in the temp directory
	sampleFile1 := filepath.Join(tempDir, "service.go")
	code1 := `package sample

// Greeter defines greeting behaviors.
type Greeter interface {
	Greet(name string) string
}

// User represents a customer entity.
type User struct {
	ID   string
	Name string
}

const DefaultGreeting = "Hello"

var ActiveUserCount = 0

// NewUser creates a new user instance.
func NewUser(id, name string) *User {
	return &User{ID: id, Name: name}
}

// Greet returns a greeting for the user.
func (u *User) Greet(name string) string {
	return DefaultGreeting + ", " + name
}
`
	require.NoError(t, fileutil.WriteFile(sampleFile1, []byte(code1), paths.FilePerm644))

	sampleFile2 := filepath.Join(tempDir, "pkg", "worker.go")
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(sampleFile2), paths.DirPerm755))
	code2 := `package pkg

type Worker struct {
	Active bool
}

func (w *Worker) Start() error {
	w.Active = true
	return nil
}
`
	require.NoError(t, fileutil.WriteFile(sampleFile2, []byte(code2), paths.FilePerm644))

	engine := NewEngine(tempDir)
	opts := PrimeOptions{
		ProjectRoot: tempDir,
		Verify:      true,
		Format:      FormatText,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := engine.Prime(ctx, opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.Success)
	assert.True(t, result.TargetDurationMet, "Priming must complete in <5s")
	assert.True(t, result.OrphanedStateVerified)
	assert.Equal(t, 2, result.FilesScanned)

	// Verify symbol counts:
	// File 1: 1 interface (Greeter), 1 struct (User), 1 const (DefaultGreeting), 1 var (ActiveUserCount), 1 func (NewUser), 1 method (Greet)
	// File 2: 1 struct (Worker), 1 method (Start)
	// Totals: Structs=2, Interfaces=1, Functions=1, Methods=2, Constants=1, Variables=1
	assert.Equal(t, 2, result.SymbolStats.Structs, "expected 2 structs")
	assert.Equal(t, 1, result.SymbolStats.Interfaces, "expected 1 interface")
	assert.Equal(t, 1, result.SymbolStats.Functions, "expected 1 function")
	assert.Equal(t, 2, result.SymbolStats.Methods, "expected 2 methods")
	assert.Equal(t, 1, result.SymbolStats.Constants, "expected 1 constant")
	assert.Equal(t, 1, result.SymbolStats.Variables, "expected 1 variable")
	assert.Equal(t, 8, result.SymbolStats.Total, "expected 8 total symbols")

	// Verify cache files are inside .zqk/cache/
	cacheDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.CacheDir)
	astCachePath := filepath.Join(cacheDir, ASTSymbolsCacheFile)
	assert.True(t, fileutil.Exists(astCachePath), "ast_symbols.json must exist")

	trigramPath := filepath.Join(cacheDir, TrigramIndexCacheFile)
	assert.True(t, fileutil.Exists(trigramPath), "trigram.idx must exist")

	manifestPath := filepath.Join(cacheDir, PrimeManifestFile)
	assert.True(t, fileutil.Exists(manifestPath), "prime_manifest.json must exist")

	// Verify ast_symbols.json content
	astBytes, err := fileutil.ReadFile(astCachePath)
	require.NoError(t, err)
	var loadedData ASTIndexData
	require.NoError(t, json.Unmarshal(astBytes, &loadedData))
	assert.Equal(t, 2, loadedData.Stats.Structs)
	assert.Len(t, loadedData.Symbols, 8)

	// Invariant: no lock file left behind
	lockPath := filepath.Join(tempDir, paths.ProjectDataDir, LockDir, PrimeLockFile)
	assert.False(t, fileutil.Exists(lockPath), "prime.lock must be cleaned up")
}

func TestNegativeBoundary_ZeroGitModification(t *testing.T) {
	tempDir := t.TempDir()

	// Initialize real git repo
	cmd := testkit.ManagedCommand(t, t.Context(), "git", "init")
	cmd.Dir = tempDir
	require.NoError(t, cmd.Run())

	cmd = testkit.ManagedCommand(t, t.Context(), "git", "config", "user.name", "ZQK Agent")
	cmd.Dir = tempDir
	require.NoError(t, cmd.Run())

	cmd = testkit.ManagedCommand(t, t.Context(), "git", "config", "user.email", "agent@zqk.dev")
	cmd.Dir = tempDir
	require.NoError(t, cmd.Run())

	// Add tracked file and commit
	trackedFile := filepath.Join(tempDir, "main.go")
	code := `package main
func main() {}
`
	require.NoError(t, fileutil.WriteFile(trackedFile, []byte(code), paths.FilePerm644))

	cmd = testkit.ManagedCommand(t, t.Context(), "git", "add", "main.go")
	cmd.Dir = tempDir
	require.NoError(t, cmd.Run())

	cmd = testkit.ManagedCommand(t, t.Context(), "git", "commit", "-m", "initial commit")
	cmd.Dir = tempDir
	require.NoError(t, cmd.Run())

	// Run Prime with verification
	engine := NewEngine(tempDir)
	result, err := engine.Prime(context.Background(), PrimeOptions{
		ProjectRoot: tempDir,
		Verify:      true,
	})
	require.NoError(t, err)
	assert.True(t, result.GitUnmodifiedVerified)

	// Check git status directly: zero modifications to tracked files
	cmd = testkit.ManagedCommand(t, t.Context(), "git", "status", "--porcelain")
	cmd.Dir = tempDir
	out, err := cmd.Output()
	require.NoError(t, err)

	// Tracked files must have no status (only .zqk untracked if not gitignored)
	for _, line := range filepath.SplitList(string(out)) {
		assert.NotContains(t, line, "M main.go")
		assert.NotContains(t, line, "D main.go")
	}
}

func TestNegativeBoundary_OrphanedStateViolation(t *testing.T) {
	tempDir := t.TempDir()

	// Place an illegal lockfile outside .zqk/
	rogueLock := filepath.Join(tempDir, "stale.lock")
	require.NoError(t, fileutil.WriteFile(rogueLock, []byte("stale"), paths.FilePerm644))

	err := verifyNoOrphanedStateOutsideZqk(tempDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "negative boundary violation")
}

func TestPrime_ContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	engine := NewEngine(tempDir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := engine.Prime(ctx, PrimeOptions{ProjectRoot: tempDir})
	assert.ErrorIs(t, err, context.Canceled)
}
