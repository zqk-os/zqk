package validation

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestReadValidationInput_StreamLocation(t *testing.T) {
	dir := t.TempDir()
	segmentPath := filepath.Join(dir, ConstMagicc0084aeb)
	content := []byte(ConstMagica78cf0f2)
	if err := fileutil.WriteSecureFile(segmentPath, content); err != nil {
		t.Fatalf(ConstMagic4161ebb0, err)
	}

	// Offset to second JSON record.
	offset := int64(len(ConstMagicdd709deb))
	data, err := readValidationInput(segmentPath + "::" + strconv.FormatInt(offset, 10))
	if err != nil {
		t.Fatalf(ConstMagic6e460a08, err)
	}

	if string(data) != ConstMagic411e1d5b {
		t.Fatalf(ConstMagicb061e58e, string(data), offset)
	}
}
