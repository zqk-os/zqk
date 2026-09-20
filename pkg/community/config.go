package community

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ProfileConfig represents a configuration overlay profile.
type ProfileConfig struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Inherits    string                 `json:"inherits,omitempty"`
	Settings    map[string]interface{} `json:"settings"`
}

// ConfigOverlayEngine manages profiles and computes layered configuration overlays.
type ConfigOverlayEngine struct {
	mu       sync.RWMutex
	base     map[string]interface{}
	profiles map[string]ProfileConfig
}

// NewConfigOverlayEngine initializes an engine with optional default settings.
func NewConfigOverlayEngine(defaultSettings map[string]interface{}) *ConfigOverlayEngine {
	base := make(map[string]interface{})
	if defaultSettings != nil {
		for k, v := range defaultSettings {
			base[k] = v
		}
	}
	return &ConfigOverlayEngine{
		base:     base,
		profiles: make(map[string]ProfileConfig),
	}
}

// RegisterProfile registers or updates a configuration profile.
func (e *ConfigOverlayEngine) RegisterProfile(profile ProfileConfig) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	name := strings.ToLower(strings.TrimSpace(profile.Name))
	if name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}
	if profile.Settings == nil {
		profile.Settings = make(map[string]interface{})
	}
	profile.Name = name
	e.profiles[name] = profile
	return nil
}

// GetProfile retrieves a registered profile.
func (e *ConfigOverlayEngine) GetProfile(name string) (ProfileConfig, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	p, ok := e.profiles[strings.ToLower(strings.TrimSpace(name))]
	return p, ok
}

// ListProfiles returns names of all registered profiles.
func (e *ConfigOverlayEngine) ListProfiles() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	names := make([]string, 0, len(e.profiles))
	for name := range e.profiles {
		names = append(names, name)
	}
	return names
}

// DeepMerge recursively merges src into dst.
func DeepMerge(dst, src map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	for k, v := range dst {
		out[k] = v
	}
	for k, v := range src {
		if vMap, ok := v.(map[string]interface{}); ok {
			if dstMap, ok := out[k].(map[string]interface{}); ok {
				out[k] = DeepMerge(dstMap, vMap)
				continue
			}
		}
		out[k] = v
	}
	return out
}

// ResolveEffectiveConfig resolves the effective settings for a given profile name and env overlays.
// Resolution order: Base settings -> Inherited Profile settings -> Profile settings -> Env Overrides.
func (e *ConfigOverlayEngine) ResolveEffectiveConfig(profileName string, envPrefix string) (map[string]interface{}, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	effective := make(map[string]interface{})
	for k, v := range e.base {
		effective[k] = v
	}

	profileName = strings.ToLower(strings.TrimSpace(profileName))
	if profileName != "" {
		chain, err := e.resolveInheritanceChain(profileName)
		if err != nil {
			return nil, err
		}

		for _, pName := range chain {
			p := e.profiles[pName]
			effective = DeepMerge(effective, p.Settings)
		}
	}

	// Apply environment variable overlays if prefix specified
	if envPrefix != "" {
		effective = DeepMerge(effective, extractEnvOverrides(envPrefix))
	}

	return effective, nil
}

// resolveInheritanceChain resolves profile hierarchy preventing cycles.
func (e *ConfigOverlayEngine) resolveInheritanceChain(name string) ([]string, error) {
	var chain []string
	visited := make(map[string]bool)

	curr := name
	for curr != "" {
		if visited[curr] {
			return nil, fmt.Errorf("cyclic inheritance detected in profile %q", curr)
		}
		visited[curr] = true

		p, exists := e.profiles[curr]
		if !exists {
			return nil, fmt.Errorf("profile %q not found in registry", curr)
		}

		chain = append([]string{curr}, chain...) // prepend to evaluate parent first
		curr = strings.ToLower(strings.TrimSpace(p.Inherits))
	}

	return chain, nil
}

// extractEnvOverrides extracts key/values from OS env matching prefix (e.g. ZQK_CFG_).
func extractEnvOverrides(prefix string) map[string]interface{} {
	overrides := make(map[string]interface{})
	prefix = strings.ToUpper(strings.TrimSpace(prefix))
	if !strings.HasSuffix(prefix, "_") {
		prefix += "_"
	}

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := parts[0], parts[1]
		if strings.HasPrefix(strings.ToUpper(key), prefix) {
			cleanKey := strings.ToLower(strings.TrimPrefix(strings.ToUpper(key), prefix))
			if cleanKey != "" {
				overrides[cleanKey] = val
			}
		}
	}
	return overrides
}

// FormatConfigJSON formats configuration to pretty JSON.
func FormatConfigJSON(cfg map[string]interface{}) (string, error) {
	bytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// LoadProfileFromFile loads a profile configuration from a JSON file using fileutil.
func LoadProfileFromFile(path string) (ProfileConfig, error) {
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return ProfileConfig{}, fmt.Errorf("failed to read profile file: %w", err)
	}

	var p ProfileConfig
	if err := json.Unmarshal(data, &p); err != nil {
		return ProfileConfig{}, fmt.Errorf("invalid profile json: %w", err)
	}

	name := strings.ToLower(strings.TrimSpace(p.Name))
	if name == "" {
		return ProfileConfig{}, fmt.Errorf("profile name cannot be empty")
	}
	p.Name = name
	if p.Settings == nil {
		p.Settings = make(map[string]interface{})
	}
	return p, nil
}

// SaveProfileToFile writes a profile configuration to a JSON file using durable fileutil write.
func SaveProfileToFile(path string, profile ProfileConfig) error {
	name := strings.ToLower(strings.TrimSpace(profile.Name))
	if name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}
	profile.Name = name
	if profile.Settings == nil {
		profile.Settings = make(map[string]interface{})
	}

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal profile: %w", err)
	}

	if err := fileutil.WriteDurableStandardFile(path, data); err != nil {
		return fmt.Errorf("failed to write profile file: %w", err)
	}
	return nil
}

// LoadProfilesFromDirectory scans a directory and registers all profile JSON files into the engine.
func (e *ConfigOverlayEngine) LoadProfilesFromDirectory(dir string) (int, error) {
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("failed to read profile directory: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		fullPath := dir + "/" + entry.Name()
		p, err := LoadProfileFromFile(fullPath)
		if err != nil {
			return count, err
		}
		if err := e.RegisterProfile(p); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
