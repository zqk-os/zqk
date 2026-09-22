// BLI-STARTER-COMMUNITY-043 / PRI-STARTER-COMMUNITY-043 coverage elevation
package sync

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestExtraSyncNowBranchesAndHeartbeat(t *testing.T) {
	ctx := context.Background()
	empty := NewHiveMindSynchronizer(&mockMemoryStore{}, &mockIndexerWorker{}, &mockObjectLoader{})
	if err := empty.SyncNow(ctx); err != nil {
		t.Fatal(err)
	}

	store := &mockMemoryStore{missingIDs: []string{"OBJ-1", "OBJ-2", "OBJ-3"}}
	worker := &mockIndexerWorker{indexErr: errors.New("index")}
	loader := &mockObjectLoader{loadErr: errors.New("load")}
	if err := NewHiveMindSynchronizer(store, worker, loader).SyncNow(ctx); err != nil {
		t.Fatal(err)
	}

	store = &mockMemoryStore{missingIDs: []string{"OBJ-1"}, linkErr: errors.New("link")}
	worker = &mockIndexerWorker{}
	loader = &mockObjectLoader{}
	if err := NewHiveMindSynchronizer(store, worker, loader).SyncNow(ctx); err != nil {
		t.Fatal(err)
	}

	store = &mockMemoryStore{missingIDs: []string{"OBJ-1"}}
	worker = &mockIndexerWorker{indexErr: errors.New("index")}
	if err := NewHiveMindSynchronizer(store, worker, loader).SyncNow(ctx); err != nil {
		t.Fatal(err)
	}

	s := NewHiveMindSynchronizer(&mockMemoryStore{missingIDs: []string{"OBJ-1"}}, &mockIndexerWorker{}, &mockObjectLoader{})
	s.Start(ctx, 15*time.Millisecond)
	time.Sleep(40 * time.Millisecond)
	s.Stop()
	empty.Stop()
}
