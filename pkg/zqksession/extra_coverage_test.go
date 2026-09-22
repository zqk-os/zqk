// BLI-STARTER-COMMUNITY-040 / PRI-STARTER-COMMUNITY-040 coverage elevation
package zqksession

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type extraSessionStore struct {
	storage.NoopObjectStorage
	created     []map[string]any
	createCalls int
	createErr   error
	retryOnce   bool
	readable    map[string]map[string]any
	updates     int
}

func (s *extraSessionStore) Create(_ context.Context, _ *storage.SecurityContext, obj map[string]any) error {
	s.createCalls++
	if s.retryOnce && s.createCalls == 1 {
		return errors.New("already exists")
	}
	if s.createErr != nil {
		return s.createErr
	}
	s.created = append(s.created, obj)
	return nil
}

func (s *extraSessionStore) Read(_ context.Context, _ *storage.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := s.readable[id]; ok {
		return obj, nil
	}
	return nil, errors.New("not found")
}

func (s *extraSessionStore) Update(_ context.Context, _ *storage.SecurityContext, _ string, _ map[string]any) error {
	s.updates++
	return nil
}

func TestExtraSessionLifecycleAndReuse(t *testing.T) {
	if LooksLikeID("") || LooksLikeID("   ") {
		t.Fatal("empty id")
	}
	_ = IDPrefixes()

	ctx := pkgctx.NewSystemContext()
	if StartCLISession(ctx, "", "t", "", nil) != EmptyValue {
		t.Fatal("empty root")
	}
	if StartCLISession(ctx, t.TempDir(), "t", "", nil) != EmptyValue {
		t.Fatal("nil storage")
	}

	root := t.TempDir()
	store := &extraSessionStore{retryOnce: true}
	id := StartCLISession(ctx, root, "title", EmptyValue, store)
	if id == EmptyValue || store.createCalls < 2 || len(store.created) != 1 {
		t.Fatalf("retry create = %q calls=%d created=%d", id, store.createCalls, len(store.created))
	}

	End(ctx, "", id, StatusCompleted, "", store)
	End(ctx, root, "", StatusCompleted, "", store)
	End(ctx, root, id, "", "", store)
	End(ctx, root, id, StatusCompleted, "", nil)
	End(ctx, root, id, StatusCompleted, EmptyValue, store)
	if store.updates == 0 {
		t.Fatal("end update")
	}

	Touch(ctx, "", id, "x", "", store)
	Touch(ctx, root, "", "x", "", store)
	Touch(ctx, root, id, "x", "", nil)
	Touch(ctx, root, id, "retitle", EmptyValue, store)
	TouchIfNotThrottled(ctx, "", id, "x", "", store)
	TouchIfNotThrottled(ctx, root, "", "x", "", store)
	TouchIfNotThrottled(ctx, root, id, "x", "", nil)
	TouchIfNotThrottled(ctx, root, id, "first", EmptyValue, store)
	TouchIfNotThrottled(ctx, root, id, "throttled", EmptyValue, store)

	if ReadPersistedID("") != EmptyValue {
		t.Fatal("empty persisted")
	}
	if !WritePersistedID(ctx, root, id, pkgctx.SystemAccountID, store) {
		t.Fatal("write persisted")
	}
	if ReadPersistedID(root) != id {
		t.Fatalf("read persisted = %q", ReadPersistedID(root))
	}
	ClearPersisted("")
	ClearPersisted(root)
	if ReadPersistedID(root) != EmptyValue {
		t.Fatal("cleared")
	}

	t.Setenv(zqkenv.SessionID().Name(), "ZS-from-env")
	if GetCurrentID(root) != "ZS-from-env" {
		t.Fatalf("env id = %q", GetCurrentID(root))
	}
	t.Setenv(zqkenv.SessionID().Name(), "")

	other := "ZS-other"
	store.readable = map[string]map[string]any{
		other: {
			objects.FieldKeyStatus:    StatusActive,
			objects.FieldKeyAccountID: "acct-other",
		},
	}
	if !WritePersistedID(ctx, root, other, "acct-other", store) {
		t.Fatal("write other")
	}
	if WritePersistedID(ctx, root, id, pkgctx.SystemAccountID, store) {
		t.Fatal("clobber other account")
	}
	if WritePersistedID(ctx, "", id, pkgctx.SystemAccountID, store) || WritePersistedID(ctx, root, "", pkgctx.SystemAccountID, store) || WritePersistedID(ctx, root, id, pkgctx.SystemAccountID, nil) {
		t.Fatal("write early")
	}

	cfgDir := filepath.Join(root, paths.ConfigDir)
	if err := fileutil.MkdirAll(cfgDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(cfgDir, paths.ZqkConfigFileName), []byte("session:\n  idle_timeout: 1ns\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if GetIdleTimeout(root) != time.Nanosecond {
		t.Fatalf("idle = %v", GetIdleTimeout(root))
	}

	if _, ok := TryReuse(ctx, "", "t", "", store); ok {
		t.Fatal("reuse empty")
	}
	if _, ok := TryReuse(ctx, root, "t", "", nil); ok {
		t.Fatal("reuse nil")
	}

	expiredID := "ZS-expired"
	expRoot := t.TempDir()
	store.readable[expiredID] = map[string]any{
		objects.FieldKeyStatus:    StatusActive,
		objects.FieldKeyAccountID: pkgctx.SystemAccountID,
		objects.FieldKeyUpdatedAt: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
	}
	if !WritePersistedID(ctx, expRoot, expiredID, pkgctx.SystemAccountID, store) {
		t.Fatal("write expired")
	}
	cfgDir2 := filepath.Join(expRoot, paths.ConfigDir)
	if err := fileutil.MkdirAll(cfgDir2, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(cfgDir2, paths.ZqkConfigFileName), []byte("session:\n  idle_timeout: 1ns\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if sid, ok := TryReuse(ctx, expRoot, "t", EmptyValue, store); ok || sid != EmptyValue {
		t.Fatalf("expired reuse = %q %v", sid, ok)
	}

	inactive := "ZS-inactive"
	inRoot := t.TempDir()
	store.readable[inactive] = map[string]any{
		objects.FieldKeyStatus:    StatusCompleted,
		objects.FieldKeyAccountID: pkgctx.SystemAccountID,
	}
	if !WritePersistedID(ctx, inRoot, inactive, pkgctx.SystemAccountID, store) {
		t.Fatal("write inactive")
	}
	if _, ok := TryReuse(ctx, inRoot, "t", pkgctx.SystemAccountID, store); ok {
		t.Fatal("inactive reuse")
	}

	ownerRoot := t.TempDir()
	ownerID := "ZS-owner"
	store.readable[ownerID] = map[string]any{
		objects.FieldKeyStatus:    StatusActive,
		objects.FieldKeyAccountID: "someone-else",
	}
	if !WritePersistedID(ctx, ownerRoot, ownerID, "someone-else", store) {
		t.Fatal("write owner")
	}
	if _, ok := TryReuse(ctx, ownerRoot, "t", pkgctx.SystemAccountID, store); ok {
		t.Fatal("other owner reuse")
	}

	missingRoot := t.TempDir()
	if !WritePersistedID(ctx, missingRoot, "ZS-missing", pkgctx.SystemAccountID, store) {
		t.Fatal("write missing")
	}
	if _, ok := TryReuse(ctx, missingRoot, "t", pkgctx.SystemAccountID, store); ok {
		t.Fatal("missing reuse")
	}

	badRoot := t.TempDir()
	badCfg := filepath.Join(badRoot, paths.ConfigDir)
	if err := fileutil.MkdirAll(badCfg, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(badCfg, paths.ZqkConfigFileName), []byte("{not yaml"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if GetIdleTimeout(badRoot) != defaultIdleTimeout {
		t.Fatalf("bad yaml idle = %v", GetIdleTimeout(badRoot))
	}
}
