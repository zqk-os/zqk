package bootstrap

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/appledouble"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/zqktime"
	"gopkg.in/yaml.v3"
)

// BootstrapCapture represents a complete snapshot of all system specs
type BootstrapCapture struct {
	Version     string            `yaml:"version"`
	ObjectSpecs map[string][]byte `yaml:"-"` // filename -> content
	Lifecycles  map[string][]byte `yaml:"-"` // filename -> content
	Profiles    map[string][]byte `yaml:"-"` // filename -> content
	Configs     map[string][]byte `yaml:"-"` // filename -> content
	Traits      map[string][]byte `yaml:"-"` // filename -> content
	Metadata    BootstrapMetadata `yaml:"metadata"`
}

// BootstrapMetadata contains metadata about the bootstrap capture
type BootstrapMetadata struct {
	CapturedAt      string              `yaml:"captured_at"`
	SourceDirectory string              `yaml:"source_directory"`
	FileCounts      map[string]int      `yaml:"file_counts"`
	FileList        map[string][]string `yaml:"file_list"`
}

// CaptureCurrentState captures the current state of all system specs
func CaptureCurrentState(sourceDir string) (*BootstrapCapture, error) {
	capture := &BootstrapCapture{
		Version:     BootstrapCaptureFormatVersion,
		ObjectSpecs: make(map[string][]byte),
		Lifecycles:  make(map[string][]byte),
		Profiles:    make(map[string][]byte),
		Configs:     make(map[string][]byte),
		Traits:      make(map[string][]byte),
		Metadata: BootstrapMetadata{
			FileCounts: make(map[string]int),
			FileList:   make(map[string][]string),
		},
	}

	// Capture object specs
	specsDir := filepath.Join(sourceDir, "object_specs")
	if err := captureDirectory(specsDir, capture.ObjectSpecs, "object_specs"); err != nil {
		return nil, errfmt.Newf("failed to capture object specs").Wrap(err)
	}
	capture.Metadata.FileCounts["object_specs"] = len(capture.ObjectSpecs)

	// Capture lifecycles
	lifecyclesDir := filepath.Join(sourceDir, "lifecycles")
	if err := captureDirectory(lifecyclesDir, capture.Lifecycles, "lifecycles"); err != nil {
		return nil, errfmt.Newf("failed to capture lifecycles").Wrap(err)
	}
	capture.Metadata.FileCounts["lifecycles"] = len(capture.Lifecycles)

	// Capture profiles
	profilesDir := filepath.Join(sourceDir, "profile_specs")
	if err := captureDirectory(profilesDir, capture.Profiles, "profile_specs"); err != nil {
		return nil, errfmt.Newf("failed to capture profiles").Wrap(err)
	}
	capture.Metadata.FileCounts["profile_specs"] = len(capture.Profiles)

	// Capture config files (top-level YAML files)
	if err := captureConfigFiles(sourceDir, capture.Configs); err != nil {
		return nil, errfmt.Newf("failed to capture configs").Wrap(err)
	}
	capture.Metadata.FileCounts["configs"] = len(capture.Configs)

	// Capture traits
	traitsDir := filepath.Join(sourceDir, "traits")
	if err := captureDirectory(traitsDir, capture.Traits, "traits"); err != nil {
		return nil, errfmt.Newf("failed to capture traits").Wrap(err)
	}
	capture.Metadata.FileCounts["traits"] = len(capture.Traits)

	// Set metadata
	capture.Metadata.SourceDirectory = sourceDir
	capture.Metadata.CapturedAt = zqktime.NowRFC3339UTC()
	capture.Metadata.FileList["object_specs"] = getFileList(capture.ObjectSpecs)
	capture.Metadata.FileList["lifecycles"] = getFileList(capture.Lifecycles)
	capture.Metadata.FileList["profiles"] = getFileList(capture.Profiles)
	capture.Metadata.FileList["configs"] = getFileList(capture.Configs)
	capture.Metadata.FileList["traits"] = getFileList(capture.Traits)

	return capture, nil
}

// encodeBase64 encodes bytes to base64 string
func encodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// decodeBase64 decodes base64 string to bytes
func decodeBase64(encoded string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(encoded)
}

// captureDirectory captures all YAML files from a directory
func captureDirectory(dirPath string, target map[string][]byte, _ string) error {
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		// Directory doesn't exist, that's okay
		return nil
	}

	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}

		// Only capture YAML files
		if !isYAMLFile(path) {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return errfmt.Errorf("failed to read file %s: %w", path, err)
		}

		// Use relative path as key
		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return errfmt.Newf("failed to get relative path").Wrap(err)
		}

		// For root-level files, use just the filename
		if relPath == filepath.Base(path) {
			target[info.Name()] = content
		} else {
			// For subdirectory files, use relative path
			target[relPath] = content
		}

		return nil
	})
}

