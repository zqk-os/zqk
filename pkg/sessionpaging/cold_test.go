package sessionpaging_test

import (
	"bytes"
	"fmt"
	"testing"

	zqkctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/sessionpaging"
)

func TestColdStore_PutRecallRoundTrip(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cold, err := sessionpaging.NewColdStore(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("early fact: go concurrency patterns")
	if err := cold.Put("sess-1", "fact-early", want); err != nil {
		t.Fatal(err)
	}
	got, err := cold.Recall("sess-1", "fact-early")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("recall mismatch: %q", got)
	}
}

func TestEvictHotToCold_PreservesEarlyFact(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	hot, err := sessionpaging.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	cold, err := sessionpaging.NewColdStore(root)
	if err != nil {
		t.Fatal(err)
	}
	budget, err := zqkctx.NewTokenBudget(50, 100)
	if err != nil {
		t.Fatal(err)
	}

	early := []byte("episode-0: install path")
	if err := hot.Put("agent-a", "page-0", early); err != nil {
		t.Fatal(err)
	}
	budget.Add(60) // crosses soft limit → ShouldEvict
	if !budget.ShouldEvict() {
		t.Fatal("expected soft-limit eviction signal")
	}

	payload, err := sessionpaging.EvictHotToCold(hot, cold, "agent-a", "page-0")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, early) {
		t.Fatalf("evicted payload=%q", payload)
	}
	if _, err := hot.Get("agent-a", "page-0"); err == nil {
		t.Fatal("expected hot page removed after eviction")
	}
	recalled, err := cold.Recall("agent-a", "page-0")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recalled, early) {
		t.Fatalf("cold recall=%q want %q", recalled, early)
	}

	// Synthetic multi-turn: later hot pages do not erase cold early fact.
	for i := 1; i <= 5; i++ {
		if err := hot.Put("agent-a", fmt.Sprintf("page-%d", i), []byte("later")); err != nil {
			t.Fatal(err)
		}
		budget.Add(10)
	}
	still, err := cold.Recall("agent-a", "page-0")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(still, early) {
		t.Fatalf("after later turns cold=%q", still)
	}
}
