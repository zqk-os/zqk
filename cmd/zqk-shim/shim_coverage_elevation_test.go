package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestShimLogger(t *testing.T) {
	logger := shimLogger()
	require.NotNil(t, logger, "shimLogger must return a non-nil system logger")
}

func TestInjectPRBodyStamp(t *testing.T) {
	stamp := "abcd1234ef5678"

	t.Run("appends to existing --body argument", func(t *testing.T) {
		args := []string{"pr", "create", "--title", "Feature", "--body", "Initial PR description"}
		res := injectPRBodyStamp(args, stamp)
		require.Len(t, res, 6)
		assert.Equal(t, "--body", res[4])
		assert.Contains(t, res[5], "Initial PR description")
		assert.Contains(t, res[5], "[ZQK_AGENT_STAMP: abcd1234ef5678]")
	})

	t.Run("appends to existing -b argument", func(t *testing.T) {
		args := []string{"pr", "create", "-b", "Short flag body"}
		res := injectPRBodyStamp(args, stamp)
		require.Len(t, res, 4)
		assert.Equal(t, "-b", res[2])
		assert.Contains(t, res[3], "Short flag body")
		assert.Contains(t, res[3], "[ZQK_AGENT_STAMP: abcd1234ef5678]")
	})

	t.Run("appends --body when no body argument present", func(t *testing.T) {
		args := []string{"pr", "create", "--title", "Only title"}
		res := injectPRBodyStamp(args, stamp)
		require.Len(t, res, 6)
		assert.Equal(t, "--body", res[4])
		assert.Equal(t, "[ZQK_AGENT_STAMP: abcd1234ef5678]", res[5])
	})

	t.Run("trailing body flag without value handles gracefully", func(t *testing.T) {
		args := []string{"pr", "create", "--body"}
		res := injectPRBodyStamp(args, stamp)
		// Should fall through and append --body <stamp>
		assert.Contains(t, res, "[ZQK_AGENT_STAMP: abcd1234ef5678]")
	})
}

func TestHasBreakGlassOverride_Fallbacks(t *testing.T) {
	t.Run("nil getenv defaults to os.Getenv without panic", func(t *testing.T) {
		ok, reason := hasBreakGlassOverride(nil)
		assert.False(t, ok)
		assert.Empty(t, reason)
	})

	t.Run("resolves brand key fallback", func(t *testing.T) {
		brandKey := zqkenv.DefaultBrandKey("BREAK_GLASS_REASON")
		validReason := "emergency operational hotfix approved by security lead"
		env := map[string]string{
			brandKey: validReason,
		}
		ok, reason := hasBreakGlassOverride(func(k string) string { return env[k] })
		assert.True(t, ok)
		assert.Equal(t, validReason, reason)
	})

	t.Run("resolves plain BREAK_GLASS_REASON fallback", func(t *testing.T) {
		validReason := "emergency operational hotfix approved by security lead"
		env := map[string]string{
			"BREAK_GLASS_REASON": validReason,
		}
		ok, reason := hasBreakGlassOverride(func(k string) string { return env[k] })
		assert.True(t, ok)
		assert.Equal(t, validReason, reason)
	})

	t.Run("whitespace is trimmed and checked against length", func(t *testing.T) {
		env := map[string]string{
			"BREAK_GLASS_REASON": "   too short   ",
		}
		ok, reason := hasBreakGlassOverride(func(k string) string { return env[k] })
		assert.False(t, ok)
		assert.Equal(t, "too short", reason)
	})
}

func TestShimTraceabilityBypassRequested_Variants(t *testing.T) {
	tests := []struct {
		key   string
		value string
		want  bool
	}{
		{zqkenv.ZqkShimBypassTraceability().Name(), "1", true},
		{zqkenv.ZqkShimBypassTraceability().Name(), "0", false},
		{zqkenv.DefaultBrandKey("SHIM_BYPASS_TRACEABILITY"), "1", true},
		{"GIT_" + zqkenv.ZqkShimBypassTraceability().Name(), "1", true},
		{"GH_" + zqkenv.ZqkShimBypassTraceability().Name(), "1", true},
		{"GIT_" + zqkenv.DefaultBrandKey("SHIM_BYPASS_TRACEABILITY"), "1", true},
		{"GH_" + zqkenv.DefaultBrandKey("SHIM_BYPASS_TRACEABILITY"), "1", true},
		{"OTHER_KEY", "1", false},
	}

	for _, tt := range tests {
		env := map[string]string{tt.key: tt.value}
		got := shimTraceabilityBypassRequested(func(k string) string { return env[k] })
		assert.Equal(t, tt.want, got, "key: %s, value: %s", tt.key, tt.value)
	}
}

