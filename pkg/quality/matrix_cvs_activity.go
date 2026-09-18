package quality

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// BuildMatrixActivityLogEntry builds one convergence_session activity_log entry for a matrix update.
func BuildMatrixActivityLogEntry(matrixAlias, csvPath string, bulk bool, updatedCount int, updates map[string]string) map[string]any {
	csvPath = filepath.Clean(csvPath)
	base := filepath.Base(csvPath)
	var notes string
	if bulk {
		notes = fmt.Sprintf("matrix update %q (%s): %d row(s) with fields %s (zqk matrix update).",
			matrixAlias, base, updatedCount, summarizeUpdateKeys(updates))
	} else {
		notes = fmt.Sprintf("matrix update %q (%s): %s (zqk matrix update).",
			matrixAlias, base, summarizeUpdateKeys(updates))
	}
	return map[string]any{
		"timestamp":           zqktime.NowRFC3339UTC(),
		objects.FieldKeyPhase: "",
		"action":              "matrix_update",
		objects.FieldKeyNotes: notes,
	}
}

func summarizeUpdateKeys(updates map[string]string) string {
	if len(updates) == 0 {
		return "(none)"
	}
	keys := make([]string, 0, len(updates))
	for k := range updates {
		keys = append(keys, strings.TrimSpace(k))
	}
	slices.Sort(keys)
	return strings.Join(keys, ", ")
}
