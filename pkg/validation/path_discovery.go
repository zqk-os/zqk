package validation

import (
	"strconv"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var (
	// idPrefixConfigs is keyed by id_prefixes_config.yaml path (closed set).
	idPrefixConfigs stampmemo.Table[*IDPrefixesConfig]
	// namespaceConfigs is keyed by namespaces_config.yaml path (closed set).
	namespaceConfigs stampmemo.Table[*NamespacesConfig]
	// pathsConfigs is keyed by paths_config.yaml path (closed set).
	pathsConfigs stampmemo.Table[*PathsConfig]
	// discoveredPaths is keyed by cwd+relative+isDir (cwd × closed path keys).
	discoveredPaths stampmemo.Table[string]
	// resolvedFindPaths is keyed by cwd+pathKey. Stamp is cwd.
	resolvedFindPaths stampmemo.Table[string]
	// timeoutConfigs is keyed by config/zqk.yaml path (closed set).
	timeoutConfigs stampmemo.Table[*ValidationTimeoutConfig]
	// tierConfigs is keyed by config/zqk.yaml path (closed set).
	tierConfigs stampmemo.Table[*ValidationTierConfig]
)

func resetDiscoveredPaths() {
	discoveredPaths.Reset()
	resolvedFindPaths.Reset()
	timeoutConfigs.Reset()
	tierConfigs.Reset()
	paths.ResetCwdDiscovery()
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
