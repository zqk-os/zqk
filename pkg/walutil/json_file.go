package walutil

import (
	"encoding/json"
	"os"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// ReadJSONFile reads JSON from path into out. Missing file returns nil.
func ReadJSONFile(path string, out any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
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

// WriteJSONFileAtomic writes JSON to path using a temp file then rename.
func WriteJSONFileAtomic(path string, v any, perm os.FileMode) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
