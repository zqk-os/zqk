package zqksession

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/config"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// mockCycle246Storage provides controlled responses for coverage testing.
type mockCycle246Storage struct {
	storage.NoopObjectStorage
	mu            sync.Mutex
	created       []map[string]any
	createErr     error
	createErrOnce bool
	createFunc    func(ctx context.Context, sec *storage.SecurityContext, obj map[string]any) error
	readMap       map[string]map[string]any
	readErr       error
	updateMap     map[string]map[string]any
	updateCalls   int
}

func (m *mockCycle246Storage) Create(ctx context.Context, sec *storage.SecurityContext, obj map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createFunc != nil {
		return m.createFunc(ctx, sec, obj)
	}
	if m.createErr != nil {
		if m.createErrOnce {
			err := m.createErr
			m.createErr = nil
			return err
		}
		return m.createErr
	}
	m.created = append(m.created, obj)
	return nil
}

func (m *mockCycle246Storage) Read(_ context.Context, _ *storage.SecurityContext, id string) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.readErr != nil {
		return nil, m.readErr
	}
	if m.readMap != nil {
		if obj, ok := m.readMap[id]; ok {
			return obj, nil
		}
	}
	return nil, errors.New("object not found")
}

func (m *mockCycle246Storage) Update(_ context.Context, _ *storage.SecurityContext, id string, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls++
	if m.updateMap == nil {
		m.updateMap = make(map[string]map[string]any)
	}
	m.updateMap[id] = updates
	return nil
}

func setupCycle246TestEnv(t *testing.T) (string, *mockCycle246Storage) {
	t.Helper()
	t.Parallel()
	return t.TempDir(), &mockCycle246Storage{}
}

func createBlockedStateDir(t *testing.T, tmpDir string) {
	t.Helper()
	blockedPath := sessionStateDir(tmpDir)
	if err := fileutil.MkdirAll(filepath.Dir(blockedPath), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(blockedPath, []byte("blocking-file"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
}

func writeTestSessionConfig(t *testing.T, tmpDir, cfgContent string) {
	t.Helper()
	cfgDir := filepath.Join(tmpDir, paths.ConfigDir)
	if err := fileutil.MkdirAll(cfgDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(cfgDir, paths.ZqkConfigFileName), []byte(cfgContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
}

// TestCycle246_ContextCoverage tests all branches in context.go.
func TestCycle246_ContextCoverage(t *testing.T) {
	t.Parallel()

	t.Run("NilContextHandling", func(t *testing.T) {
		t.Parallel()
		if id := GetIDFromContext(nil); id != EmptyValue {
			t.Fatalf("GetIDFromContext(nil) = %q, want empty", id)
		}

		ctx := WithID(nil, "ZQK-CTX-1")
		if ctx == nil {
			t.Fatal("WithID(nil, ...) returned nil context")
		}
		if got := GetIDFromContext(ctx); got != "ZQK-CTX-1" {
			t.Fatalf("GetIDFromContext() = %q, want ZQK-CTX-1", got)
		}
	})

	t.Run("NonStringValueInContext", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), sessionIDKey{}, 42)
		if got := GetIDFromContext(ctx); got != EmptyValue {
			t.Fatalf("GetIDFromContext() with non-string = %q, want empty", got)
		}
	})

	t.Run("ContextWithoutSessionKey", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), "other-key", "val")
		if got := GetIDFromContext(ctx); got != EmptyValue {
			t.Fatalf("GetIDFromContext() without session key = %q, want empty", got)
		}
	})
}