func TestFindRealBinary(t *testing.T) {
	tempDir := t.TempDir()

	// Create fake executable binary
	fakeGit := filepath.Join(tempDir, "fake-git")
	err := fileutil.WriteFile(fakeGit, []byte("#!/bin/sh\necho fake\n"), 0755)
	require.NoError(t, err)

	// Create fake non-executable file
	fakeNoExec := filepath.Join(tempDir, "fake-noexec")
	err = fileutil.WriteFile(fakeNoExec, []byte("data"), 0644)
	require.NoError(t, err)

	// Create subfolder with binary name
	fakeDir := filepath.Join(tempDir, "fake-dir")
	err = fileutil.MkdirAll(fakeDir, 0755)
	require.NoError(t, err)

	// Setup PATH with tempDir
	origPath := os.Getenv("PATH")
	t.Cleanup(func() {
		_ = os.Setenv("PATH", origPath)
	})
	_ = os.Setenv("PATH", tempDir+string(fileutil.PathListSeparator)+origPath)

	t.Run("locates executable binary on PATH", func(t *testing.T) {
		realPath := findRealBinary("fake-git")
		assert.Equal(t, fakeGit, realPath)
	})

	t.Run("skips non-executable file", func(t *testing.T) {
		realPath := findRealBinary("fake-noexec")
		assert.Empty(t, realPath)
	})

	t.Run("skips directory matching binary name", func(t *testing.T) {
		realPath := findRealBinary("fake-dir")
		assert.Empty(t, realPath)
	})

	t.Run("returns empty string for nonexistent binary", func(t *testing.T) {
		realPath := findRealBinary("completely-nonexistent-executable-binary-12345")
		assert.Empty(t, realPath)
	})
}

func TestGenerateCryptographicStampWithEnv_Scenarios(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("falls back to SHA256 hash when no private key exists", func(t *testing.T) {
		stamp := generateCryptographicStampWithEnv(tempDir, func(k string) string {
			if k == zqkenv.AgentID().Name() {
				return "test-agent-fallback"
			}
			return ""
		})
		require.NotEmpty(t, stamp)
		// Fallback hash is 64 hex characters (sha256)
		assert.Len(t, stamp, 64)
	})

	t.Run("falls back to anonymous-agent when agent ID is empty", func(t *testing.T) {
		stamp := generateCryptographicStampWithEnv(tempDir, func(k string) string {
			return ""
		})
		require.NotEmpty(t, stamp)
		assert.Len(t, stamp, 64)
	})

	t.Run("handles empty project root by running git rev-parse", func(t *testing.T) {
		stamp := generateCryptographicStampWithEnv("", func(k string) string {
			return ""
		})
		require.NotEmpty(t, stamp)
		assert.Len(t, stamp, 64)
	})

	t.Run("falls back to hash when private key is corrupt", func(t *testing.T) {
		agentID := "corrupt-key-agent"
		// Write invalid non-hex private key to seat credential
		credDir := filepath.Join(tempDir, ".zqk", "security", "credentials")
		require.NoError(t, fileutil.MkdirAll(credDir, 0700))
		credFile := filepath.Join(credDir, agentID+".cred")
		require.NoError(t, fileutil.WriteFile(credFile, []byte("invalid-non-hex-seed"), 0600))

		stamp := generateCryptographicStampWithEnv(tempDir, func(k string) string {
			if k == zqkenv.AgentID().Name() {
				return agentID
			}
			return ""
		})
		require.NotEmpty(t, stamp)
		assert.Len(t, stamp, 64)
	})
}

