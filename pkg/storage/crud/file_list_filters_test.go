package crud_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/crud"
)

func TestIdsFromListFilter(t *testing.T) {
	tests := []struct {
		name    string
		filters map[string]any
		want    []string
	}{
		{
			name:    "empty filters",
			filters: nil,
			want:    nil,
		},
		{
			name: "single string id",
			filters: map[string]any{
				objects.FieldKeyID: "BLI-001",
			},
			want: []string{"BLI-001"},
		},
		{
			name: "operator $eq",
			filters: map[string]any{
				objects.FieldKeyID: map[string]any{"$eq": "BLI-002"},
			},
			want: []string{"BLI-002"},
		},
		{
			name: "operator $in",
			filters: map[string]any{
				objects.FieldKeyID: map[string]any{"$in": []any{"BLI-003", "BLI-004"}},
			},
			want: []string{"BLI-003", "BLI-004"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := crud.IdsFromListFilter(tt.filters)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("IdsFromListFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReferenceValuesFromListFilter(t *testing.T) {
	tests := []struct {
		name    string
		filters map[string]any
		want    []string
	}{
		{
			name:    "empty filters",
			filters: nil,
			want:    nil,
		},
		{
			name: "no reference fields",
			filters: map[string]any{
				objects.FieldKeyStatus: objects.ObjectStatusComplete,
				objects.FieldKeyTitle:  "Test",
			},
			want: nil,
		},
		{
			name: "single string _ref",
			filters: map[string]any{
				objects.FieldKeyPriorityPlanRef: "PRI-001",
			},
			want: []string{"PRI-001"},
		},
		{
			name: "slice string _refs",
			filters: map[string]any{
				objects.FieldKeyCriteriaRefs: []string{"CRIT-001", "CRIT-002"},
			},
			want: []string{"CRIT-001", "CRIT-002"},
		},
		{
			name: "slice any _refs",
			filters: map[string]any{
				objects.FieldKeyMilestoneRefs: []any{"MIL-001", "MIL-002"},
			},
			want: []string{"MIL-001", "MIL-002"},
		},
		{
			name: "operator $eq",
			filters: map[string]any{
				objects.FieldKeyGoalRefs: map[string]any{"$eq": "GOAL-001"},
			},
			want: []string{"GOAL-001"},
		},
		{
			name: "operator $in",
			filters: map[string]any{
				objects.FieldKeyRequirementRefs: map[string]any{"$in": []any{"REQ-001", "REQ-002"}},
			},
			want: []string{"REQ-001", "REQ-002"},
		},
		{
			name: "multiple ref filters with deduplication",
			filters: map[string]any{
				objects.FieldKeyPriorityPlanRef: "PRI-001",
				objects.FieldKeyMilestoneRef:    "PRI-001", // duplicate value across different fields
			},
			want: []string{"PRI-001"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := crud.ReferenceValuesFromListFilter(tt.filters)
			sort.Strings(got)
			sort.Strings(tt.want)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReferenceValuesFromListFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}