// TestCycle246_IDCoverage tests all branches in id.go.
func TestCycle246_IDCoverage(t *testing.T) {
	t.Run("LooksLikeID_EdgeCases", func(t *testing.T) {
		tests := []struct {
			input string
			want  bool
		}{
			{"", false},
			{"   ", false},
			{"|persona", false},
			{"   |   ", false},
			{"ZQK-12345|persona", true},
			{"  ZQK-99999|seat  ", true},
			{"ZQK-ABC", true},
			{CanonicalIDPrefix + "123", true},
			{"INVALID-123", false},
			{"ZQK", false},
		}

		for _, tc := range tests {
			got := LooksLikeID(tc.input)
			if got != tc.want {
				t.Errorf("LooksLikeID(%q) = %v, want %v", tc.input, got, tc.want)
			}
		}
	})

	t.Run("IDPrefixes_CustomConfigBranches", func(t *testing.T) {
		// Set up a temporary project root with custom prefixes to hit:
		// 1. prefix == "" (trimmed empty string)
		// 2. !strings.HasSuffix(prefix, hyphen) (prefix without trailing hyphen)
		tmpRoot := t.TempDir()
		cfgDir := filepath.Join(tmpRoot, paths.ProcessInternalConfigsDir)
		if err := fileutil.MkdirAll(cfgDir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create config dir: %v", err)
		}

		// Create marker so findPathByWalkingUp recognizes tmpRoot
		if err := fileutil.WriteFile(filepath.Join(tmpRoot, "go.mod"), []byte("module testroot\n"), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write go.mod marker: %v", err)
		}

		customConfigContent := `kind_to_prefixes:
  zqk_session:
    - "   "
    - "CUSTOM_NO_DASH"
    - "CUSTOM-WITH-DASH-"
`
		if err := fileutil.WriteFile(filepath.Join(cfgDir, paths.IdPrefixesConfigFile), []byte(customConfigContent), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write custom id_prefixes config: %v", err)
		}

		// Reset configs and change working directory to test root
		validation.ResetGlobalIDPrefixesConfig()
		validation.ResetGlobalPathsConfig()

		oldWd, err := fileutil.Getwd()
		if err != nil {
			t.Fatalf("failed to get wd: %v", err)
		}
		defer func() {
			if chdirErr := fileutil.Chdir(oldWd); chdirErr != nil {
				t.Logf("failed to restore wd: %v", chdirErr)
			}
			validation.ResetGlobalIDPrefixesConfig()
			validation.ResetGlobalPathsConfig()
		}()

		if err := fileutil.Chdir(tmpRoot); err != nil {
			t.Fatalf("failed to chdir: %v", err)
		}
		validation.ResetGlobalIDPrefixesConfig()
		validation.ResetGlobalPathsConfig()

		prefixes := IDPrefixes()
		hasNoDashNormalized := false
		hasDash := false
		for _, p := range prefixes {
			if p == "CUSTOM_NO_DASH-" {
				hasNoDashNormalized = true
			}
			if p == "CUSTOM-WITH-DASH-" {
				hasDash = true
			}
		}

		if !hasNoDashNormalized {
			t.Errorf("expected CUSTOM_NO_DASH- to be present in prefixes, got: %v", prefixes)
		}
		if !hasDash {
			t.Errorf("expected CUSTOM-WITH-DASH- to be present in prefixes, got: %v", prefixes)
		}
	})
}

// TestCycle246_SessionSecurityContext tests authorization branches in sessionStorageSecCtx.
func TestCycle246_SessionSecurityContext(t *testing.T) {
	t.Parallel()

	t.Run("NilSecurityContextInContext", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		sec := sessionStorageSecCtx(ctx)
		if sec == nil {
			t.Fatal("expected non-nil system security context")
		}
		if !pkgctx.IsSystemAccount(sec.AccountID) {
			t.Errorf("expected system account ID, got %q", sec.AccountID)
		}
	})

	t.Run("SecurityContextWithWritePermission", func(t *testing.T) {
		t.Parallel()
		userSec := &pkgctx.SecurityContext{
			AccountID:   "ACC-USER-1",
			Roles:       []string{"developer"},
			Permissions: []string{fmt.Sprintf("write:%s", objects.KindZqkSession)},
		}
		ctx := pkgctx.WithSecurityContext(context.Background(), userSec)
		sec := sessionStorageSecCtx(ctx)
		if sec != userSec {
			t.Fatalf("expected original security context to be retained")
		}
	})

	t.Run("SecurityContextWithoutWritePermission", func(t *testing.T) {
		t.Parallel()
		userSec := &pkgctx.SecurityContext{
			AccountID:   "ACC-READONLY",
			Roles:       []string{"viewer"},
			Permissions: []string{"read:only"},
		}
		ctx := pkgctx.WithSecurityContext(context.Background(), userSec)
		sec := sessionStorageSecCtx(ctx)
		if sec == userSec {
			t.Fatal("expected system security context override for unauthorized caller")
		}
		if !pkgctx.IsSystemAccount(sec.AccountID) {
			t.Errorf("expected system account ID, got %q", sec.AccountID)
		}
	})
}

