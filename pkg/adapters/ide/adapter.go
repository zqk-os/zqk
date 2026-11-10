package ide

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// Adapter encapsulates vendor-specific configuration detection, workspace bindings,
// and transport characteristics for an IDE or host agent.
type Adapter interface {
	// Name returns the human-readable display name of the IDE/tool adapter.
	Name() string

	// Vendor returns the canonical vendor identifier (e.g. "cursor", "anthropic", "codeium", "google").
	Vendor() string

	// WorkspaceConfig returns the display label and path for a project workspace MCP configuration,
	// or empty strings if the vendor does not use workspace-local configuration.
	WorkspaceConfig(projectRoot string) (displayName string, configPath string)

	// GlobalConfigs returns any host-global configuration paths mapped to their display labels.
	GlobalConfigs(homeDir string) map[string]string

	// SearchDirs returns filesystem directories to search for existing configurations on the host.
	SearchDirs(homeDir, goos, appData string) []string

	// KnownFiles returns a mapping of filenames to their display identifiers for file discovery.
	KnownFiles() map[string]string

	// IsForeign reports whether a configuration path or IDE name corresponds to a third-party
	// foreign format that should not be warn-flooded or rewritten as a native ZQK MCP server.
	IsForeign(ideName, configPath string) bool

	// IsStdioAdapter reports whether the configuration requires launching the ide-adapter
	// daemon bridge over stdio rather than a standalone fat MCP server.
	IsStdioAdapter(ideName, configPath string) bool
}

var (
	registryMu sync.RWMutex
	adapters   []Adapter
)

// Register registers an IDE adapter in the global registry.
func Register(a Adapter) {
	if a == nil {
		return
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	for _, existing := range adapters {
		if existing.Vendor() == a.Vendor() && existing.Name() == a.Name() {
			return
		}
	}
	adapters = append(adapters, a)
}

// All returns a slice of all registered IDE adapters.
func All() []Adapter {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Adapter, len(adapters))
	copy(out, adapters)
	return out
}

// FindConfigs discovers all applicable MCP configurations for the given home directory
// and project root using all registered adapters.
func FindConfigs(homeDir, projectRoot string) map[string]string {
	configs := make(map[string]string)
	allAdapters := All()

	// 1. Collect workspace configs
	if projectRoot != "" {
		for _, a := range allAdapters {
			if label, path := a.WorkspaceConfig(projectRoot); label != "" && path != "" {
				configs[label] = path
			}
		}
	}

	if homeDir == "" {
		return configs
	}

	// 2. Collect explicit global configs
	for _, a := range allAdapters {
		for label, path := range a.GlobalConfigs(homeDir) {
			if label != "" && path != "" {
				configs[label] = path
			}
		}
	}

	// 3. Search directories for known files
	goos := runtime.GOOS
	appData := zqkenv.OSAppData().Get()

	var searchDirs []string
	knownFiles := make(map[string]string)

	for _, a := range allAdapters {
		searchDirs = append(searchDirs, a.SearchDirs(homeDir, goos, appData)...)
		for filename, label := range a.KnownFiles() {
			knownFiles[filename] = label
		}
	}

	already := make(map[string]struct{}, len(configs))
	for _, p := range configs {
		already[filepath.Clean(p)] = struct{}{}
	}

	matches := fileutil.FindFiles(searchDirs, knownFiles)
	for _, match := range matches {
		clean := filepath.Clean(match.Path)
		if _, dup := already[clean]; dup {
			continue
		}
		// Prevent overwriting IDE workspace config if discovered during walk
		if projectRoot != "" && clean == filepath.Clean(filepath.Join(projectRoot, ".ide", "mcp.json")) {
			continue
		}
		configs[match.Identifier+" ("+match.Path+")"] = match.Path
		already[clean] = struct{}{}
	}

	return configs
}

// IsForeignConfig reports whether a configuration is owned by a foreign vendor format.
func IsForeignConfig(ideName, configPath string) bool {
	for _, a := range All() {
		if a.IsForeign(ideName, configPath) {
			return true
		}
	}
	return false
}

// IsIDEStdioAdapterConfig reports whether an IDE configuration requires stdio daemon bridging.
func IsIDEStdioAdapterConfig(ideName, configPath string) bool {
	for _, a := range All() {
		if a.IsStdioAdapter(ideName, configPath) {
			return true
		}
	}
	return false
}

// IsWorkspaceConfig reports whether a configuration path belongs inside a project workspace.
func IsWorkspaceConfig(ideName, projectRoot, configPath string) bool {
	if projectRoot == "" {
		return false
	}
	if strings.Contains(ideName, "Desktop") || strings.Contains(ideName, "Global") {
		return false
	}
	cleanRoot := filepath.Clean(projectRoot)
	cleanPath := filepath.Clean(configPath)
	return strings.HasPrefix(cleanPath, cleanRoot+string(filepath.Separator)) || cleanPath == cleanRoot
}
