package state

import (
	"strings"
	"testing"
)

func TestStateTree_RenderText(t *testing.T) {
	t.Parallel()

	payload := &StateTreePayload{
		Organization: &struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}{
			ID:    "ORG-TEST-001",
			Title: "Test Organization",
		},
		Mission: &struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}{
			ID:    "MIS-TEST-001",
			Title: "Test Mission",
		},
		Vision: &struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}{
			ID:    "VIS-TEST-001",
			Title: "Test Vision",
		},
		Goal: &struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}{
			ID:    "GOAL-TEST-001",
			Title: "Test Goal",
		},
		ActivePlans: []PriorityPlanNode{
			{
				ID:          "PRI-TEST-001",
				Title:       "Test Priority Plan",
				Status:      "active",
				Workstreams: []string{"WS-TEST-001"},
				Requirements: []RequirementNode{
					{
						ID:     "REQ-TEST-001",
						Title:  "Test Requirement",
						Status: "active",
						BacklogItems: []BacklogItemNode{
							{
								ID:        "BLI-TEST-001",
								Title:     "Test Backlog Item",
								Status:    "in_progress",
								Priority:  "P0",
								ClaimedBy: "PER-DEVELOPER",
							},
						},
					},
				},
			},
		},
		RecentMutations: []JournalMutation{
			{
				ID:          "CHA-001",
				ChangeType:  "update",
				ObjectRef:   "backlog_item:BLI-TEST-001",
				DiffSummary: "status: planned -> in_progress",
			},
		},
	}

	out := FormatStateTreeText(payload)
	if !strings.Contains(out, "ORG-TEST-001") {
		t.Errorf("expected ORG-TEST-001 in output, got: %s", out)
	}
	if !strings.Contains(out, "MIS-TEST-001") {
		t.Errorf("expected MIS-TEST-001 in output, got: %s", out)
	}
	if !strings.Contains(out, "PRI-TEST-001") {
		t.Errorf("expected PRI-TEST-001 in output, got: %s", out)
	}
	if !strings.Contains(out, "BLI-TEST-001") {
		t.Errorf("expected BLI-TEST-001 in output, got: %s", out)
	}
	if !strings.Contains(out, "PER-DEVELOPER") {
		t.Errorf("expected leased agent in output, got: %s", out)
	}
	if !strings.Contains(out, "CHA-001") {
		t.Errorf("expected CHA-001 mutation in output, got: %s", out)
	}
}

func TestStateCommands_Structure(t *testing.T) {
	t.Parallel()

	stateCmd := NewStateCmd()
	if stateCmd.Use != "state" {
		t.Fatalf("expected command use 'state', got %q", stateCmd.Use)
	}

	subCommands := stateCmd.Commands()
	foundTree := false
	foundJournal := false

	for _, sub := range subCommands {
		if sub.Name() == "tree" {
			foundTree = true
		}
		if sub.Name() == "journal" {
			foundJournal = true
		}
	}

	if !foundTree {
		t.Error("expected 'tree' subcommand on stateCmd")
	}
	if !foundJournal {
		t.Error("expected 'journal' subcommand on stateCmd")
	}
}