// TestCycle246_CreateSession_BoundaryAndErrors tests error pathways in createSession.
func TestCycle246_CreateSession_BoundaryAndErrors(t *testing.T) {
	t.Parallel()

	ctx := pkgctx.NewSystemContext()

	t.Run("EmptyProjectRoot", func(t *testing.T) {
		t.Parallel()
		store := &mockCycle246Storage{}
		id := StartCLISession(ctx, "", "title", "acc", store)
		if id != EmptyValue {
			t.Fatalf("StartCLISession with empty root = %q, want empty", id)
		}
	})

	t.Run("NilStorageProvider", func(t *testing.T) {
		t.Parallel()
		id := StartCLISession(ctx, t.TempDir(), "title", "acc", nil)
		if id != EmptyValue {
			t.Fatalf("StartCLISession with nil store = %q, want empty", id)
		}
	})

	t.Run("DefaultAccountIDResolution", func(t *testing.T) {
		tmpDir, store := setupCycle246TestEnv(t)
		id := StartCLISession(ctx, tmpDir, "default-acc-title", "", store)
		if id == EmptyValue {
			t.Fatal("expected successful session creation")
		}
		if len(store.created) != 1 {
			t.Fatalf("created = %d, want 1", len(store.created))
		}
		if store.created[0][objects.FieldKeyAccountID] != pkgctx.SystemAccountID {
			t.Errorf("accountID = %v, want system default", store.created[0][objects.FieldKeyAccountID])
		}
	})

	t.Run("DirectoryCreationFailure", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		// Place a regular file at .zqk so MkdirAll fails
		zqkFile := filepath.Join(tmpDir, paths.ProjectDataDir)
		if err := fileutil.WriteFile(zqkFile, []byte("blocker"), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write blocker file: %v", err)
		}

		store := &mockCycle246Storage{}
		id := StartCLISession(ctx, tmpDir, "blocked", "acc", store)
		if id != EmptyValue {
			t.Fatalf("StartCLISession with blocked dir = %q, want empty", id)
		}
	})

	t.Run("StorageCreateUnrecoverableError", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		wantErr := errors.New("storage disk full")
		store := &mockCycle246Storage{createErr: wantErr}
		id := StartCLISession(ctx, tmpDir, "unrecoverable", "acc", store)
		if id != EmptyValue {
			t.Fatalf("StartCLISession = %q, want empty on storage error", id)
		}
	})

	t.Run("MaxCreateAttemptsExceeded", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		// Always return "already exists" error to exceed maxCreateAttempts (10)
		store := &mockCycle246Storage{createErr: errors.New("object already exists in store")}
		id := StartCLISession(ctx, tmpDir, "exhausted", "acc", store)
		if id != EmptyValue {
			t.Fatalf("StartCLISession = %q, want empty when max attempts exhausted", id)
		}
	})
}

