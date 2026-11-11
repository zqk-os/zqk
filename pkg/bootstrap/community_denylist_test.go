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
		{"cli_specs/agent/orchestrate_command.yaml", true},
		{"cli_specs/system/orchestrate_batch_command.yaml", true},
		{"scripts/scheduler_jobs/cap_night_duty.yaml", true},
		{"scripts/scheduler_jobs/cap_orchestrator.yaml", true},
		{"scripts/default_agent_skills/tpm_orchestration.yaml", true},
		{"docs/onboarding/MMORCH_PEER_SESSION_BOOTSTRAP.md", true},
		{"cli_specs/scheduler/activity_command.yaml", false},
		{"object_specs/mission.yaml", false},
		{"scripts/scheduler_jobs/cleanup.yaml", false},
	}
	for _, tc := range cases {
		if got := shouldExcludeCommunityCommandSpec(tc.path); got != tc.want {
			t.Errorf("shouldExcludeCommunityCommandSpec(%q)=%v want %v", tc.path, got, tc.want)
		}
	}
}

func TestCommunityBootstrapDenylist_CoversCAPMMORCH(t *testing.T) {
	t.Parallel()
	required := []string{"cap_night_duty", "cap_orchestrator", "mmorch", "orchestrate_command", "tpm_orchestration"}
	have := map[string]bool{}
	for _, frag := range CommunityBootstrapDenylist() {
		have[frag] = true
	}
	for _, frag := range required {
		if !have[frag] {
			t.Errorf("CommunityBootstrapDenylist missing required fragment %q", frag)
		}
	}
}
