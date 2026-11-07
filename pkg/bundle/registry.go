package bundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Registry manages installed and discovery-path language bundles.
type Registry struct {
	workspaceRoot string
	bundlesDir    string
}

// NewRegistry initializes a registry for language bundles within a workspace.
func NewRegistry(workspaceRoot string) *Registry {
	if workspaceRoot == "" {
		workspaceRoot = "."
	}
	return &Registry{
		workspaceRoot: workspaceRoot,
		bundlesDir:    filepath.Join(workspaceRoot, paths.ProjectDataDir, "bundles"),
	}
}

// ListInstalled returns all valid language bundles currently present in .zqk/bundles/.
func (r *Registry) ListInstalled() ([]*LanguageBundleManifest, error) {
	if _, err := fileutil.Stat(r.bundlesDir); fileutil.IsNotExist(err) {
		return []*LanguageBundleManifest{}, nil
	}

	entries, err := fileutil.ReadDir(r.bundlesDir)
	if err != nil {
		return nil, fmt.Errorf("read bundles directory: %w", err)
	}

	var manifests []*LanguageBundleManifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(r.bundlesDir, entry.Name(), "bundle.yaml")
		if _, err := fileutil.Stat(manifestPath); fileutil.IsNotExist(err) {
			manifestPath = filepath.Join(r.bundlesDir, entry.Name(), "bundle.json")
			if _, err := fileutil.Stat(manifestPath); fileutil.IsNotExist(err) {
				continue
			}
		}

		m, err := LoadManifestFile(manifestPath)
		if err != nil {
			// Skip or log corrupted packs
			continue
		}
		manifests = append(manifests, m)
	}

	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].Name < manifests[j].Name
	})

	return manifests, nil
}

// Get installed bundle by name.
func (r *Registry) Get(name string) (*LanguageBundleManifest, error) {
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return nil, fmt.Errorf("invalid bundle name: %q", name)
	}
	installed, err := r.ListInstalled()
	if err != nil {
		return nil, err
	}
	for _, b := range installed {
		if strings.EqualFold(b.Name, name) {
			return b, nil
		}
	}
	return nil, fmt.Errorf("bundle %q not found", name)
}

// Install copies and installs a manifest and its associated policies into .zqk/bundles/<name>/.
func (r *Registry) Install(m *LanguageBundleManifest) error {
	if err := m.Validate(); err != nil {
		return fmt.Errorf("cannot install invalid bundle: %w", err)
	}

	targetDir := filepath.Join(r.bundlesDir, m.Name)
	if err := fileutil.MkdirAll(targetDir, paths.DirPerm755); err != nil {
		return fmt.Errorf("create bundle directory: %w", err)
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	manifestFile := filepath.Join(targetDir, "bundle.json")
	if err := fileutil.WriteFile(manifestFile, data, paths.FilePerm644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	// Seed agent directives into the workspace if specified
	for _, d := range m.AgentDirectives {
		if d.Path != "" {
			destPath := filepath.Join(r.workspaceRoot, d.Path)
			if err := fileutil.MkdirAll(filepath.Dir(destPath), paths.DirPerm755); err != nil {
				return fmt.Errorf("create directive dir: %w", err)
			}
			content := fmt.Sprintf("# Directive for %s\n%s\n", d.TargetRole, d.Directive)
			if err := fileutil.WriteFile(destPath, []byte(content), paths.FilePerm644); err != nil {
				return fmt.Errorf("write directive file: %w", err)
			}
		}
	}

	return nil
}

// Remove uninstalls a bundle from .zqk/bundles/.
func (r *Registry) Remove(name string) error {
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return errors.New("invalid bundle name")
	}
	targetDir := filepath.Join(r.bundlesDir, name)
	if _, err := fileutil.Stat(targetDir); fileutil.IsNotExist(err) {
		return fmt.Errorf("bundle %q is not installed", name)
	}
	return fileutil.RemoveAll(targetDir)
}
