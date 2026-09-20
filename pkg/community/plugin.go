package community

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var validPluginNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// PluginManifest describes an offline community plugin package.
type PluginManifest struct {
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	Description      string   `json:"description,omitempty"`
	Author           string   `json:"author,omitempty"`
	Entrypoint       string   `json:"entrypoint"`
	MinKernelVersion string   `json:"min_kernel_version,omitempty"`
	ChecksumSHA256   string   `json:"checksum_sha256,omitempty"`
	Capabilities     []string `json:"capabilities,omitempty"`
}

// PluginManager manages offline plugin manifests and integrity verification.
type PluginManager struct {
	mu        sync.RWMutex
	pluginDir string
	registry  map[string]PluginManifest
}

// NewPluginManager creates a new plugin manager rooted at pluginDir.
func NewPluginManager(pluginDir string) *PluginManager {
	return &PluginManager{
		pluginDir: pluginDir,
		registry:  make(map[string]PluginManifest),
	}
}

// ValidateManifest validates plugin metadata, naming, and required fields.
func ValidateManifest(m PluginManifest) error {
	trimmedName := strings.TrimSpace(m.Name)
	if trimmedName == "" {
		return fmt.Errorf("plugin name cannot be empty")
	}
	if !validPluginNameRegex.MatchString(trimmedName) {
		return fmt.Errorf("invalid plugin name %q: must contain only letters, numbers, hyphens, and underscores", trimmedName)
	}

	trimmedVer := strings.TrimSpace(m.Version)
	if trimmedVer == "" {
		return fmt.Errorf("plugin version cannot be empty")
	}

	trimmedEntry := strings.TrimSpace(m.Entrypoint)
	if trimmedEntry == "" {
		return fmt.Errorf("plugin entrypoint cannot be empty")
	}

	// Security: entrypoint must be a relative clean path without parent directory traversal
	cleanEntry := filepath.Clean(trimmedEntry)
	if filepath.IsAbs(cleanEntry) || strings.HasPrefix(cleanEntry, "..") {
		return fmt.Errorf("insecure entrypoint %q: path traversal or absolute path not allowed", trimmedEntry)
	}

	return nil
}

// LoadManifestFile loads and validates a plugin manifest JSON using fileutil.
func LoadManifestFile(path string) (PluginManifest, error) {
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return PluginManifest{}, fmt.Errorf("failed to read plugin manifest: %w", err)
	}

	var m PluginManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return PluginManifest{}, fmt.Errorf("invalid plugin manifest json: %w", err)
	}

	if err := ValidateManifest(m); err != nil {
		return PluginManifest{}, err
	}

	return m, nil
}

// SaveManifestFile securely writes a plugin manifest JSON using fileutil.
func SaveManifestFile(path string, m PluginManifest) error {
	if err := ValidateManifest(m); err != nil {
		return err
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal plugin manifest: %w", err)
	}

	if err := fileutil.WriteDurableStandardFile(path, data); err != nil {
		return fmt.Errorf("failed to write plugin manifest: %w", err)
	}
	return nil
}

// VerifyPluginChecksum computes SHA256 of file at path and matches with expected.
func VerifyPluginChecksum(path string, expectedSHA string) error {
	expectedSHA = strings.TrimSpace(strings.ToLower(expectedSHA))
	if expectedSHA == "" {
		return fmt.Errorf("expected checksum cannot be empty")
	}

	data, err := fileutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file for checksum verification: %w", err)
	}

	sum := sha256.Sum256(data)
	actualSHA := hex.EncodeToString(sum[:])

	if actualSHA != expectedSHA {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedSHA, actualSHA)
	}

	return nil
}

// RegisterPlugin validates and registers a plugin in the manager.
func (pm *PluginManager) RegisterPlugin(m PluginManifest) error {
	if err := ValidateManifest(m); err != nil {
		return err
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	nameKey := strings.ToLower(strings.TrimSpace(m.Name))
	pm.registry[nameKey] = m
	return nil
}

// GetPlugin retrieves a registered plugin by name.
func (pm *PluginManager) GetPlugin(name string) (PluginManifest, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	m, ok := pm.registry[strings.ToLower(strings.TrimSpace(name))]
	return m, ok
}

// ListPlugins returns all registered plugins.
func (pm *PluginManager) ListPlugins() []PluginManifest {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	list := make([]PluginManifest, 0, len(pm.registry))
	for _, m := range pm.registry {
		list = append(list, m)
	}
	return list
}

// ScanPluginsDirectory reads a directory of plugin manifest files into the manager.
func (pm *PluginManager) ScanPluginsDirectory(dir string) (int, error) {
	cleanDir := filepath.Clean(dir)
	if !fileutil.Exists(cleanDir) {
		return 0, fmt.Errorf("plugin directory does not exist: %s", cleanDir)
	}

	entries, err := fileutil.ReadDir(cleanDir)
	if err != nil {
		return 0, fmt.Errorf("failed to read plugin directory: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		fullPath := filepath.Join(cleanDir, entry.Name())
		m, err := LoadManifestFile(fullPath)
		if err != nil {
			return count, fmt.Errorf("error reading %s: %w", entry.Name(), err)
		}

		if err := pm.RegisterPlugin(m); err != nil {
			return count, err
		}
		count++
	}

	return count, nil
}
