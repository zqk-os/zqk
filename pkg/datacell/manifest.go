package datacell

import (
	"encoding/json"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var runtimeManifests stampmemo.Table[RuntimeManifest] // keyed by projectRoot

// RuntimeManifest is optional JSON at .zqk/agent-runtime/datacell_runtime.json (or alias override).
// When missing, consumers treat protocol version as [ProtocolVersion] constant.
type RuntimeManifest struct {
	ProtocolVersion string `json:"protocol_version"`
}

// RuntimeManifestPath returns the absolute path to the optional runtime manifest file.
func RuntimeManifestPath(projectRoot string) string {
	return paths.ResolveAgentRuntimePath(projectRoot, paths.PathAliasDatacellRuntimeManifest, paths.DataCellRuntimeManifestFile)
}

// ReadRuntimeManifest loads the manifest when present. A missing file returns (RuntimeManifest{}, nil).
// Invalid JSON returns a non-nil error.
func ReadRuntimeManifest(projectRoot string) (RuntimeManifest, error) {
	p := RuntimeManifestPath(projectRoot)
	return runtimeManifests.Load(projectRoot, stampmemo.Of(p), func() (RuntimeManifest, error) {
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
	})
}

// EffectiveProtocolVersion returns manifest.protocol_version when set, else [ProtocolVersion].
func EffectiveProtocolVersion(m RuntimeManifest) string {
	if m.ProtocolVersion != "" {
		return m.ProtocolVersion
	}
	return ProtocolVersion
}
