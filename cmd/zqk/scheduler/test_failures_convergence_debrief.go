package scheduler

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// maxConvergenceDebriefNotesLen matches convergence_session.debrief_notes validation (object spec).
const maxConvergenceDebriefNotesLen = 32000

// resolveDebriefNotesFromFlags returns text from --debrief-notes and/or --debrief-notes-file.
// If both are set, file content wins. Empty if neither supplies non-whitespace text.
func resolveDebriefNotesFromFlags(debriefNotesFlag, debriefNotesFile string) (string, error) {
	path := strings.TrimSpace(debriefNotesFile)
	if path != emptyValue {
		b, err := fileutil.ReadFile(path)
		if err != nil {
			return "", errfmt.Newf("read --debrief-notes-file").Wrap(err)
		}
		s := strings.TrimSpace(string(b))
		if len(s) > maxConvergenceDebriefNotesLen {
			return "", errfmt.Errorf("debrief notes exceed %d characters (from file)", maxConvergenceDebriefNotesLen)
		}
		return s, nil
	}
	s := strings.TrimSpace(debriefNotesFlag)
	if len(s) > maxConvergenceDebriefNotesLen {
		return "", errfmt.Errorf("debrief notes exceed %d characters", maxConvergenceDebriefNotesLen)
	}
	return s, nil
}