// TestCycle246_TouchAndThrottleCoverage tests Touch and TouchIfNotThrottled branches.
func TestCycle246_TouchAndThrottleCoverage(t *testing.T) {
	t.Parallel()

	ctx := pkgctx.NewSystemContext()

	t.Run("Touch_GuardConditions", func(t *testing.T) {
		tmpDir, store := setupCycle246TestEnv(t)

		// Nil checks and empty guards
		Touch(ctx, "", "sess-1", "title", "acc", store)
		Touch(ctx, tmpDir, "", "title", "acc", store)
		Touch(ctx, tmpDir, "sess-1", "title", "acc", nil)
		if store.updateCalls != 0 {
			t.Fatalf("unexpected update calls: %d", store.updateCalls)
		}

		// Touch with title and explicit account
		Touch(ctx, tmpDir, "sess-1", "new-title", "custom-acc", store)
		if store.updateCalls != 1 {
			t.Fatalf("updateCalls = %d, want 1", store.updateCalls)
		}
		updates := store.updateMap["sess-1"]
		if updates[objects.FieldKeyTitle] != "new-title" {
			t.Errorf("title = %v, want new-title", updates[objects.FieldKeyTitle])
		}
		if updates[objects.FieldKeyUpdatedBy] != "custom-acc" {
			t.Errorf("updatedBy = %v, want custom-acc", updates[objects.FieldKeyUpdatedBy])
		}

		// Touch without title and empty account (defaults to system)
		Touch(ctx, tmpDir, "sess-2", "", "", store)
		updates2 := store.updateMap["sess-2"]
		if _, ok := updates2[objects.FieldKeyTitle]; ok {
			t.Errorf("unexpected title in updates: %v", updates2)
		}
		if updates2[objects.FieldKeyUpdatedBy] != pkgctx.SystemAccountID {
			t.Errorf("updatedBy = %v, want system account", updates2[objects.FieldKeyUpdatedBy])
		}
	})

	t.Run("TouchIfNotThrottled_FileLockErrorFallback", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		createBlockedStateDir(t, tmpDir)

		store := &mockCycle246Storage{}
		// When NewFileLock fails, it falls back to direct unthrottled Touch
		TouchIfNotThrottled(ctx, tmpDir, "sess-fallback", "title", "acc", store)
		if store.updateCalls != 1 {
			t.Fatalf("expected fallback Touch call on file lock error, got %d calls", store.updateCalls)
		}
	})

	t.Run("TouchIfNotThrottled_EmptyGuardConditions", func(t *testing.T) {
		tmpDir, store := setupCycle246TestEnv(t)

		TouchIfNotThrottled(ctx, "", "sess-1", "t", "a", store)
		TouchIfNotThrottled(ctx, tmpDir, "", "t", "a", store)
		TouchIfNotThrottled(ctx, tmpDir, "sess-1", "t", "a", nil)
		if store.updateCalls != 0 {
			t.Fatalf("update calls on guards = %d, want 0", store.updateCalls)
		}
	})

	t.Run("TouchIfNotThrottled_ContentVariations", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		touchPath := lastSessionTouchPath(tmpDir)
		store := &mockCycle246Storage{}

		// 1. Touch file contains empty/whitespace string
		if err := fileutil.MkdirAll(filepath.Dir(touchPath), paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		if err := fileutil.WriteFile(touchPath, []byte("   \n"), paths.FilePerm600); err != nil {
			t.Fatal(err)
		}
		TouchIfNotThrottled(ctx, tmpDir, "sess-empty-touch", "t", "a", store)
		if store.updateCalls != 1 {
			t.Fatalf("updateCalls for empty touch file = %d, want 1", store.updateCalls)
		}

		// 2. Touch file contains unparseable corrupt timestamp
		if err := fileutil.WriteFile(touchPath, []byte("not-a-timestamp\n"), paths.FilePerm600); err != nil {
			t.Fatal(err)
		}
		TouchIfNotThrottled(ctx, tmpDir, "sess-corrupt-touch", "t", "a", store)
		if store.updateCalls != 2 {
			t.Fatalf("updateCalls for corrupt touch file = %d, want 2", store.updateCalls)
		}

		// 3. Touch file contains timestamp older than throttle window (e.g. 10s ago)
		oldTime := time.Now().Add(-10 * time.Second).UTC().Format(time.RFC3339)
		if err := fileutil.WriteFile(touchPath, []byte(oldTime+"\n"), paths.FilePerm600); err != nil {
			t.Fatal(err)
		}
		TouchIfNotThrottled(ctx, tmpDir, "sess-old-touch", "t", "a", store)
		if store.updateCalls != 3 {
			t.Fatalf("updateCalls for expired touch file = %d, want 3", store.updateCalls)
		}

		// 4. Touch file contains immediate recent timestamp -> should be throttled
		recentTime := time.Now().UTC().Format(time.RFC3339)
		if err := fileutil.WriteFile(touchPath, []byte(recentTime+"\n"), paths.FilePerm600); err != nil {
			t.Fatal(err)
		}
		TouchIfNotThrottled(ctx, tmpDir, "sess-throttled", "t", "a", store)
		if store.updateCalls != 3 {
			t.Fatalf("updateCalls for throttled call = %d, want 3 (no change)", store.updateCalls)
		}
	})
}

