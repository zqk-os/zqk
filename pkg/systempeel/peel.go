package systempeel

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// LargeFileReport describes a source file exceeding a lines-of-code threshold.
type LargeFileReport struct {
	RelativePath string `json:"relative_path"`
	LinesOfCode  int    `json:"lines_of_code"`
	BaseName     string `json:"base_name"`
}

// PeeledCommandMetadata captures the modularity metrics of a peeled system command.
type PeeledCommandMetadata struct {
	Name        string            `json:"name"`
	PackagePath string            `json:"package_path"`
	LinesOfCode int               `json:"lines_of_code"`
	Subcommands []string          `json:"subcommands,omitempty"`
	LargeFiles  []LargeFileReport `json:"large_files,omitempty"`
}

// SystemInspector provides structural inspection into cmd/zqk/system modularity.
type SystemInspector struct {
	RootDir string
}

// NewSystemInspector creates a new SystemInspector rooted at rootDir.
func NewSystemInspector(rootDir string) *SystemInspector {
	return &SystemInspector{RootDir: rootDir}
}

// InspectSystemPackage scans a system package directory and calculates file counts and LOC.
func (si *SystemInspector) InspectSystemPackage(pkgRelPath string) (*PeeledCommandMetadata, error) {
	fullPath := filepath.Join(si.RootDir, pkgRelPath)
	entries, err := fileutil.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read package dir %s: %w", fullPath, err)
	}

	totalLOC := 0
	var subcommands []string
	var largeFiles []LargeFileReport

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		filePath := filepath.Join(fullPath, entry.Name())
		content, err := fileutil.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
		}
		lines := strings.Split(string(content), "\n")
		loc := len(lines)
		totalLOC += loc
		base := strings.TrimSuffix(entry.Name(), ".go")
		subcommands = append(subcommands, base)

		if loc > 1000 {
			largeFiles = append(largeFiles, LargeFileReport{
				RelativePath: filepath.Join(pkgRelPath, entry.Name()),
				LinesOfCode:  loc,
				BaseName:     entry.Name(),
			})
		}
	}

	sort.Slice(largeFiles, func(i, j int) bool {
		return largeFiles[i].LinesOfCode > largeFiles[j].LinesOfCode
	})

	return &PeeledCommandMetadata{
		Name:        filepath.Base(pkgRelPath),
		PackagePath: pkgRelPath,
		LinesOfCode: totalLOC,
		Subcommands: subcommands,
		LargeFiles:  largeFiles,
	}, nil
}

// FindFilesExceedingThreshold finds all Go files in pkgRelPath with LOC > maxLOC.
func (si *SystemInspector) FindFilesExceedingThreshold(pkgRelPath string, maxLOC int) ([]LargeFileReport, error) {
	fullPath := filepath.Join(si.RootDir, pkgRelPath)
	entries, err := fileutil.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read package dir %s: %w", fullPath, err)
	}

	var results []LargeFileReport
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		filePath := filepath.Join(fullPath, entry.Name())
		content, err := fileutil.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
		}
		lines := strings.Split(string(content), "\n")
		loc := len(lines)
		if loc > maxLOC {
			results = append(results, LargeFileReport{
				RelativePath: filepath.Join(pkgRelPath, entry.Name()),
				LinesOfCode:  loc,
				BaseName:     entry.Name(),
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].LinesOfCode > results[j].LinesOfCode
	})

	return results, nil
}

// PeeledComponentRegistry maintains registered modular system components.
type PeeledComponentRegistry struct {
	mu         sync.RWMutex
	components map[string]*PeeledCommandMetadata
}

// Global registry instance.
var globalRegistry = &PeeledComponentRegistry{
	components: make(map[string]*PeeledCommandMetadata),
}

// GetGlobalRegistry returns the shared component registry.
func GetGlobalRegistry() *PeeledComponentRegistry {
	return globalRegistry
}

// Register registers metadata for a peeled component.
func (r *PeeledComponentRegistry) Register(name string, meta *PeeledCommandMetadata) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.components[name] = meta
}

// Get retrieves metadata for a peeled component.
func (r *PeeledComponentRegistry) Get(name string) (*PeeledCommandMetadata, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	meta, ok := r.components[name]
	return meta, ok
}

// List returns all registered components sorted by name.
func (r *PeeledComponentRegistry) List() []*PeeledCommandMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []*PeeledCommandMetadata
	for _, comp := range r.components {
		list = append(list, comp)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return list
}

// AttachPeeledCommand binds a dynamically peeled command to a parent Cobra command tree.
func AttachPeeledCommand(parent *cobra.Command, cmd *cobra.Command) {
	if parent != nil && cmd != nil {
		parent.AddCommand(cmd)
	}
}