func TestValidateCommitTraceabilityWithEnv_WorkstreamScenarios(t *testing.T) {
	t.Run("empty projectRoot skips gate", func(t *testing.T) {
		err := validateCommitTraceabilityWithEnv("", os.Getenv)
		assert.NoError(t, err)
	})

	t.Run("non-git directory fails to get git branch", func(t *testing.T) {
		tempDir := t.TempDir()
		err := validateCommitTraceabilityWithEnv(tempDir, os.Getenv)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get current git branch")
	})

	t.Run("isolated git repo with active workstream matching branch", func(t *testing.T) {
		proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
			SeedSchemaPlane: true,
		})
		tempDir := proj.Root

		// Init git repo
		cmd := execwrap.Command("git", "init", "-b", "main")
		cmd.Dir = tempDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git init: %s", string(out))

		// Configure git user
		cmdName := execwrap.Command("git", "config", "user.name", "Test Agent")
		cmdName.Dir = tempDir
		_ = cmdName.Run()
		cmdEmail := execwrap.Command("git", "config", "user.email", "agent@test.internal")
		cmdEmail.Dir = tempDir
		_ = cmdEmail.Run()

		// Create initial commit
		readme := filepath.Join(tempDir, "README.md")
		require.NoError(t, fileutil.WriteFile(readme, []byte("# Test Repo\n"), 0644))
		cmdAdd := execwrap.Command("git", "add", "README.md")
		cmdAdd.Dir = tempDir
		require.NoError(t, cmdAdd.Run())
		cmdCommit := execwrap.Command("git", "commit", "-m", "initial commit")
		cmdCommit.Dir = tempDir
		require.NoError(t, cmdCommit.Run())

		// Checkout branch named after a workstream
		wsID := "WS-TEST-CYCLE-84"
		cmdCheckout := execwrap.Command("git", "checkout", "-b", "integration/"+strings.ToLower(wsID))
		cmdCheckout.Dir = tempDir
		require.NoError(t, cmdCheckout.Run())

		// Setup storage in tempDir
		ctx := pkgctx.NewSystemContext()
		secCtx := pkgctx.NewSystemSecurityContext()
		p, err := storage.GetGlobalStorageProviderCache().GetOrCreate(ctx, tempDir)
		require.NoError(t, err)

		// Create owner account in storage
		accID := "ACC-1785920548450214012-68b850c0"
		accObj := map[string]interface{}{
			objects.FieldKeyID:          accID,
			objects.FieldKeyKind:        objects.KindAccount,
			objects.FieldKeyStatus:      "active",
			objects.FieldKeyTitle:       "System Account",
			objects.FieldKeyNamespaceID: "zqk:kernel",
			"username":                  "system",
			"roles":                     []string{"admin"},
			"schema_version":            "2.0.0",
			"version_context":           "default",
		}
		storage.CreateCASVisible(t, p, ctx, secCtx, accObj, "active")

		// Create active workstream object in storage and promote it
		wsObj := map[string]interface{}{
			objects.FieldKeyID:          wsID,
			objects.FieldKeyKind:        objects.KindWorkstream,
			objects.FieldKeyStatus:      "active",
			objects.FieldKeyTitle:       "Test Workstream Cycle 84",
			objects.FieldKeyDescription: "Test workstream description for Cycle 84",
			objects.FieldKeyNamespaceID: "zqk:kernel",
			"entry_point":               "packs/test/entry.yaml",
			"owner_ref":                 accID,
			"category":                  "feature",
			"schema_version":            "2.0.0",
			"version_context":           "default",
		}
		storage.CreateCASVisible(t, p, ctx, secCtx, wsObj, "active")

		// Gate should pass because branch contains wsID
		err = validateCommitTraceabilityWithEnv(tempDir, func(k string) string { return "" })
		assert.NoError(t, err, "expected traceability validation to pass when branch matches workstream ID")

		// Checkout branch that does NOT match workstream
		cmdOtherBranch := execwrap.Command("git", "checkout", "-b", "untracked-rogue-branch")
		cmdOtherBranch.Dir = tempDir
		require.NoError(t, cmdOtherBranch.Run())

		err = validateCommitTraceabilityWithEnv(tempDir, func(k string) string { return "" })
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not linked to any active workstream")

		// Change workstream status to non-active ("completed")
		require.NoError(t, p.Update(ctx, secCtx, wsID, map[string]any{objects.FieldKeyStatus: "completed"}))

		// Checkout back to workstream branch; since workstream is not active, it should fail
		cmdBack := execwrap.Command("git", "checkout", "integration/"+strings.ToLower(wsID))
		cmdBack.Dir = tempDir
		require.NoError(t, cmdBack.Run())

		err = validateCommitTraceabilityWithEnv(tempDir, func(k string) string { return "" })
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not linked to any active workstream")
	})
}

func TestResolveTrustedAgentPrivateKey_EdgeCases(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("empty agentID and empty projectRoot with no ambient key returns error", func(t *testing.T) {
		_, err := resolveTrustedAgentPrivateKey("", "", func(k string) string { return "" })
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no trusted private key found")
	})

	t.Run("ambient key with brand key BREAK_GLASS_REASON is accepted", func(t *testing.T) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		privHex := hex.EncodeToString(priv)
		brandReasonKey := zqkenv.DefaultBrandKey("BREAK_GLASS_REASON")

		env := map[string]string{
			zqkenv.AgentPrivateKey().Name(): privHex,
			brandReasonKey:                  "justification for break glass operational emergency override",
		}
		key, err := resolveTrustedAgentPrivateKey(tempDir, "any-agent", func(k string) string { return env[k] })
		require.NoError(t, err)
		assert.Equal(t, privHex, key)
	})

	t.Run("ambient key with plain BREAK_GLASS_REASON is accepted", func(t *testing.T) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		privHex := hex.EncodeToString(priv)

		env := map[string]string{
			zqkenv.AgentPrivateKey().Name(): privHex,
			"BREAK_GLASS_REASON":            "justification for break glass operational emergency override",
		}
		key, err := resolveTrustedAgentPrivateKey(tempDir, "any-agent", func(k string) string { return env[k] })
		require.NoError(t, err)
		assert.Equal(t, privHex, key)
	})
}

