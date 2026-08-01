package internal

import (
	"fmt"
	"regexp"
	"strconv"

	objkeys "github.com/lanceman/zqk/pkg/objects"
)

// NextSequentialID returns the next sequential ID for a kind given existing objects.
// idPattern must have one submatch capturing the numeric part (e.g. ^IMPTRK-(\d+)$).
// format is the printf format for the new ID (e.g. "IMPTRK-%03d").
// Used by ontology import (import_tracking, domain_registry), domain register
// (domain_registry), and any command that assigns PREFIX-NNN IDs from a list.
func NextSequentialID(objects []map[string]any, idPattern *regexp.Regexp, format string) string {
	max := 0
	for _, obj := range objects {
		id, _ := obj[objkeys.FieldKeyID].(string)
		if id == emptyValue {
			continue
		}
		m := idPattern.FindStringSubmatch(id)
		if len(m) != 2 {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		if n > max {
			max = n
		}
	}
	return fmt.Sprintf(format, max+1)
}
