package precommit

import (
	"testing"
	"time"
)

func freshResult(ok bool, blocking bool) CategoryResult {
	return CategoryResult{
		OK:        ok,
		Summary:   "checked",
		Blocking:  blocking,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func agedResult(ok bool, blocking bool, age time.Duration) CategoryResult {
	return CategoryResult{
		OK:        ok,
		Summary:   "checked",
		Blocking:  blocking,
		UpdatedAt: time.Now().UTC().Add(-age).Format(time.RFC3339),
	}
}

// Block used to derive from OK alone, so a passing verdict stayed authoritative
// no matter how old it was. When the background jobs stopped refreshing,
// results.json kept reporting block=false and the hook let every commit through.
func TestAggregate_StaleBlockingCategoryBlocks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := WriteCategory(root, "policy", agedResult(true, true, MaxCategoryAge+time.Hour)); err != nil {
		t.Fatalf("write policy category: %v", err)
	}

	got, err := Aggregate(root)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if !got.Block {
		t.Fatalf("stale blocking category did not block; categories=%+v", got.Categories)
	}
	if !got.Categories["policy"].Stale {
		t.Fatalf("policy category not marked stale: %+v", got.Categories["policy"])
	}

	summary := BlockingCategoriesSummary(got)
	if len(summary) != 1 {
		t.Fatalf("expected one blocking summary line, got %v", summary)
	}
}

func TestAggregate_FreshPassDoesNotBlock(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := WriteCategory(root, "policy", freshResult(true, true)); err != nil {
		t.Fatalf("write policy category: %v", err)
	}

	got, err := Aggregate(root)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if got.Block {
		t.Fatalf("fresh passing category blocked: %+v", got.Categories)
	}
	if got.Categories["policy"].Stale {
		t.Fatalf("fresh category marked stale: %+v", got.Categories["policy"])
	}
}

// Age is only a reason to distrust a verdict that would otherwise open the gate;
// a non-blocking category must never close it.
func TestAggregate_StaleNonBlockingCategoryDoesNotBlock(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := WriteCategory(root, "integrity", agedResult(true, false, MaxCategoryAge+time.Hour)); err != nil {
		t.Fatalf("write integrity category: %v", err)
	}

	got, err := Aggregate(root)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if got.Block {
		t.Fatalf("stale non-blocking category blocked: %+v", got.Categories)
	}
}

func TestAggregate_FailingCategoryStillBlocks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := WriteCategory(root, "lint", freshResult(false, true)); err != nil {
		t.Fatalf("write lint category: %v", err)
	}

	got, err := Aggregate(root)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if !got.Block {
		t.Fatalf("failing blocking category did not block: %+v", got.Categories)
	}
}

// A missing or malformed timestamp cannot prove freshness, so it must not pass.
func TestCategoryResult_IsStaleOnUnusableTimestamp(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	for name, stamp := range map[string]string{
		"empty":     "",
		"malformed": "not-a-timestamp",
	} {
		result := CategoryResult{OK: true, Blocking: true, UpdatedAt: stamp}
		if !result.IsStale(now) {
			t.Errorf("%s timestamp treated as fresh", name)
		}
	}
}
