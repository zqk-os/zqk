package cas_test

import (
	"github.com/lanceman/zqk/pkg/storage"
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	"bytes"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage/filecas"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestCASPendingVisibilityCache_CrossProcessReadYourWrites(t *testing.T) {
	tempDir := t.TempDir()

	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown() //nolint:errcheck // test cleanup

	// kindDir must be {root}/process/<kind> so pending projectRoot (Dir^3) == tempDir
	kindDir := filepath.Join(tempDir, paths.ProcessDir, "audit_events")
	cas := filecas.NewContentAddressableStorage(kindDir, "audit_event", casQueue)

	testID := "AUD-1785520000000000000-00000001"
	testContent := []byte(`{"id": "` + testID + `", "kind": "audit_event", "summary": "test pending visibility"}`)

	// Artificially bypass index persistence wait to simulate async lagging index
	filecas.SetSkipIndexUpdateWait(true)
	defer filecas.SetSkipIndexUpdateWait(false)

	// Create object in Process A
	if err := cas.Create(testID, testContent); err != nil {
		t.Fatalf("CAS Create failed: %v", err)
	}

	// 2. Verify pending visibility cache file exists and contains testID (CRIT-CAS-PENDING-002)
	pendingCache := caspkg.GetCASPendingVisibilityCache(tempDir)
	entry, found := pendingCache.LookupPending(testID)
	if !found {
		t.Fatalf("CRIT-CAS-PENDING-002 FAIL: pending visibility cache entry not found for %s", testID)
	}
	if entry.ObjectID != testID || entry.Kind != "audit_event" {
		t.Errorf("unexpected pending entry: %+v", entry)
	}

	// 3. Simulate Process B: Create a brand new CAS instance without pre-warmed in-memory index
	processB_CAS := filecas.NewContentAddressableStorage(kindDir, "audit_event", casQueue)

	// Cross-process GET without durable index merge or O(n) directory scan (CRIT-CAS-PENDING-001, CRIT-CAS-PENDING-004)
	hash, err := processB_CAS.GetHashForID(testID)
	if err != nil {
		t.Fatalf("CRIT-CAS-PENDING-001/004 FAIL: Process B GetHashForID failed for %s: %v", testID, err)
	}
	if hash != entry.Hash {
		t.Errorf("Process B hash mismatch: got %s, want %s", hash, entry.Hash)
	}

	// Read object content in Process B
	data, err := processB_CAS.Read(testID)
	if err != nil {
		t.Fatalf("Process B Read failed: %v", err)
	}
	if len(data) == 0 {
		t.Errorf("Process B Read returned empty data")
	}

	// 4. Eviction after SetMapping confirms the same id→hash (ADR §3)
	if err := cas.GetIndex().SetMapping(testID, entry.Hash); err != nil {
		t.Fatalf("SetMapping failed: %v", err)
	}
	if _, foundAfterEvict := pendingCache.LookupPending(testID); foundAfterEvict {
		t.Errorf("pending entry was not evicted after SetMapping confirmed durable index")
	}
}

