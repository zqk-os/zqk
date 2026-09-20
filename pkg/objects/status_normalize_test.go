package objects

import "testing"

func TestNormalizeStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status string
		want   string
	}{
		{"", ""},
		{"complete", "complete"},
		{"completed", "complete"},
		{"archived", "archived"},
		{"archive", "archived"},
		{"canceled", "cancelled"},
		{"cancelled", "cancelled"},
		{"exploring", "exploring"},
		{"in_progress", "in_progress"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := NormalizeStatus(tt.status)
			if got != tt.want {
				t.Errorf("NormalizeStatus(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestApplyAliasesForStatus(t *testing.T) {
	t.Parallel()
	// Kind has "complete" -> "completed" maps to "complete"
	withComplete := map[string]bool{"exploring": true, "complete": true, "archived": true}
	if got := ApplyAliasesForStatus("completed", withComplete); got != "complete" {
		t.Errorf("ApplyAliasesForStatus(completed, withComplete) = %q, want complete", got)
	}
	// Kind has "completed" but not "complete" -> "completed" stays (e.g. audit_event)
	withCompleted := map[string]bool{"pending": true, "completed": true, "archived": true}
	if got := ApplyAliasesForStatus("completed", withCompleted); got != "completed" {
		t.Errorf("ApplyAliasesForStatus(completed, withCompleted) = %q, want completed", got)
	}
	// "archive" -> "archived" when "archived" is valid
	if got := ApplyAliasesForStatus("archive", withComplete); got != "archived" {
		t.Errorf("ApplyAliasesForStatus(archive, withComplete) = %q, want archived", got)
	}
	// "canceled" -> "cancelled" when "cancelled" is valid
	withCancelled := map[string]bool{"active": true, "cancelled": true}
	if got := ApplyAliasesForStatus("canceled", withCancelled); got != "cancelled" {
		t.Errorf("ApplyAliasesForStatus(canceled, withCancelled) = %q, want cancelled", got)
	}
}

func TestRegisterStatusAlias(t *testing.T) {
	RegisterStatusAlias("done", "complete")
	if got := NormalizeStatus("done"); got != "complete" {
		t.Errorf("after RegisterStatusAlias(done, complete), NormalizeStatus(done) = %q, want complete", got)
	}
}
