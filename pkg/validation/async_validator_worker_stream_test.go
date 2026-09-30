package validation

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestReadValidationInput_StreamLocation(t *testing.T) {
	dir := t.TempDir()
	segmentPath := filepath.Join(dir, "2026-03-20_stream.json")
	content := []byte("{\"id\":\"A\"}\n{\"id\":\"B\",\"kind\":\"audit_event\"}\n")
	if err := fileutil.WriteSecureFile(segmentPath, content); err != nil {
		t.Fatalf("write segment: %v", err)
	}

	// Offset to second JSON record.
	offset := int64(len("{\"id\":\"A\"}\n"))
	data, err := readValidationInput(segmentPath + "::" + strconv.FormatInt(offset, 10))
	if err != nil {
		t.Fatalf("readValidationInput error: %v", err)
	}

	if string(data) != "{\"id\":\"B\",\"kind\":\"audit_event\"}" {
		t.Fatalf("unexpected record payload: %s (offset=%d)", string(data), offset)
	}
}
