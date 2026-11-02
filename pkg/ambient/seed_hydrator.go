package ambient

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// Traceability: BLI-1793578878406936000-670dab43, CRIT-1793578812700305000-d188af82,
// CRIT-1793578818089397000-f11a5a3e
// Seed-to-Stream Intent Hydrator — pools anchored intent seeds into structured
// priority-plan streams without prompt drift. Corrupt/unanchored signals fail closed.

var (
	// ErrIntentSeedIDRequired is returned when a seed lacks an identity.
	ErrIntentSeedIDRequired = errfmt.Errorf("ambient intent seed id is required")
	// ErrIntentSeedTitleRequired is returned when a seed lacks a structured title.
	ErrIntentSeedTitleRequired = errfmt.Errorf("ambient intent seed title is required")
	// ErrIntentSeedUnanchored is returned when vision/mission/goal anchors are missing.
	ErrIntentSeedUnanchored = errfmt.Errorf("ambient intent seed is unanchored")
	// ErrIntentSeedCorrupt is returned when anchor IDs use invalid kind prefixes.
	ErrIntentSeedCorrupt = errfmt.Errorf("ambient intent seed is corrupt")
)

const (
	intentSeedVisionPrefix  = "VIS-"
	intentSeedMissionPrefix = "MIS-"
	intentSeedGoalPrefix    = "GOAL-"
	defaultIntentSeedTheme  = "default"
)

// IntentSeed is a durable operator intent signal awaiting hydration into a plan stream.
type IntentSeed struct {
	ID         string
	Title      string
	Theme      string
	VisionRef  string
	MissionRef string
	GoalRef    string
}

// PriorityPlanStream is a structured priority-plan draft produced from pooled seeds.
// Titles and anchors are derived only from validated seed fields (no freeform prompt drift).
type PriorityPlanStream struct {
	Title      string
	Theme      string
	VisionRef  string
	MissionRef string
	GoalRefs   []string
	SeedIDs    []string
}

// SeedHydrator pools validated intent seeds into priority-plan streams.
type SeedHydrator struct{}

// NewSeedHydrator constructs a SeedHydrator.
func NewSeedHydrator() *SeedHydrator {
	return &SeedHydrator{}
}

// Hydrate validates every seed then pools by (vision, mission, theme).
// On any corrupt/unanchored seed it fails closed with nil streams (no partial emit).
func (h *SeedHydrator) Hydrate(seeds []IntentSeed) ([]PriorityPlanStream, error) {
	if len(seeds) == 0 {
		return []PriorityPlanStream{}, nil
	}

	type poolKey struct {
		vision  string
		mission string
		theme   string
	}
	type poolAcc struct {
		title    string
		seedIDs  []string
		goalRefs []string
		seenSeed map[string]struct{}
	}

	pools := make(map[poolKey]*poolAcc)
	order := make([]poolKey, 0)

	for _, seed := range seeds {
		if err := validateIntentSeed(seed); err != nil {
			return nil, err
		}
		theme := strings.TrimSpace(seed.Theme)
		if theme == "" {
			theme = defaultIntentSeedTheme
		}
		key := poolKey{
			vision:  seed.VisionRef,
			mission: seed.MissionRef,
			theme:   theme,
		}
		acc, ok := pools[key]
		if !ok {
			acc = &poolAcc{
				title:    structuredStreamTitle(theme, seed.VisionRef, seed.MissionRef),
				seenSeed: make(map[string]struct{}),
			}
			pools[key] = acc
			order = append(order, key)
		}
		if _, dup := acc.seenSeed[seed.ID]; dup {
			continue
		}
		acc.seenSeed[seed.ID] = struct{}{}
		acc.seedIDs = append(acc.seedIDs, seed.ID)
		acc.goalRefs = append(acc.goalRefs, seed.GoalRef)
	}

	streams := make([]PriorityPlanStream, 0, len(order))
	for _, key := range order {
		acc := pools[key]
		streams = append(streams, PriorityPlanStream{
			Title:      acc.title,
			Theme:      key.theme,
			VisionRef:  key.vision,
			MissionRef: key.mission,
			GoalRefs:   append([]string(nil), acc.goalRefs...),
			SeedIDs:    append([]string(nil), acc.seedIDs...),
		})
	}

	// Deterministic order for callers that do not rely on insertion sequence.
	sort.SliceStable(streams, func(i, j int) bool {
		if streams[i].Theme != streams[j].Theme {
			return streams[i].Theme < streams[j].Theme
		}
		if streams[i].VisionRef != streams[j].VisionRef {
			return streams[i].VisionRef < streams[j].VisionRef
		}
		return streams[i].MissionRef < streams[j].MissionRef
	})

	return streams, nil
}

func validateIntentSeed(seed IntentSeed) error {
	if strings.TrimSpace(seed.ID) == "" {
		return ErrIntentSeedIDRequired
	}
	if strings.TrimSpace(seed.Title) == "" {
		return ErrIntentSeedTitleRequired
	}
	if strings.TrimSpace(seed.VisionRef) == "" ||
		strings.TrimSpace(seed.MissionRef) == "" ||
		strings.TrimSpace(seed.GoalRef) == "" {
		return ErrIntentSeedUnanchored
	}
	if !hasPrefixFold(seed.VisionRef, intentSeedVisionPrefix) ||
		!hasPrefixFold(seed.MissionRef, intentSeedMissionPrefix) ||
		!hasPrefixFold(seed.GoalRef, intentSeedGoalPrefix) {
		return ErrIntentSeedCorrupt
	}
	return nil
}

func hasPrefixFold(value, prefix string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(value)), strings.ToUpper(prefix))
}

func structuredStreamTitle(theme, visionRef, missionRef string) string {
	return fmt.Sprintf("PRI stream [%s] %s/%s", theme, visionRef, missionRef)
}
