package filecas

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// ensureExactlyOneLiveCASBlob deletes every live hash YAML for objectID except
// keeperHash.yaml, then fails if a second blob is still on disk.
// Call after a successful CAS write, before Update returns.
// TRACK: BLI-CEF-R19-CAS-INTERMEDIATE-LEAK-001 — write-path prevention; detection alone is not enough.
func ensureExactlyOneLiveCASBlob(objectID, keeperHash, kindDir string) error {
	if objectID == emptyValue || keeperHash == emptyValue || kindDir == emptyValue {
		return nil
	}
	keeperBase := keeperHash + ".yaml"
	for _, p := range listLiveCASBlobsForObjectID(objectID, kindDir) {
		if filepath.Base(p) == keeperBase {
			continue
		}
		if err := RemoveOrphanCASHashFileSync(p); err != nil {
			return err
		}
	}
	var extras []string
	for _, p := range listLiveCASBlobsForObjectID(objectID, kindDir) {
		if filepath.Base(p) != keeperBase {
			extras = append(extras, p)
		}
	}
	if len(extras) > 0 {
		return errfmt.Errorf("CAS update left %d extra blob(s) for %s (keeper %s): %s", len(extras), objectID, keeperHash, strings.Join(extras, ", "))
	}
	return nil
}

func listLiveCASBlobsForObjectID(objectID, kindDir string) []string {
	if objectID == emptyValue || kindDir == emptyValue {
		return nil
	}
	var out []string
	scan := func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !CasHashFilenameRe.MatchString(name) {
				continue
			}
			p := filepath.Join(dir, name)
			if CasHashFilePeekObjectID(p) == objectID {
				out = append(out, p)
			}
		}
	}
	scan(kindDir)
	entries, err := os.ReadDir(kindDir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() {
			scan(filepath.Join(kindDir, e.Name()))
		}
	}
	return out
}
