package datacell

import (
	"encoding/json"
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// RuntimeManifest is optional JSON at .zqk/config/datacell_runtime.json (or alias override).
// When missing, consumers treat protocol version as [ProtocolVersion] constant.
type RuntimeManifest struct {
	ProtocolVersion string `json:"protocol_version"`
}

// RuntimeManifestPath returns the absolute path to the optional runtime manifest file.
func RuntimeManifestPath(projectRoot string) string {
	fallback := filepath.Join(paths.ProjectDataDir, paths.ConfigDir, paths.DataCellRuntimeManifestFile)
	return paths.ResolvePathFromCacheOrConstant(projectRoot, paths.PathAliasDatacellRuntimeManifest, fallback)
}

// ReadRuntimeManifest loads the manifest when present. A missing file returns (RuntimeManifest{}, nil).
// Invalid JSON returns a non-nil error.
func ReadRuntimeManifest(projectRoot string) (RuntimeManifest, error) {
	p := RuntimeManifestPath(projectRoot)
	b, err := fileutil.ReadFile(p)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return RuntimeManifest{}, nil
		}
		return RuntimeManifest{}, err
	}
	var m RuntimeManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return RuntimeManifest{}, err
	}
	return m, nil
}

// EffectiveProtocolVersion returns manifest.protocol_version when set, else [ProtocolVersion].
func EffectiveProtocolVersion(m RuntimeManifest) string {
	if m.ProtocolVersion != "" {
		return m.ProtocolVersion
	}
	return ProtocolVersion
}
