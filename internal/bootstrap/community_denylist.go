package bootstrap

import "strings"

// communityCommandSpecDenylist path fragments and command names excluded from community bootstrap
var communityCommandSpecDenylist = []string{
	"print_cursor_paste_applescript",
	"record_cvs_orchestrate_run",
	"paste_cursor",
	"zqk_cursor",
	"generate-builders",
	"generate-instance-builders",
	"generate-command-builders",
	"generate-spec-index",
	"analyze-drift-hotspots",
	"autofix",
	"paste",
	"evolve",
	"env-literals",
	"validate-agent-rules",
	"clean-branches",
	"emergency-manager",
	"spec",
	"mesh",
	"agent",
	"keystore",
	"ambient",
	"matrix",
}

func shouldExcludeCommunityCommandSpec(archivePath string) bool {
	lower := strings.ToLower(archivePath)
	for _, frag := range communityCommandSpecDenylist {
		switch frag {
		case "spec":
			// Do not match cli_specs/ or object_specs/; match studio `spec` command surfaces.
			if strings.Contains(lower, "/spec/") || strings.Contains(lower, "spec_command") ||
				strings.Contains(lower, "generate-spec") || strings.HasPrefix(lower, "spec/") {
				return true
			}
		case "agent":
			// Do not match unrelated words; match agent CLI / agent_* command specs.
			if strings.Contains(lower, "/agent/") || strings.Contains(lower, "agent_") ||
				strings.Contains(lower, "agent-") || strings.Contains(lower, "agents_") {
				return true
			}
		default:
			if strings.Contains(lower, frag) {
				return true
			}
		}
	}
	return false
}
