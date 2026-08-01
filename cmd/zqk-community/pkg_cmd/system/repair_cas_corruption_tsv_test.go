package system

import "testing"

func TestIsRepairCASCorruptionTSVHeader(t *testing.T) {
	if !isRepairCASCorruptionTSVHeader("object_kind\tobject_id\tfile_path\tmessage") {
		t.Fatalf("expected header match")
	}
	if !isRepairCASCorruptionTSVHeader("  object_kind\tobject_id\tfile_path\tmessage  ") {
		t.Fatalf("expected header match with whitespace")
	}
	if isRepairCASCorruptionTSVHeader("role\tOBJ-1\t/path/to/file\tmsg") {
		t.Fatalf("did not expect header match for a data line")
	}
}

func TestParseRepairCASCorruptionTSVLine(t *testing.T) {
	row, ok := parseRepairCASCorruptionTSVLine("role\tOBJ-1\t/abs/path/file\tbad hash\tmore")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if row.Kind != "role" || row.ObjectID != "OBJ-1" || row.FilePath != "/abs/path/file" {
		t.Fatalf("unexpected row: %+v", row)
	}

	_, ok = parseRepairCASCorruptionTSVLine("role\tOBJ-1") // not enough columns
	if ok {
		t.Fatalf("expected ok=false for invalid column count")
	}

	_, ok = parseRepairCASCorruptionTSVLine("role\t\t/abs/path/file\tmsg") // missing object id
	if ok {
		t.Fatalf("expected ok=false for missing field")
	}

	_, ok = parseRepairCASCorruptionTSVLine("") // empty line
	if ok {
		t.Fatalf("expected ok=false for empty line")
	}
}
