package objects

import (
	"testing"
)

func TestGetDisplayLength(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		spec       *Spec
		fieldName  string
		defaultLen int
		want       int
	}{
		{
			name:       "nil spec returns default",
			spec:       nil,
			fieldName:  "test_field",
			defaultLen: 20,
			want:       20,
		},
		{
			name: "spec with nil ResolvedFields returns default",
			spec: &Spec{
				ResolvedFields: nil,
			},
			fieldName:  "test_field",
			defaultLen: 20,
			want:       20,
		},
		{
			name: "field not found returns default",
			spec: &Spec{
				ResolvedFields: map[string]any{
					"other_field": map[string]any{},
				},
			},
			fieldName:  "test_field",
			defaultLen: 20,
			want:       20,
		},
		{
			name: "field with display_length as int",
			spec: &Spec{
				ResolvedFields: map[string]any{
					"test_field": map[string]any{
						"validation": map[string]any{
							"display_length": 30,
						},
					},
				},
			},
			fieldName:  "test_field",
			defaultLen: 20,
			want:       30,
		},
		{
			name: "field with display_length as float64 (YAML parsing)",
			spec: &Spec{
				ResolvedFields: map[string]any{
					"test_field": map[string]any{
						"validation": map[string]any{
							"display_length": float64(25),
						},
					},
				},
			},
			fieldName:  "test_field",
			defaultLen: 20,
			want:       25,
		},
		{
			name: "field without validation section returns default",
			spec: &Spec{
				ResolvedFields: map[string]any{
					"test_field": map[string]any{
						FieldKeyType: "string",
					},
				},
			},
			fieldName:  "test_field",
			defaultLen: 20,
			want:       20,
		},
		{
			name: "field with validation but no display_length returns default",
			spec: &Spec{
				ResolvedFields: map[string]any{
					"test_field": map[string]any{
						"validation": map[string]any{
							"required": true,
						},
					},
				},
			},
			fieldName:  "test_field",
			defaultLen: 20,
			want:       20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetDisplayLength(tt.spec, tt.fieldName, tt.defaultLen)
			if got != tt.want {
				t.Errorf("GetDisplayLength() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetDisplayLengthFromFieldDef(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		fieldDef   map[string]any
		defaultLen int
		want       int
	}{
		{
			name:       "nil fieldDef returns default",
			fieldDef:   nil,
			defaultLen: 20,
			want:       20,
		},
		{
			name: "fieldDef with display_length as int",
			fieldDef: map[string]any{
				"validation": map[string]any{
					"display_length": 30,
				},
			},
			defaultLen: 20,
			want:       30,
		},
		{
			name: "fieldDef with display_length as float64",
			fieldDef: map[string]any{
				"validation": map[string]any{
					"display_length": float64(25),
				},
			},
			defaultLen: 20,
			want:       25,
		},
		{
			name: "fieldDef without validation section returns default",
			fieldDef: map[string]any{
				FieldKeyType: "string",
			},
			defaultLen: 20,
			want:       20,
		},
		{
			name: "fieldDef with validation but no display_length returns default",
			fieldDef: map[string]any{
				"validation": map[string]any{
					"required": true,
				},
			},
			defaultLen: 20,
			want:       20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetDisplayLengthFromFieldDef(tt.fieldDef, tt.defaultLen)
			if got != tt.want {
				t.Errorf("GetDisplayLengthFromFieldDef() = %v, want %v", got, tt.want)
			}
		})
	}
}
