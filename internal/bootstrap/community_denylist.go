package bootstrap

import "strings"

// communityCommandSpecDenylist path fragments excluded from community bootstrap extract
// (core-backlog). Matched case-insensitively against archive entry paths.
var communityCommandSpecDenylist = []string{
	"print_cursor_paste_applescript",
	"record_cvs_orchestrate_run",
	"paste_cursor",
	"zqk_cursor",
}

func shouldExcludeCommunityCommandSpec(archivePath string) bool {
	lower := strings.ToLower(archivePath)
	for _, frag := range communityCommandSpecDenylist {
		if strings.Contains(lower, frag) {
			return true
		}
	}
	return false
}