func TestDirectWrappers(t *testing.T) {
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))

	// In a non-project directory, validateCommitTraceability returns nil immediately
	assert.NoError(t, validateCommitTraceability())

	// generateCryptographicStamp returns a fallback hash stamp in a non-project directory
	stamp := generateCryptographicStamp()
	assert.NotEmpty(t, stamp)
}

func TestRunShim_Comprehensive(t *testing.T) {
	t.Run("empty rawArgs returns 1", func(t *testing.T) {
		code := runShim(nil, nil, nil, nil)
		assert.Equal(t, 1, code)
	})

	t.Run("nonexistent binary on PATH returns 1", func(t *testing.T) {
		var errBuf bytes.Buffer
		code := runShim([]string{"nonexistent-executable-binary-xyz-9876"}, nil, nil, &errBuf)
		assert.Equal(t, 1, code)
	})

	t.Run("mutating git commit fails when traceability violated", func(t *testing.T) {
		// Mock an untracked/non-workstream branch
		proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
			SeedSchemaPlane: true,
		})
		origDir, err := os.Getwd()
		require.NoError(t, err)
		defer func() { _ = os.Chdir(origDir) }()
		require.NoError(t, os.Chdir(proj.Root))

		// Init git repo on non-workstream branch
		cmd := execwrap.Command("git", "init", "-b", "rogue-branch")
		cmd.Dir = proj.Root
		_ = cmd.Run()

		var errBuf bytes.Buffer
		code := runShim([]string{"git", "commit", "-m", "violating commit"}, func(k string) string { return "" }, nil, &errBuf)
		assert.Equal(t, 1, code)
	})

	t.Run("non-mutating git command succeeds with stdout capture", func(t *testing.T) {
		var outBuf, errBuf bytes.Buffer
		code := runShim([]string{"git", "version"}, nil, &outBuf, &errBuf)
		assert.Equal(t, 0, code)
		assert.Contains(t, outBuf.String(), "git version")
	})

	t.Run("break-glass bypass emits telemetry and executes", func(t *testing.T) {
		var outBuf, errBuf bytes.Buffer
		env := map[string]string{
			zqkenv.ZqkShimBypassTraceability().Name(): "1",
			zqkenv.BreakGlassReason().Name():          "authorizing emergency bypass during automated unit testing execution",
		}
		code := runShim([]string{"git", "version"}, func(k string) string { return env[k] }, &outBuf, &errBuf)
		assert.Equal(t, 0, code)
		assert.Contains(t, errBuf.String(), "cli_exec_start\n")
		assert.Contains(t, errBuf.String(), "cli_exec_success\n")
	})

	t.Run("gh pr create with mock script on PATH exercises cryptographic stamp injection", func(t *testing.T) {
		// Create a temporary mock 'gh' executable script in a temp PATH directory
		tempBinDir := t.TempDir()
		mockGh := filepath.Join(tempBinDir, "gh")
		mockGhScript := "#!/bin/sh\necho \"gh mocked: $@\"\nexit 0\n"
		require.NoError(t, fileutil.WriteFile(mockGh, []byte(mockGhScript), 0755))

		// Prepend tempBinDir to PATH
		origPath := os.Getenv("PATH")
		t.Setenv("PATH", tempBinDir+string(fileutil.PathListSeparator)+origPath)

		var outBuf, errBuf bytes.Buffer
		env := map[string]string{
			zqkenv.ZqkShimBypassTraceability().Name(): "1",
			zqkenv.BreakGlassReason().Name():          "authorizing emergency bypass during automated unit testing execution",
		}
		code := runShim([]string{"gh", "pr", "create", "--title", "Feature", "--body", "Initial PR description"}, func(k string) string { return env[k] }, &outBuf, &errBuf)
		assert.Equal(t, 0, code)
		assert.Contains(t, outBuf.String(), "[ZQK_AGENT_STAMP:")
	})

	t.Run("command execution failure returns 1", func(t *testing.T) {
		// Mock an executable script that exits 2
		tempBinDir := t.TempDir()
		mockFail := filepath.Join(tempBinDir, "mockfail")
		mockFailScript := "#!/bin/sh\nexit 2\n"
		require.NoError(t, fileutil.WriteFile(mockFail, []byte(mockFailScript), 0755))

		origPath := os.Getenv("PATH")
		t.Setenv("PATH", tempBinDir+string(fileutil.PathListSeparator)+origPath)

		var outBuf, errBuf bytes.Buffer
		code := runShim([]string{"mockfail"}, nil, &outBuf, &errBuf)
		assert.Equal(t, 1, code)
	})
}
