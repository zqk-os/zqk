package validation

import (
	"encoding/json"
	"path/filepath"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// readLiveCASBlobFromIndex loads objectID via the on-disk listing index when the
// discovery path still names a deleted hash (object-id-cache lag). Avoids
// importing pkg/storage (cycle).
//
// TRACK: REDACTED
func readLiveCASBlobFromIndex(kindDir, kind, objectID string) (path string, data []byte, ok bool) {
	if kindDir == emptyValue || kind == emptyValue || objectID == emptyValue {
		return "", nil, false
	}
	indexPath := filepath.Join(kindDir, "."+kind+".index")
	raw, err := fileutil.ReadFile(indexPath)
	if err != nil {
		return "", nil, false
	}
	var idx struct {
		Mappings   map[string]string `json:"mappings"`
		BucketKeys map[string]string `json:"bucket_keys"`
	}
	if err := json.Unmarshal(raw, &idx); err != nil || idx.Mappings == nil {
		return "", nil, false
	}
	hash := idx.Mappings[objectID]
	if hash == emptyValue {
		return "", nil, false
	}
	candidates := make([]string, 0, 3)
	if bk := idx.BucketKeys[objectID]; bk != emptyValue {
		candidates = append(candidates, filepath.Join(kindDir, bk, hash+".yaml"))
	}
	candidates = append(candidates, filepath.Join(kindDir, hash+".yaml"))
	for _, p := range candidates {
		body, rErr := fileutil.ReadFile(p)
		if rErr == nil {
			return p, body, true
		}
	}
	return "", nil, false
}