// Stale pending must not shadow a newer durable mapping (kernel get→not-found race).
func TestCASPendingVisibility_StalePendingDoesNotShadowDurable(t *testing.T) {
	tempDir := t.TempDir()
	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown() //nolint:errcheck // test cleanup

	kindDir := filepath.Join(tempDir, paths.ProcessDir, "audit_events")
	cas := filecas.NewContentAddressableStorage(kindDir, "audit_event", casQueue)

	testID := "AUD-1785520000000000000-stale01"
	v1 := []byte(`{"id": "` + testID + `", "kind": "audit_event", "summary": "v1"}`)
	v2 := []byte(`{"id": "` + testID + `", "kind": "audit_event", "summary": "v2-updated"}`)

	if err := cas.Create(testID, v1); err != nil {
		t.Fatalf("Create: %v", err)
	}
	hash1, err := cas.GetHashForID(testID)
	if err != nil {
		t.Fatalf("GetHash after create: %v", err)
	}
	if err := cas.Update(testID, v2); err != nil {
		t.Fatalf("Update: %v", err)
	}
	hash2, err := cas.GetHashForID(testID)
	if err != nil {
		t.Fatalf("GetHash after update: %v", err)
	}
	if hash1 == hash2 {
		t.Fatalf("expected content hash to change on update")
	}

	pendingCache := caspkg.GetCASPendingVisibilityCache(tempDir)
	// Poison: advertise the superseded hash after durable index already has hash2.
	if err := pendingCache.PublishPending(testID, "audit_event", hash1, ""); err != nil {
		t.Fatalf("PublishPending poison: %v", err)
	}

	processB := filecas.NewContentAddressableStorage(kindDir, "audit_event", casQueue)
	got, err := processB.GetHashForID(testID)
	if err != nil {
		t.Fatalf("Process B GetHashForID: %v", err)
	}
	if got != hash2 {
		t.Fatalf("stale pending shadowed durable: got %s want durable %s (stale pending was %s)", got, hash2, hash1)
	}
	data, err := processB.Read(testID)
	if err != nil {
		t.Fatalf("Process B Read: %v", err)
	}
	// Read returns YAML-normalized bytes; assert via content hash of the on-disk blob path.
	if err := storage.VerifyContentHash(mustReadCASHashFile(t, kindDir, got), got); err != nil {
		t.Fatalf("Process B durable blob missing/corrupt for hash %s: %v (read_len=%d)", got, err, len(data))
	}
	if !bytes.Contains(data, []byte("v2-updated")) {
		t.Fatalf("Process B Read did not return updated summary; got %q", truncateForTest(data, 120))
	}
}

func mustReadCASHashFile(t *testing.T, kindDir, hash string) []byte {
	t.Helper()
	b, err := fileutil.ReadFile(filepath.Join(kindDir, hash+".yaml"))
	if err != nil {
		t.Fatalf("read hash file: %v", err)
	}
	return b
}

