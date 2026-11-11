package bootstrap

import "strings"

// communityBootstrapDenylist path fragments excluded from community bootstrap extract
// (core-backlog / REQ seed impermeability). Matched case-insensitively against archive
// entry paths. Covers CAP/MMORCH/orchestrate/TPM operator ontology that must not land
// in a stranger init seed.
//
// TRACK: BLI-1794356846362471000-b781465c — keep CI script scripts/open-core/check-bootstrap-seed-impermeability.sh in sync when extending.
var communityBootstrapDenylist = []string{
	"print_cursor_paste_applescript",
	"print_ide_paste_applescript",
	"paste_applescript",
	"record_cvs_orchestrate_run",
	"paste_cursor",
	"zqk_cursor",
	"cap_night_duty",
	"cap_orchestrator",
	"cap_background",
	"mmorch",
	"orchestrate_command",
	"orchestrate_batch",
	"tpm_orchestration",
	"/agent/orchestrate",
}

// communityCommandSpecDenylist is the historical name retained for callers/tests.
// Prefer communityBootstrapDenylist for new references.
var communityCommandSpecDenylist = communityBootstrapDenylist

func shouldExcludeCommunityCommandSpec(archivePath string) bool {
	lower := strings.ToLower(archivePath)
	for _, frag := range communityBootstrapDenylist {
		if strings.Contains(lower, frag) {
			return true
		}
	}
	return false
}

// CommunityBootstrapDenylist returns a copy of path fragments scrubbed from
// community-edition bootstrap extract (for CI / docs inventory).
func CommunityBootstrapDenylist() []string {
	out := make([]string, len(communityBootstrapDenylist))
	copy(out, communityBootstrapDenylist)
	return out
}