// captureConfigFiles captures top-level config YAML files
func captureConfigFiles(sourceDir string, target map[string][]byte) error {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return errfmt.Newf("failed to read source directory").Wrap(err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if appledouble.SkipNameInReadDir(entry.Name()) {
			continue
		}

		if !isYAMLFile(entry.Name()) {
			continue
		}

		// Skip if it's a metadata file (we'll handle that separately)
		if entry.Name() == "bootstrap.yaml" || entry.Name() == "bootstrap.yml" {
			continue
		}

		filePath := filepath.Join(sourceDir, entry.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			return errfmt.Newf("failed to read config file %s", entry.Name()).Wrap(err)
		}

		target[entry.Name()] = content
	}

	return nil
}

// isYAMLFile checks if a file is a YAML file
func isYAMLFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".yaml" || ext == ".yml"
}

// getFileList returns a sorted list of file keys
func getFileList(files map[string][]byte) []string {
	list := make([]string, 0, len(files))
	for k := range files {
		list = append(list, k)
	}
	return list
}

// SaveToFile saves the bootstrap capture to a YAML file
func (bc *BootstrapCapture) SaveToFile(filePath string) error {
	// Create a serializable structure
	type SerializableCapture struct {
		Version  string            `yaml:"version"`
		Metadata BootstrapMetadata `yaml:"metadata"`
		Files    map[string]string `yaml:"files"` // filename -> base64 encoded content
	}

	// Convert binary content to base64 strings
	files := make(map[string]string)

	// Add object specs with prefix
	for k, v := range bc.ObjectSpecs {
		files["object_specs/"+k] = encodeBase64(v)
	}

	// Add lifecycles with prefix
	for k, v := range bc.Lifecycles {
		files["lifecycles/"+k] = encodeBase64(v)
	}

	// Add profiles with prefix
	for k, v := range bc.Profiles {
		files["profile_specs/"+k] = encodeBase64(v)
	}

	// Add configs with prefix
	for k, v := range bc.Configs {
		files["configs/"+k] = encodeBase64(v)
	}

	// Add traits with prefix
	for k, v := range bc.Traits {
		files["traits/"+k] = encodeBase64(v)
	}

	serializable := SerializableCapture{
		Version:  bc.Version,
		Metadata: bc.Metadata,
		Files:    files,
	}

	data, err := yaml.Marshal(serializable)
	if err != nil {
		return errfmt.Newf("failed to marshal bootstrap capture").Wrap(err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create directory").Wrap(err)
	}

	if err := os.WriteFile(filePath, data, paths.FilePerm644); err != nil {
		return errfmt.Newf("failed to write bootstrap file").Wrap(err)
	}

	return nil
}

// LoadFromFile loads a bootstrap capture from a YAML file
func LoadFromFile(filePath string) (*BootstrapCapture, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errfmt.Newf("failed to read bootstrap file").Wrap(err)
	}

	type SerializableCapture struct {
		Version  string            `yaml:"version"`
		Metadata BootstrapMetadata `yaml:"metadata"`
		Files    map[string]string `yaml:"files"` // filename -> base64 encoded content
	}

	var serializable SerializableCapture
	if err := yaml.Unmarshal(data, &serializable); err != nil {
		return nil, errfmt.Newf("failed to unmarshal bootstrap file").Wrap(err)
	}

	capture := &BootstrapCapture{
		Version:     serializable.Version,
		Metadata:    serializable.Metadata,
		ObjectSpecs: make(map[string][]byte),
		Lifecycles:  make(map[string][]byte),
		Profiles:    make(map[string][]byte),
		Configs:     make(map[string][]byte),
		Traits:      make(map[string][]byte),
	}

	// Decode files and categorize them
	for path, encoded := range serializable.Files {
		content, err := decodeBase64(encoded)
		if err != nil {
			return nil, errfmt.Errorf("failed to decode file %s: %w", path, err)
		}

		// Categorize based on prefix
		if strings.HasPrefix(path, "object_specs/") {
			filename := strings.TrimPrefix(path, "object_specs/")
			capture.ObjectSpecs[filename] = content
		} else if strings.HasPrefix(path, "lifecycles/") {
			filename := strings.TrimPrefix(path, "lifecycles/")
			capture.Lifecycles[filename] = content
		} else if strings.HasPrefix(path, "profile_specs/") {
			filename := strings.TrimPrefix(path, "profile_specs/")
			capture.Profiles[filename] = content
		} else if strings.HasPrefix(path, "configs/") {
			filename := strings.TrimPrefix(path, "configs/")
			capture.Configs[filename] = content
		} else if strings.HasPrefix(path, "traits/") {
			filename := strings.TrimPrefix(path, "traits/")
			capture.Traits[filename] = content
		}
	}

	return capture, nil
}