// TestCycle246_UnderLockFileOperations covers readSessionFileUnderLock, writeSessionFileUnderLock, clearSessionFileUnderLock.
func TestCycle246_UnderLockFileOperations(t *testing.T) {
	t.Parallel()

	t.Run("ReadSessionFileUnderLock_GuardsAndLockError", func(t *testing.T) {
		t.Parallel()
		if got := readSessionFileUnderLock(""); got != EmptyValue {
			t.Fatalf("readSessionFileUnderLock('') = %q, want empty", got)
		}

		tmpDir := t.TempDir()
		createBlockedStateDir(t, tmpDir)
		if got := readSessionFileUnderLock(tmpDir); got != EmptyValue {
			t.Fatalf("readSessionFileUnderLock with lock error = %q, want empty", got)
		}
	})

	t.Run("WriteSessionFileUnderLock_GuardsAndErrors", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		if ok := writeSessionFileUnderLock("", "sess-1"); ok {
			t.Fatal("writeSessionFileUnderLock with empty root should return false")
		}
		if ok := writeSessionFileUnderLock(tmpDir, ""); ok {
			t.Fatal("writeSessionFileUnderLock with empty session should return false")
		}

		// Block state dir creation
		blockDir := t.TempDir()
		createBlockedStateDir(t, blockDir)
		if ok := writeSessionFileUnderLock(blockDir, "sess-1"); ok {
			t.Fatal("writeSessionFileUnderLock with blocked directory should return false")
		}

		// Block lock file creation by creating session.lock as a directory
		lockDir := t.TempDir()
		stateDir := sessionStateDir(lockDir)
		if err := fileutil.MkdirAll(stateDir, paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		if err := fileutil.MkdirAll(filepath.Join(stateDir, "session.lock"), paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		if ok := writeSessionFileUnderLock(lockDir, "sess-1"); ok {
			t.Fatal("writeSessionFileUnderLock with directory lock should return false")
		}
	})

	t.Run("ClearSessionFileUnderLock_GuardsAndErrors", func(t *testing.T) {
		t.Parallel()
		// Guard check for empty project root
		clearSessionFileUnderLock("")

		// Guard check for lock failure
		tmpDir := t.TempDir()
		createBlockedStateDir(t, tmpDir)
		clearSessionFileUnderLock(tmpDir)
	})
}

// TestCycle246_IdleTimeoutCoverage tests readIdleTimeout variations and edge cases.
func TestCycle246_IdleTimeoutCoverage(t *testing.T) {
	t.Parallel()

	t.Run("NoConfigFileDefaults", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		d := GetIdleTimeout(tmpDir)
		if d != defaultIdleTimeout {
			t.Fatalf("GetIdleTimeout() = %v, want default %v", d, defaultIdleTimeout)
		}
	})

	t.Run("EmptyIdleTimeoutConfigValue", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		writeTestSessionConfig(t, tmpDir, "session:\n  idle_timeout: \"\"\n")
		d := GetIdleTimeout(tmpDir)
		if d != defaultIdleTimeout {
			t.Fatalf("GetIdleTimeout with empty timeout = %v, want default %v", d, defaultIdleTimeout)
		}
	})

	t.Run("NegativeOrZeroDurationFallback", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		writeTestSessionConfig(t, tmpDir, "session:\n  idle_timeout: \"-10s\"\n")
		d := GetIdleTimeout(tmpDir)
		if d != defaultIdleTimeout {
			t.Fatalf("GetIdleTimeout with negative timeout = %v, want default %v", d, defaultIdleTimeout)
		}
	})

	t.Run("NonMapSessionConfigFallback", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		writeTestSessionConfig(t, tmpDir, "session: \"not-a-map\"\n")
		d := GetIdleTimeout(tmpDir)
		if d != defaultIdleTimeout {
			t.Fatalf("GetIdleTimeout with non-map session = %v, want default %v", d, defaultIdleTimeout)
		}
	})
}

