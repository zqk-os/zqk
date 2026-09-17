package system

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func mustProcessRoot(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Join(tmp, paths.ProcessDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir process: %v", err)
	}
	return tmp
}

// Regression: system check StorageProvider is Batching(Routing(...)). Integrity used to
// type-assert only *FileObjectStorage, miss the cache, and NewContentAddressableStorage
// (full index reload) per object — doc_entry (~2k, ~170KB index) blew the 5s validation budget.
// TRACK: BLI-1785895580100186000-c5539372
func TestExtractFileStorage_UnwrapsBatchingRouting(t *testing.T) {
	t.Parallel()
	tmp := mustProcessRoot(t)
	fs, err := storage.NewFileObjectStorageForTest(tmp)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmp, fs)

	// Mirror StorageProviderCache shape: Batching → Routing tip is optional; Batching alone
	// was enough to break a bare *FileObjectStorage assert. Also wrap Routing when factory-shaped.
	batched := storage.NewBatchingObjectStorage(fs)
	if extractFileStorage(batched) != fs {
		t.Fatal("extractFileStorage must unwrap BatchingObjectStorage")
	}

	ctx := pkgctx.NewSystemContext()
	factory, err := storage.NewStorageFactory(ctx, tmp)
	if err != nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	t.Cleanup(func() {
		_ = factory.GetStorage().Shutdown(ctx)
	})
	factoryShaped := storage.NewBatchingObjectStorage(storage.NewRoutingObjectStorage(factory))
	if extractFileStorage(factoryShaped) == nil {
		t.Fatal("extractFileStorage must unwrap Batching(Routing(...))")
	}
}

func TestCheckIntegrity_ReusesCASUnderBatchedProvider(t *testing.T) {
	t.Parallel()
	tmp := mustProcessRoot(t)

	kind := objects.KindDocEntry
	dirName := objects.GetDirectoryFromKind(kind)
	kindDir := filepath.Join(tmp, paths.ProcessDir, dirName)
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	fs, err := storage.NewFileObjectStorageForTest(tmp)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmp, fs)
	provider := storage.NewBatchingObjectStorage(fs)

	cas, err := fs.GetContentAddressableStorage(kind)
	if err != nil || cas == nil {
		t.Fatalf("GetContentAddressableStorage: %v", err)
	}

	const n = 48
	pathsList := make([]string, n)
	contents := make([][]byte, n)
	mappings := make(map[string]string, n)
	for i := 0; i < n; i++ {
		id := "DOC-REUSE-" + strconv.Itoa(i)
		body := []byte("id: " + id + "\nkind: doc_entry\ntitle: t\nstatus: active\ncontent_searchable: true\n")
		sum := sha256.Sum256(body)
		hash := hex.EncodeToString(sum[:])
		p := testkit.WriteTestObjectStandalone(t, tmp, string(body))
		mappings[id] = hash
		pathsList[i] = p
		contents[i] = body
	}
	if err := cas.GetIndex().SetMappings(mappings, nil); err != nil {
		t.Fatalf("SetMappings: %v", err)
	}

	cas2, err := fs.GetContentAddressableStorage(kind)
	if err != nil || cas2 == nil {
		t.Fatalf("GetContentAddressableStorage 2: %v", err)
	}
	if cas != cas2 {
		t.Fatal("expected cached CAS instance reuse on FileObjectStorage")
	}

	checkCtx := cli.ContextForProjectAndProfile(tmp, "system")
	var wg sync.WaitGroup
	errs := make(chan error, n)
	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			obj, err := parser.NewYAMLParser().ParseBytes(contents[i])
			if err != nil {
				errs <- err
				return
			}
			_, _ = checkIntegrityWithRegistryAndContent(checkCtx, obj, pathsList[i], kind, contents[i], nil, provider)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("parse/integrity: %v", err)
		}
	}
	elapsed := time.Since(start)
	if elapsed > 3*time.Second {
		t.Fatalf("concurrent integrity with batched provider took %v; want <3s (CAS reuse)", elapsed)
	}
	t.Logf("concurrent integrity n=%d elapsed=%v", n, elapsed)
}

func TestObjectIDCache_EntriesForID(t *testing.T) {
	t.Parallel()
	cache := NewObjectIDCache()
	now := time.Now()
	cache.Set("DOC-1", &ObjectIDCacheEntry{ID: "DOC-1", Kind: objects.KindDocEntry, FilePath: "/tmp/a/1.yaml", MTime: now})
	cache.Set("DOC-2", &ObjectIDCacheEntry{ID: "DOC-2", Kind: objects.KindDocEntry, FilePath: "/tmp/a/2.yaml", MTime: now})
	got := cache.EntriesForID("DOC-1")
	if len(got) != 1 || got[0].ID != "DOC-1" {
		t.Fatalf("EntriesForID: got %#v", got)
	}
	if len(cache.EntriesForID("DOC-MISSING")) != 0 {
		t.Fatal("missing id should return empty")
	}
}
