package validation

import (
	"strconv"

	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const globalConfigKey = "global"

var (
	// idPrefixConfigs is keyed "global" (one process config). Stamp is the YAML file.
	idPrefixConfigs stampmemo.Table[*IDPrefixesConfig]
	// namespaceConfigs is keyed "global". Stamp is the YAML file.
	namespaceConfigs stampmemo.Table[*NamespacesConfig]
	// pathsConfigs is keyed "global". Stamp is the YAML file.
	pathsConfigs stampmemo.Table[*PathsConfig]
	// discoveredPaths is keyed by cwd+relative+isDir (cwd × closed path keys).
	discoveredPaths stampmemo.Table[string]
	// resolvedFindPaths is keyed by cwd+pathKey. Stamp is cwd.
	resolvedFindPaths stampmemo.Table[string]
)

func resetDiscoveredPaths() {
	discoveredPaths.Reset()
	resolvedFindPaths.Reset()
}

func rememberDiscoveredPath(relativePath string, isDir bool, loadFn func() string) string {
	wd, err := fileutil.Getwd()
	if err != nil {
		return loadFn()
	}
	key := wd + "\x00" + relativePath + "\x00" + strconv.FormatBool(isDir)
	path, _ := discoveredPaths.Load(key, stampmemo.Of(wd), func() (string, error) {
		return loadFn(), nil
	})
	return path
}
