package bootstrap

import "testing"

func TestShouldExcludeCommunityCommandSpec(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path string
		want bool
	}{
		{"cli_specs/scheduler/print_cursor_paste_applescript_command.yaml", true},
		{"cli_specs/scheduler/record_cvs_orchestrate_run_command.yaml", true},
		{"cli_specs/agent/paste_cursor_command.yaml", true},
		{"cli_specs/scheduler/scan_tests_command.yaml", false},
		{"cli_specs/scheduler/activity_command.yaml", false},
		{"object_specs/mission.yaml", false},
	}
	for _, tc := range cases {
		if got := shouldExcludeCommunityCommandSpec(tc.path); got != tc.want {
			t.Errorf("shouldExcludeCommunityCommandSpec(%q)=%v want %v", tc.path, got, tc.want)
		}
	}
}
