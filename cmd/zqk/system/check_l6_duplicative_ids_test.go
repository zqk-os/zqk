package system

import (
	"testing"

	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
)

func TestDedupeCheckResultIssues(t *testing.T) {
	issues := []Issue{
		{Tier: 1, Category: "registration", Message: "Duplicate CAS blob"},
		{Tier: 1, Category: "registration", Message: "Duplicate CAS blob"},
		{Tier: 2, Category: "reference", Message: "Missing reference"},
	}

	deduped := DedupeCheckResultIssues(issues)
	if len(deduped) != 2 {
		t.Fatalf("expected 2 deduped issues, got %d", len(deduped))
	}
	if deduped[0].Category != "registration" || deduped[1].Category != "reference" {
		t.Errorf("unexpected issue order/content: %+v", deduped)
	}
}

func TestAppendCASDuplicateIDCheckResults_DedupesByObjectID(t *testing.T) {
	initialResults := []CheckResult{
		{
			ObjectID:   "BLI-DUPE-001",
			ObjectKind: "backlog_item",
			FilePath:   "/path/to/keeper.yaml",
			Issues: []Issue{
				{Tier: 3, Category: "CacheLag", Message: "CacheLag: Referenced object exists"},
			},
		},
	}

	inv := caspkg.CASDuplicateIDInventory{
		DuplicateCount: 1,
		Hits: []caspkg.CASDuplicateIDHit{
			{
				ObjectID:   "BLI-DUPE-001",
				Kind:       "backlog_item",
				KeeperPath: "/path/to/keeper.yaml",
				Paths:      []string{"/path/to/keeper.yaml", "/path/to/loser.yaml"},
			},
		},
	}

	results := appendCASDuplicateIDCheckResults(initialResults, inv)
	if len(results) != 1 {
		t.Fatalf("expected 1 aggregated result row for BLI-DUPE-001, got %d", len(results))
	}
	if len(results[0].Issues) != 2 {
		t.Fatalf("expected 2 issues under BLI-DUPE-001, got %d", len(results[0].Issues))
	}
}
