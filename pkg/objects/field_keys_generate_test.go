package objects

import "testing"

func TestFieldKeyConstName(t *testing.T) {
	tests := []struct {
		field string
		want  string
	}{
		{"id", "ID"},
		{"priority_plan_ref", "PriorityPlanRef"},
		{"api_endpoint", "APIEndpoint"},
		{"kind_under_test", "KindUnderTest"},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			if got := FieldKeyConstName(tt.field); got != tt.want {
				t.Errorf("FieldKeyConstName(%q) = %q, want %q", tt.field, got, tt.want)
			}
		})
	}
}
