package validation

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestLinkedBacklogItemsAllTerminal(t *testing.T) {
	tests := []struct {
		name          string
		planID        string
		childStatuses map[string]string
		want          bool
	}{
		{
			name:   "all complete children",
			planID: "PRI-TEST-001",
			childStatuses: map[string]string{
				"BLI-001": objects.ObjectStatusComplete,
				"BLI-002": objects.ObjectStatusComplete,
			},
			want: true,
		},
		{
			name:   "mixed complete and archived children",
			planID: "PRI-TEST-001",
			childStatuses: map[string]string{
				"BLI-001": objects.ObjectStatusComplete,
				"BLI-002": objects.ObjectStatusArchived,
			},
			want: true,
		},
		{
			name:   "has deferred child",
			planID: "PRI-TEST-001",
			childStatuses: map[string]string{
				"BLI-001": objects.ObjectStatusComplete,
				"BLI-002": objects.ObjectStatusDeferred,
			},
			want: false,
		},
		{
			name:   "has roadmap child",
			planID: "PRI-TEST-001",
			childStatuses: map[string]string{
				"BLI-001": objects.ObjectStatusComplete,
				"BLI-002": objects.ObjectStatusRoadmap,
			},
			want: false,
		},
		{
			name:   "has in_progress child",
			planID: "PRI-TEST-001",
			childStatuses: map[string]string{
				"BLI-001": objects.ObjectStatusComplete,
				"BLI-002": objects.ObjectStatusInProgress,
			},
			want: false,
		},
		{
			name:   "has planned child",
			planID: "PRI-TEST-001",
			childStatuses: map[string]string{
				"BLI-001": objects.ObjectStatusComplete,
				"BLI-002": objects.ObjectStatusPlanned,
			},
			want: false,
		},
		{
			name:   "has exploring child",
			planID: "PRI-TEST-001",
			childStatuses: map[string]string{
				"BLI-001": objects.ObjectStatusComplete,
				"BLI-002": objects.ObjectStatusExploring,
			},
			want: false,
		},
		{
			name:          "no children (vacuous true)",
			planID:        "PRI-TEST-001",
			childStatuses: map[string]string{},
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := make([]string, 0, len(tt.childStatuses))
			for id := range tt.childStatuses {
				deps = append(deps, id)
			}
			opts := &ValidationOptions{
				DependentsLookup: func(id string) []string {
					if id == tt.planID {
						return deps
					}
					return nil
				},
				ObjectStatusLookup: func(id string) (string, error) {
					return tt.childStatuses[id], nil
				},
				ObjectLookup: func(id string) (map[string]any, error) {
					return map[string]any{
						objects.FieldKeyID:              id,
						objects.FieldKeyKind:            objects.KindBacklogItem,
						objects.FieldKeyStatus:          tt.childStatuses[id],
						objects.FieldKeyPriorityPlanRef: tt.planID,
					}, nil
				},
			}
			got := LinkedBacklogItemsAllTerminal(tt.planID, opts, func(id string) string {
				return objects.KindBacklogItem
			})
			if got != tt.want {
				t.Errorf("LinkedBacklogItemsAllTerminal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLinkedBacklogItemsAllReadyOrLater(t *testing.T) {
	tests := []struct {
		name          string
		planID        string
		childStatuses map[string]string
		want          bool
	}{
		{
			name:   "all planned children",
			planID: "PRI-TEST-001",
			childStatuses: map[string]string{
				"BLI-001": objects.ObjectStatusPlanned,
				"BLI-002": objects.ObjectStatusPlanned,
			},
			want: true,
		},
		{
			name:   "mixed planned and in_progress children",
			planID: "PRI-TEST-001",
			childStatuses: map[string]string{
				"BLI-001": objects.ObjectStatusPlanned,
				"BLI-002": objects.ObjectStatusInProgress,
			},
			want: true,
		},
		{
			name:   "mixed planned and complete children",
			planID: "PRI-TEST-001",
			childStatuses: map[string]string{
				"BLI-001": objects.ObjectStatusPlanned,
				"BLI-002": objects.ObjectStatusComplete,
			},
			want: true,
		},
		{
			name:   "has deferred child",
			planID: "PRI-TEST-001",
			childStatuses: map[string]string{
				"BLI-001": objects.ObjectStatusPlanned,
				"BLI-002": objects.ObjectStatusDeferred,
			},
			want: false,
		},
		{
			name:   "has roadmap child",
			planID: "PRI-TEST-001",
			childStatuses: map[string]string{
				"BLI-001": objects.ObjectStatusPlanned,
				"BLI-002": objects.ObjectStatusRoadmap,
			},
			want: false,
		},
		{
			name:   "has exploring child",
			planID: "PRI-TEST-001",
			childStatuses: map[string]string{
				"BLI-001": objects.ObjectStatusPlanned,
				"BLI-002": objects.ObjectStatusExploring,
			},
			want: false,
		},
		{
			name:   "has validated child",
			planID: "PRI-TEST-001",
			childStatuses: map[string]string{
				"BLI-001": objects.ObjectStatusPlanned,
				"BLI-002": objects.ObjectStatusValidated,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := make([]string, 0, len(tt.childStatuses))
			for id := range tt.childStatuses {
				deps = append(deps, id)
			}
			opts := &ValidationOptions{
				DependentsLookup: func(id string) []string {
					if id == tt.planID {
						return deps
					}
					return nil
				},
				ObjectStatusLookup: func(id string) (string, error) {
					return tt.childStatuses[id], nil
				},
				ObjectLookup: func(id string) (map[string]any, error) {
					return map[string]any{
						objects.FieldKeyID:              id,
						objects.FieldKeyKind:            objects.KindBacklogItem,
						objects.FieldKeyStatus:          tt.childStatuses[id],
						objects.FieldKeyPriorityPlanRef: tt.planID,
					}, nil
				},
			}
			got := LinkedBacklogItemsAllReadyOrLater(tt.planID, opts, func(id string) string {
				return objects.KindBacklogItem
			})
			if got != tt.want {
				t.Errorf("LinkedBacklogItemsAllReadyOrLater() = %v, want %v", got, tt.want)
			}
		})
	}
}
