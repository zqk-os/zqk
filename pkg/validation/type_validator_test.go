package validation

import "testing"

func TestValidateType_NumberAcceptsIntsAndFloats(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		value any
		want  bool
	}{
		{"int", int(1), true},
		{"int64", int64(1), true},
		{"uint", uint(1), true},
		{"float64", float64(1.25), true},
		{"float32", float32(1.25), true},
		{"bool", true, false},
		{"string", "1", false},
		{"nil", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateType(tc.value, "number")
			if got != tc.want {
				t.Fatalf("ValidateType(%T, number)=%v, want %v", tc.value, got, tc.want)
			}
		})
	}
}
