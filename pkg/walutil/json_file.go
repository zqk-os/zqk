package walutil

import (
	"encoding/json"

	"github.com/lanceman/zqk/pkg/errfmt"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// ReadJSONFile reads JSON from path into out. Missing file returns nil.
func ReadJSONFile(path string, out any) error {
	b, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(b) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return errfmt.Errorf("unmarshal %s: %w", path, err)
	}
	return nil
}

// WriteJSONFileAtomic writes JSON to path using a durable temp file with synchronous fsync before rename.
func WriteJSONFileAtomic(path string, v any, perm fileutil.FileMode) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteDurableFile(path, b, perm)
}
