package system

import "strings"

type casCorruptionTSVRow struct {
	Kind     string
	ObjectID string
	FilePath string
	Message  string // preserved for potential diagnostics; ignored by the repair loop
}

func isRepairCASCorruptionTSVHeader(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "object_kind\t")
}

func parseRepairCASCorruptionTSVLine(line string) (casCorruptionTSVRow, bool) {
	line = strings.TrimSpace(line)
	if line == emptyValue {
		return casCorruptionTSVRow{}, false
	}

	// object_kind <tab> object_id <tab> file_path <tab> message
	parts := strings.SplitN(line, "\t", 4)
	if len(parts) < 3 {
		return casCorruptionTSVRow{}, false
	}

	row := casCorruptionTSVRow{
		Kind:     strings.TrimSpace(parts[0]),
		ObjectID: strings.TrimSpace(parts[1]),
		FilePath: strings.TrimSpace(parts[2]),
	}
	if row.Kind == emptyValue || row.ObjectID == emptyValue || row.FilePath == emptyValue {
		return casCorruptionTSVRow{}, false
	}

	if len(parts) == 4 {
		row.Message = parts[3]
	}

	return row, true
}