// TestCycle246_TryReuseAndPersistedIDEdgeCases tests all branches in TryReuse and WritePersistedID.
func TestCycle246_TryReuseAndPersistedIDEdgeCases(t *testing.T) {
	ctx := pkgctx.NewSystemContext()

	t.Run("GetCurrentID_EnvOverride", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(zqkenv.SessionID().Name(), "ZQK-ENV-SESSION-42")
		if got := GetCurrentID(tmpDir); got != "ZQK-ENV-SESSION-42" {
			t.Fatalf("GetCurrentID() with env = %q, want ZQK-ENV-SESSION-42", got)
		}
	})

	t.Run("TryReuse_EmptySessionPersisted", func(t *testing.T) {
		tmpDir, store := setupCycle246TestEnv(t)
		// No persisted ID in tmpDir
		sid, ok := TryReuse(ctx, tmpDir, "title", "acc", store)
		if ok || sid != EmptyValue {
			t.Fatalf("TryReuse with no persisted ID = (%q, %v), want ('', false)", sid, ok)
		}
	})

	t.Run("WritePersistedID_OverwriteSameID", func(t *testing.T) {
		tmpDir, store := setupCycle246TestEnv(t)
		sessID := "ZQK-SAME-1"

		if !WritePersistedID(ctx, tmpDir, sessID, "acc-1", store) {
			t.Fatal("first write failed")
		}
		// Overwrite with same session ID (hits existingID == sessionID branch)
		if !WritePersistedID(ctx, tmpDir, sessID, "acc-1", store) {
			t.Fatal("second write of same ID failed")
		}
		if ReadPersistedID(tmpDir) != sessID {
			t.Fatalf("persisted ID = %q, want %q", ReadPersistedID(tmpDir), sessID)
		}
	})

	t.Run("WritePersistedID_ReadErrorAllowsOverwrite", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		store := &mockCycle246Storage{
			readErr: errors.New("storage unreadable"),
		}
		oldID := "ZQK-OLD-1"
		newID := "ZQK-NEW-1"

		if !writeSessionFileUnderLock(tmpDir, oldID) {
			t.Fatal("failed to seed old session file")
		}
		// When store.Read returns error, it allows overwriting
		if !WritePersistedID(ctx, tmpDir, newID, "acc-1", store) {
			t.Fatal("WritePersistedID should allow overwrite when existing session read fails")
		}
		if ReadPersistedID(tmpDir) != newID {
			t.Fatalf("persisted ID = %q, want %q", ReadPersistedID(tmpDir), newID)
		}
	})

	t.Run("WritePersistedID_InactiveSessionAllowsOverwrite", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		oldID := "ZQK-COMPLETED-1"
		newID := "ZQK-NEW-2"
		store := &mockCycle246Storage{
			readMap: map[string]map[string]any{
				oldID: {
					objects.FieldKeyStatus:    StatusCompleted,
					objects.FieldKeyAccountID: "different-acc",
				},
			},
		}

		if !writeSessionFileUnderLock(tmpDir, oldID) {
			t.Fatal("failed to seed completed session file")
		}
		// Inactive session can be clobbered even by different account
		if !WritePersistedID(ctx, tmpDir, newID, "acc-1", store) {
			t.Fatal("WritePersistedID should allow overwrite when existing session is not active")
		}
		if ReadPersistedID(tmpDir) != newID {
			t.Fatalf("persisted ID = %q, want %q", ReadPersistedID(tmpDir), newID)
		}
	})

	t.Run("WritePersistedID_ActiveSessionEmptyOwnerAllowsOverwrite", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		oldID := "ZQK-NO-OWNER-1"
		newID := "ZQK-NEW-3"
		store := &mockCycle246Storage{
			readMap: map[string]map[string]any{
				oldID: {
					objects.FieldKeyStatus:    StatusActive,
					objects.FieldKeyAccountID: "",
				},
			},
		}

		if !writeSessionFileUnderLock(tmpDir, oldID) {
			t.Fatal("failed to seed session file")
		}
		// Owner empty allows overwrite
		if !WritePersistedID(ctx, tmpDir, newID, "acc-1", store) {
			t.Fatal("WritePersistedID should allow overwrite when existing owner is empty")
		}
		if ReadPersistedID(tmpDir) != newID {
			t.Fatalf("persisted ID = %q, want %q", ReadPersistedID(tmpDir), newID)
		}
	})

	t.Run("WritePersistedID_ActiveSessionSameOwnerAllowsOverwrite", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		oldID := "ZQK-SAME-OWNER-1"
		newID := "ZQK-NEW-4"
		store := &mockCycle246Storage{
			readMap: map[string]map[string]any{
				oldID: {
					objects.FieldKeyStatus:    StatusActive,
					objects.FieldKeyAccountID: "same-acc",
				},
			},
		}

		if !writeSessionFileUnderLock(tmpDir, oldID) {
			t.Fatal("failed to seed session file")
		}
		// Same owner allows overwrite
		if !WritePersistedID(ctx, tmpDir, newID, "same-acc", store) {
			t.Fatal("WritePersistedID should allow overwrite when account matches")
		}
		if ReadPersistedID(tmpDir) != newID {
			t.Fatalf("persisted ID = %q, want %q", ReadPersistedID(tmpDir), newID)
		}
	})
}

