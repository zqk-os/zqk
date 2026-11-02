package ambient

import (
	"errors"
	"testing"
)

// Traceability: BLI-1793578878406936000-670dab43, CRIT-1793578812700305000-d188af82,
// CRIT-1793578818089397000-f11a5a3e
// Static Floor: hydrator construction and empty-pool identity.
func TestSeedHydrator_StaticFloor(t *testing.T) {
	h := NewSeedHydrator()
	if h == nil {
		t.Fatal("expected non-nil SeedHydrator")
	}

	streams, err := h.Hydrate(nil)
	if err != nil {
		t.Fatalf("nil seed slice must hydrate to empty stream: %v", err)
	}
	if len(streams) != 0 {
		t.Fatalf("expected empty streams, got %d", len(streams))
	}

	streams, err = h.Hydrate([]IntentSeed{})
	if err != nil {
		t.Fatalf("empty seed slice must hydrate to empty stream: %v", err)
	}
	if len(streams) != 0 {
		t.Fatalf("expected empty streams, got %d", len(streams))
	}
}

// Fail-closed: corrupt or unanchored intent signals must not mutate/emit streams.
func TestSeedHydrator_FailClosedCorruptUnanchored(t *testing.T) {
	h := NewSeedHydrator()

	cases := []struct {
		name string
		seed IntentSeed
		want error
	}{
		{
			name: "missing_id",
			seed: IntentSeed{
				Title: "Cycle intent", VisionRef: "VIS-1", MissionRef: "MIS-1", GoalRef: "GOAL-1",
			},
			want: ErrIntentSeedIDRequired,
		},
		{
			name: "missing_title",
			seed: IntentSeed{
				ID: "SEED-1", VisionRef: "VIS-1", MissionRef: "MIS-1", GoalRef: "GOAL-1",
			},
			want: ErrIntentSeedTitleRequired,
		},
		{
			name: "unanchored_no_vision",
			seed: IntentSeed{
				ID: "SEED-2", Title: "No vision", MissionRef: "MIS-1", GoalRef: "GOAL-1",
			},
			want: ErrIntentSeedUnanchored,
		},
		{
			name: "unanchored_no_mission",
			seed: IntentSeed{
				ID: "SEED-3", Title: "No mission", VisionRef: "VIS-1", GoalRef: "GOAL-1",
			},
			want: ErrIntentSeedUnanchored,
		},
		{
			name: "unanchored_no_goal",
			seed: IntentSeed{
				ID: "SEED-4", Title: "No goal", VisionRef: "VIS-1", MissionRef: "MIS-1",
			},
			want: ErrIntentSeedUnanchored,
		},
		{
			name: "corrupt_vision_prefix",
			seed: IntentSeed{
				ID: "SEED-5", Title: "Bad vision", VisionRef: "X-1", MissionRef: "MIS-1", GoalRef: "GOAL-1",
			},
			want: ErrIntentSeedCorrupt,
		},
		{
			name: "corrupt_mission_prefix",
			seed: IntentSeed{
				ID: "SEED-6", Title: "Bad mission", VisionRef: "VIS-1", MissionRef: "bad", GoalRef: "GOAL-1",
			},
			want: ErrIntentSeedCorrupt,
		},
		{
			name: "corrupt_goal_prefix",
			seed: IntentSeed{
				ID: "SEED-7", Title: "Bad goal", VisionRef: "VIS-1", MissionRef: "MIS-1", GoalRef: "G-1",
			},
			want: ErrIntentSeedCorrupt,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			streams, err := h.Hydrate([]IntentSeed{tc.seed})
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
			if streams != nil {
				t.Fatalf("fail-closed must return nil streams, got %#v", streams)
			}
		})
	}
}

// Operational: valid seeds pool into structured priority-plan streams without prompt drift.
func TestSeedHydrator_PoolsSeedsIntoStreams(t *testing.T) {
	h := NewSeedHydrator()

	seeds := []IntentSeed{
		{
			ID: "SEED-A1", Title: "Hydrate ambient holons", Theme: "ambient-middle-brain",
			VisionRef: "VIS-100", MissionRef: "MIS-200", GoalRef: "GOAL-300",
		},
		{
			ID: "SEED-A2", Title: "Branch hygiene automation", Theme: "ambient-middle-brain",
			VisionRef: "VIS-100", MissionRef: "MIS-200", GoalRef: "GOAL-301",
		},
		{
			ID: "SEED-B1", Title: "Separate theme", Theme: "observability",
			VisionRef: "VIS-100", MissionRef: "MIS-200", GoalRef: "GOAL-400",
		},
	}

	streams, err := h.Hydrate(seeds)
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if len(streams) != 2 {
		t.Fatalf("expected 2 pooled streams (by theme+anchors), got %d", len(streams))
	}

	byTheme := map[string]PriorityPlanStream{}
	for _, s := range streams {
		byTheme[s.Theme] = s
		if s.VisionRef != "VIS-100" || s.MissionRef != "MIS-200" {
			t.Fatalf("stream anchors drifted: %#v", s)
		}
		if s.Title == "" {
			t.Fatal("stream title must be structured (non-empty), never prompt-derived freeform empty")
		}
	}

	ambient := byTheme["ambient-middle-brain"]
	if len(ambient.SeedIDs) != 2 {
		t.Fatalf("ambient stream seed ids=%v", ambient.SeedIDs)
	}
	if len(ambient.GoalRefs) != 2 {
		t.Fatalf("ambient stream goal refs=%v", ambient.GoalRefs)
	}

	obs := byTheme["observability"]
	if len(obs.SeedIDs) != 1 || obs.SeedIDs[0] != "SEED-B1" {
		t.Fatalf("observability stream seed ids=%v", obs.SeedIDs)
	}
}

func TestSeedHydrator_DedupesSeedIDsWithinPool(t *testing.T) {
	h := NewSeedHydrator()
	seeds := []IntentSeed{
		{ID: "SEED-DUP", Title: "First", Theme: "t", VisionRef: "VIS-1", MissionRef: "MIS-1", GoalRef: "GOAL-1"},
		{ID: "SEED-DUP", Title: "Duplicate ignored", Theme: "t", VisionRef: "VIS-1", MissionRef: "MIS-1", GoalRef: "GOAL-2"},
	}
	streams, err := h.Hydrate(seeds)
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if len(streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(streams))
	}
	if len(streams[0].SeedIDs) != 1 {
		t.Fatalf("duplicate seed id must not expand pool: %v", streams[0].SeedIDs)
	}
	if len(streams[0].GoalRefs) != 1 || streams[0].GoalRefs[0] != "GOAL-1" {
		t.Fatalf("first-seen seed wins on dedupe: goals=%v", streams[0].GoalRefs)
	}
}