func truncateForTest(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

// TestConfirmPendingAfterDurableMappingRequiresIndexEntry ensures pending is kept when
// the durable index save omitted the id (async validate false-stale).
// TRACK: [REDACTED-ID]
func TestConfirmPendingAfterDurableMappingRequiresIndexEntry(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, paths.ProcessDir, "doc_entries", ".doc_entry.index")
	if err := fileutil.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	id := "DOC-1785520000000000000-pend01"
	hash := "abc123hash"
	pendingCache := caspkg.GetCASPendingVisibilityCache(tempDir)
	if err := pendingCache.PublishPending(id, "doc_entry", hash, ""); err != nil {
		t.Fatalf("PublishPending: %v", err)
	}

	// Index without the id — must not clear pending.
	if err := fileutil.WriteFile(indexPath, []byte(`{"`+objects.FieldKeyVersion+`":"1.0","`+objects.FieldKeyKind+`":"doc_entry","mappings":{}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	caspkg.ConfirmPendingAfterDurableMapping(indexPath, id, hash)
	if _, found := pendingCache.LookupPending(id); !found {
		t.Fatal("pending cleared despite missing durable mapping")
	}

	// Index with matching id→hash — may clear pending.
	body := `{"version":"1.0","kind":"doc_entry","mappings":{"` + id + `":"` + hash + `"}}` + "\n"
	if err := fileutil.WriteFile(indexPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	caspkg.ConfirmPendingAfterDurableMapping(indexPath, id, hash)
	if _, found := pendingCache.LookupPending(id); found {
		t.Fatal("expected pending cleared after durable confirm")
	}
}

func TestCASPendingVisibility_EvictPendingIfHashSkipsNewerPending(t *testing.T) {
	tempDir := t.TempDir()
	pendingCache := caspkg.GetCASPendingVisibilityCache(tempDir)
	id := "AUD-1785520000000000000-race02"
	if err := pendingCache.PublishPending(id, "audit_event", "hash-new", ""); err != nil {
		t.Fatalf("PublishPending: %v", err)
	}
	if err := pendingCache.EvictPendingIfHash(id, "hash-old"); err != nil {
		t.Fatalf("EvictPendingIfHash: %v", err)
	}
	entry, found := pendingCache.LookupPending(id)
	if !found || entry.Hash != "hash-new" {
		t.Fatalf("evicted newer pending; found=%v hash=%q", found, entry.Hash)
	}
	if err := pendingCache.EvictPendingIfHash(id, "hash-new"); err != nil {
		t.Fatalf("EvictPendingIfHash match: %v", err)
	}
	if _, found := pendingCache.LookupPending(id); found {
		t.Fatal("expected eviction when hashes match")
	}
}

func TestCASPendingVisibility_ShutdownOrderedDrain(t *testing.T) {
	tempDir := t.TempDir()

	kindDir := filepath.Join(tempDir, paths.ProcessDir, "audit_events")
	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	cas := filecas.NewContentAddressableStorage(kindDir, "audit_event", casQueue)

	testID := "AUD-1785520000000000000-00000002"
	wantHash := "fakehash123"

	pendingCache := caspkg.GetCASPendingVisibilityCache(tempDir)
	if err := pendingCache.PublishPending(testID, "audit_event", wantHash, ""); err != nil {
		t.Fatalf("PublishPending failed: %v", err)
	}

	// CRIT-CAS-PENDING-003: reject writes → dump pending → drain index queue
	if err := pendingCache.DumpPendingToDurableIndexes(func(kind string) (*filecas.ContentAddressableStorage, error) {
		if kind != "audit_event" {
			t.Fatalf("unexpected kind %q", kind)
		}
		return cas, nil
	}); err != nil {
		t.Fatalf("DumpPendingToDurableIndexes: %v", err)
	}

	if !pendingCache.WritesRejected() {
		t.Fatal("expected WritesRejected after ordered dump")
	}
	if err := pendingCache.PublishPending("AUD-should-fail", "audit_event", "x", ""); err == nil {
		t.Fatal("PublishPending should fail after BeginOrderedShutdown")
	}

	got, err := cas.GetIndex().GetHash(testID)
	if err != nil {
		t.Fatalf("durable index GetHash after dump: %v", err)
	}
	if got != wantHash {
		t.Fatalf("durable hash=%s want=%s", got, wantHash)
	}
	if _, found := pendingCache.LookupPending(testID); found {
		t.Fatal("pending entry should be evicted after successful dump")
	}

	_ = casQueue.Shutdown() //nolint:errcheck // drain WAL/index workers after dump
}

func TestCASPendingVisibility_UncleanShutdownWALReconcile(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Simulate process 1 crash / unclean shutdown with uncommitted pending visibility entry
	pendingCache := caspkg.GetCASPendingVisibilityCache(tempDir)
	testID := "AUD-1785520000000000000-00000005"
	wantHash := "unclean_hash_999"

	if err := pendingCache.PublishPending(testID, "audit_event", wantHash, ""); err != nil {
		t.Fatalf("PublishPending failed: %v", err)
	}

	// Verify entry is present in pending cache prior to restart/reconcile
	entry, found := pendingCache.LookupPending(testID)
	if !found {
		t.Fatalf("LookupPending failed prior to restart: expected entry for %s", testID)
	}
	if entry.Hash != wantHash {
		t.Fatalf("LookupPending hash mismatch: got %s, want %s", entry.Hash, wantHash)
	}

	// 2. Simulate process 2 restart: Reconcile WAL + pending visibility cache into durable indexes
	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(tempDir, "audit_event", casQueue)

	if err := pendingCache.DumpPendingToDurableIndexes(func(kind string) (*filecas.ContentAddressableStorage, error) {
		if kind == "audit_event" {
			return cas, nil
		}
		return nil, nil
	}); err != nil {
		t.Fatalf("Reconcile DumpPendingToDurableIndexes failed: %v", err)
	}

	// 3. Verify object ID is not silently ghosted and resolves to durable index mapping
	gotHash, err := cas.GetIndex().GetHash(testID)
	if err != nil {
		t.Fatalf("CRIT-CAS-PENDING-005 FAIL: GetHash failed after restart reconcile: %v", err)
	}
	if gotHash != wantHash {
		t.Fatalf("CRIT-CAS-PENDING-005 FAIL: durable index hash = %s, want %s", gotHash, wantHash)
	}

	// 4. Verify evicted from pending cache post-reconciliation
	if _, foundAfter := pendingCache.LookupPending(testID); foundAfter {
		t.Errorf("pending entry %s still present after successful reconciliation", testID)
	}
}
