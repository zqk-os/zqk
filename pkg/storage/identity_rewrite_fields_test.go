package storage

import "testing"

func TestIsIdentityRewriteField(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		want bool
	}{
		{"created_by", true},
		{"updated_by", true},
		{"account_id", true},
		{"priority_plan_ref", true},
		{"goal_refs", true},
		{"title", false},
		{"status", false},
	}
	for _, tc := range cases {
		if got := isIdentityRewriteField(tc.name); got != tc.want {
			t.Errorf("isIdentityRewriteField(%q)=%v want %v", tc.name, got, tc.want)
		}
	}
}