// TestCycle246_WorkerSession_BoundaryAndInheritance tests worker session edge cases.
func TestCycle246_WorkerSession_BoundaryAndInheritance(t *testing.T) {
	t.Parallel()

	ctx := pkgctx.NewSystemContext()

	t.Run("CustomExecutorTypePreserved", func(t *testing.T) {
		t.Parallel()
		store := &mockCycle246Storage{}
		input := WorkerSessionInput{
			ProjectRoot:  t.TempDir(),
			AgentID:      "agent-custom-seat",
			ExecutorType: "custom_worker_engine",
		}

		id, err := StartWorkerSession(ctx, input, store)
		if err != nil {
			t.Fatalf("StartWorkerSession: %v", err)
		}
		if id == EmptyValue {
			t.Fatal("expected valid worker session ID")
		}
		if len(store.created) != 1 {
			t.Fatalf("created = %d, want 1", len(store.created))
		}
		if store.created[0][objects.FieldKeyExecutorType] != "custom_worker_engine" {
			t.Errorf("executor_type = %v, want custom_worker_engine", store.created[0][objects.FieldKeyExecutorType])
		}
	})

	t.Run("ParentSessionRef_PersistableBranches", func(t *testing.T) {
		t.Parallel()
		store := &mockCycle246Storage{
			readMap: map[string]map[string]any{
				"ZQK-PARENT-READABLE": {objects.FieldKeyID: "ZQK-PARENT-READABLE"},
			},
		}

		// Empty parent
		if ref := persistableParentSessionRef(ctx, store, ""); ref != EmptyValue {
			t.Errorf("expected empty ref for empty parent, got %q", ref)
		}

		// Nil store
		if ref := persistableParentSessionRef(ctx, nil, "ZQK-PARENT-READABLE"); ref != EmptyValue {
			t.Errorf("expected empty ref for nil store, got %q", ref)
		}

		// Unreadable parent
		if ref := persistableParentSessionRef(ctx, store, "ZQK-PARENT-MISSING"); ref != EmptyValue {
			t.Errorf("expected empty ref for missing parent, got %q", ref)
		}

		// When stream storage is disabled, readable parent is returned
		cfg := config.Get()
		orig := cfg.System.StreamStorageEnabled
		disabled := false
		cfg.System.StreamStorageEnabled = &disabled
		defer func() {
			cfg.System.StreamStorageEnabled = orig
		}()

		// Readable parent with whitespace
		if ref := persistableParentSessionRef(ctx, store, "  ZQK-PARENT-READABLE  "); ref != "ZQK-PARENT-READABLE" {
			t.Errorf("expected trimmed parent ref, got %q", ref)
		}
	})
}

