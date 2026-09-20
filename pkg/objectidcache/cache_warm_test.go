package objectidcache

import (
	stdcontext "context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestCacheWarm_NoOpNotifier(t *testing.T) {
	if noOpCacheProgressNotifier == nil {
		t.Fatal("expected non-nil noOpCacheProgressNotifier")
	}
	noOpCacheProgressNotifier.NotifyCacheProgress("kind", "step")
}

func TestWarmCASIndexesFromCache_ExcludesDraftPlane(t *testing.T) {
	testRoot, fos, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	cache := NewObjectIDCache()

	draftID := "DOC-warm-draft-001"
	draftPath := storage.ObjectDraftPlanePath(testRoot, objects.KindDocEntry, draftID)
	cache.Set(draftID, &ObjectIDCacheEntry{
		ID:       draftID,
		Kind:     objects.KindDocEntry,
		FilePath: draftPath,
		MTime:    time.Now(),
		Exists:   true,
	})

	warmCASIndexesFromCache(stdcontext.Background(), testRoot, cache, fos, 100*time.Millisecond)

	cas, err := fos.GetContentAddressableStorageForTest(objects.KindDocEntry)
	if err == nil && cas != nil {
		if hash, err := cas.GetIndex().GetHash(draftID); err == nil && hash != "" {
			t.Fatalf("CAS index should NOT contain draft ID %s, got %s", draftID, hash)
		}
	}
}
