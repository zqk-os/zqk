// BLI-STARTER-COMMUNITY-038 / PRI-STARTER-COMMUNITY-038 coverage elevation
package breeding

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/events"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type breedStore struct {
	storage.NoopObjectStorage
	lists     []*storage.QueryResult
	listErrs  []error
	i         int
	created   []map[string]any
	createErr error
}

func (s *breedStore) List(context.Context, *storage.SecurityContext, *storage.StorageContext, storage.ListFilter) (*storage.QueryResult, error) {
	idx := s.i
	s.i++
	if idx < len(s.listErrs) && s.listErrs[idx] != nil {
		return nil, s.listErrs[idx]
	}
	if idx < len(s.lists) && s.lists[idx] != nil {
		return s.lists[idx], nil
	}
	return &storage.QueryResult{}, nil
}

func (s *breedStore) Create(_ context.Context, _ *storage.SecurityContext, obj map[string]any) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.created = append(s.created, obj)
	return nil
}

type stubMutator struct {
	path string
	err  error
}

func (s stubMutator) MutateSkill(context.Context, string, []string) (string, error) {
	return s.path, s.err
}

func TestExtraBreedEngine(t *testing.T) {
	ctx := context.Background()
	eng := NewEngine(&breedStore{listErrs: []error{errors.New("list")}}, stubMutator{}, 0.5)
	if err := eng.Breed(ctx); err == nil {
		t.Fatal("expected list error")
	}

	skip := &breedStore{lists: []*storage.QueryResult{
		{Objects: []map[string]any{{}, {objects.FieldKeyID: "SKL-1"}}},
		{},
	}}
	if err := NewEngine(skip, stubMutator{}, 0.5).Breed(ctx); err != nil {
		t.Fatal(err)
	}

	reportsFail := &breedStore{
		lists: []*storage.QueryResult{
			{Objects: []map[string]any{{objects.FieldKeyID: "SKL-1"}}},
		},
		listErrs: []error{nil, errors.New("reports")},
	}
	if err := NewEngine(reportsFail, stubMutator{}, 0.5).Breed(ctx); err != nil {
		t.Fatal(err)
	}

	noPath := &breedStore{lists: []*storage.QueryResult{
		{Objects: []map[string]any{{objects.FieldKeyID: "SKL-1"}}},
		{Objects: []map[string]any{{objects.FieldKeyFitnessScore: 0.1}}},
		{Objects: []map[string]any{{objects.FieldKeyAnalysis: "slow"}}},
	}}
	if err := NewEngine(noPath, stubMutator{path: "new.md"}, 0.5).Breed(ctx); err != nil {
		t.Fatal(err)
	}

	mutFail := &breedStore{lists: []*storage.QueryResult{
		{Objects: []map[string]any{{objects.FieldKeyID: "SKL-1", objects.FieldKeyFilePath: "old.md", objects.FieldKeyProvider: "p"}}},
		{Objects: []map[string]any{{objects.FieldKeyFitnessScore: 0.1}}},
		{},
	}}
	if err := NewEngine(mutFail, stubMutator{err: errors.New("mutate")}, 0.5).Breed(ctx); err != nil {
		t.Fatal(err)
	}

	ok := &breedStore{lists: []*storage.QueryResult{
		{Objects: []map[string]any{{objects.FieldKeyID: "SKL-1", objects.FieldKeyFilePath: "old.md", objects.FieldKeyProvider: "p"}}},
		{Objects: []map[string]any{{objects.FieldKeyFitnessScore: 0.1}}},
		{Objects: []map[string]any{{objects.FieldKeyAnalysis: "tighten"}}},
	}}
	engOK := NewEngine(ok, stubMutator{path: "new.md"}, 0.5)
	if err := engOK.Breed(ctx); err != nil {
		t.Fatal(err)
	}
	if len(ok.created) != 1 {
		t.Fatalf("created = %d", len(ok.created))
	}
}

func TestExtraBreedListener(t *testing.T) {
	store := &breedStore{lists: []*storage.QueryResult{
		{Objects: []map[string]any{{objects.FieldKeyID: "SKL-1", objects.FieldKeyFilePath: "old.md", objects.FieldKeyProvider: "p"}}},
		{Objects: []map[string]any{{objects.FieldKeyFitnessScore: 0.1}}},
		{Objects: []map[string]any{{objects.FieldKeyAnalysis: "tighten"}}},
	}}
	router := events.NewRouter()
	listener := NewListener(router, NewEngine(store, stubMutator{path: "new.md"}, 0.5))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listener.Start(ctx)
	router.Publish(events.Shape{Type: events.EventTypeObjectMutated, Kind: objects.KindAgentSkill, TargetID: "skip"})
	router.Publish(events.Shape{Type: events.EventTypeObjectMutated, Kind: objects.MaturationReport, TargetID: "r"})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(store.created) > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("listener did not trigger breed")
}