// TestCycle246_EndSession_Guards tests all guard clauses in End().
func TestCycle246_EndSession_Guards(t *testing.T) {
	t.Parallel()

	ctx := pkgctx.NewSystemContext()
	store := &mockCycle246Storage{}
	tmpDir := t.TempDir()

	End(ctx, "", "sess-1", StatusCompleted, "acc", store)
	End(ctx, tmpDir, "", StatusCompleted, "acc", store)
	End(ctx, tmpDir, "sess-1", "", "acc", store)
	End(ctx, tmpDir, "sess-1", StatusCompleted, "acc", nil)
	if store.updateCalls != 0 {
		t.Fatalf("expected 0 update calls on guards, got %d", store.updateCalls)
	}

	// Successful End with default system account
	End(ctx, tmpDir, "sess-1", StatusCompleted, "", store)
	if store.updateCalls != 1 {
		t.Fatalf("expected 1 update call, got %d", store.updateCalls)
	}
	updates := store.updateMap["sess-1"]
	if updates[objects.FieldKeyStatus] != StatusCompleted {
		t.Errorf("status = %v, want completed", updates[objects.FieldKeyStatus])
	}
	if updates[objects.FieldKeyUpdatedBy] != pkgctx.SystemAccountID {
		t.Errorf("updatedBy = %v, want system default", updates[objects.FieldKeyUpdatedBy])
	}
}

// TestCycle246_ConcurrentPersistedIDAndTouchStress tests concurrency and thread safety under heavy contention.
func TestCycle246_ConcurrentPersistedIDAndTouchStress(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := &mockCycle246Storage{
		readMap: make(map[string]map[string]any),
	}
	ctx := pkgctx.NewSystemContext()

	const workers = 16
	const iterations = 25
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		workerID := i
		wg.Add(1)
		goroutinelabels.NewGoroutine("cycle246_stress_worker", fmt.Sprintf("worker-%d", workerID)).StartSimple(func() {
			defer wg.Done()
			account := fmt.Sprintf("ACC-WORKER-%d", workerID)
			sessID := fmt.Sprintf("ZQK-SESS-%d", workerID)

			store.mu.Lock()
			store.readMap[sessID] = map[string]any{
				objects.FieldKeyStatus:    StatusActive,
				objects.FieldKeyAccountID: account,
				objects.FieldKeyUpdatedAt: zqktime.NowRFC3339UTC(),
			}
			store.mu.Unlock()

			for j := 0; j < iterations; j++ {
				WritePersistedID(ctx, tmpDir, sessID, account, store)
				ReadPersistedID(tmpDir)
				GetCurrentID(tmpDir)
				TouchIfNotThrottled(ctx, tmpDir, sessID, "stress-title", account, store)
				Touch(ctx, tmpDir, sessID, "touch-title", account, store)

				if j%10 == 0 {
					ClearPersisted(tmpDir)
				}
			}
		})
	}

	wg.Wait()
}
