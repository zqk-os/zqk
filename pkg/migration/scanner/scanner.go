package scanner

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/appledouble"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const emptyValue = ""

// YAMLFile represents a discovered YAML file
type YAMLFile struct {
	Path       string
	ObjectType string // Inferred from directory name (e.g., "backlog" -> "backlog_item")
	ObjectID   string // Extracted from filename or from file (CAS)
	Kind       string // Set from directory (ObjectType) or from file when we read for ID (avoids second read in cache build)
	Size       int64
	ModTime    int64
}

// YAMLScanner scans a directory for YAML files
type YAMLScanner struct {
	rootPath        string
	excludeDirs     []string
	excludePatterns []string
}

// NewYAMLScanner creates a new YAML scanner
// Uses scanner configuration instead of hardcoded exclude lists
func NewYAMLScanner(rootPath string) *YAMLScanner {
	config := GetGlobalScannerConfig()
	if config == nil {
		config = getDefaultScannerConfig()
	}

	return &YAMLScanner{
		rootPath:        rootPath,
		excludeDirs:     config.GetExcludeDirectories("yaml_scanner"),
		excludePatterns: config.GetExcludePatterns("yaml_scanner"),
	}
}

// Scan scans the root directory for YAML files
func (s *YAMLScanner) Scan() ([]*YAMLFile, error) {
	var files []*YAMLFile

	err := filepath.WalkDir(s.rootPath, func(path string, d fileutil.DirEntry, err error) error {
		if err != nil {
			if fileutil.IsNotExist(err) {
				return nil
			}
			return err
		}

		// Skip directories
		if d.IsDir() {
			// Check if directory should be excluded
			baseName := filepath.Base(path)
			for _, excludeDir := range s.excludeDirs {
				if baseName == excludeDir {
					return filepath.SkipDir
				}
			}
			return nil
		}

		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}

		// Check if file is YAML
		if !strings.HasSuffix(strings.ToLower(path), ".yaml") &&
			!strings.HasSuffix(strings.ToLower(path), ".yml") {
			return nil
		}

		// Check exclude patterns
		relPath, err := filepath.Rel(s.rootPath, path)
		if err != nil {
			return err
		}

		if s.shouldExclude(relPath) {
			return nil
		}

		// Extract object type from directory
		objectType := s.inferObjectType(path)

		// Try filename-based extraction first (fast, no file I/O)
		// Most objects have IDs that match their filename (e.g., "BLI-001.yaml" -> "BLI-001")
		objectID := s.extractObjectID(d.Name())

		// Check if this is a hash-based filename (CAS file)
		// Hash-based files have 64-character hex filenames (e.g., "219aef63504016ef2996f1808a781bd6b959d946d19d1c80f7ac74b80ee4d109.yaml")
		// In this case, we need to read the file to get the actual object ID
		isHashBasedFile := len(objectID) == 64 && s.isHexString(objectID)

		// Only read file if:
		// 1. Filename extraction failed (empty ID)
		// 2. Object type is "account" (accounts use "account:username" format, not filename-based)
		// 3. Filename is a hash (CAS file - hash-based storage)
		// This avoids reading/parsing 616 files during cache building - only reads ~2 account files + CAS files
		objectKind := objectType // Default: kind from directory
		if objectID == emptyValue || objectType == objects.KindAccount || isHashBasedFile {
			// Need to read file for ID (account format, missing ID in filename, or CAS hash-based file)
			fileID, fileKind := s.extractIDAndKindFromFile(path)
			if fileID != emptyValue {
				objectID = fileID
			}
			if fileKind != emptyValue {
				objectKind = fileKind
			}
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		file := &YAMLFile{
			Path:       path,
			ObjectType: objectType,
			ObjectID:   objectID,
			Kind:       objectKind,
			Size:       info.Size(),
			ModTime:    info.ModTime().Unix(),
		}

		files = append(files, file)
		return nil
	})

	if err != nil {
		return nil, errfmt.Newf("failed to scan directory").Wrap(err)
	}

	return files, nil
}

// shouldExclude checks if a file path should be excluded
func (s *YAMLScanner) shouldExclude(relPath string) bool {
	// Check if any part of the path matches exclude directories
	for part := range strings.SplitSeq(relPath, string(filepath.Separator)) {
		for _, excludeDir := range s.excludeDirs {
			if part == excludeDir {
				return true
			}
		}
	}

	// Check exclude patterns
	for _, pattern := range s.excludePatterns {
		matched, err := filepath.Match(pattern, filepath.Base(relPath))
		if err != nil {
			// If pattern is invalid, don't match
			matched = false
		}
		if matched {
			return true
		}
	}

	return false
}

// inferObjectType infers the object type from the directory path
func (s *YAMLScanner) inferObjectType(path string) string {
	dir := filepath.Dir(path)

	current := dir
	var highestMappedKind string

	// Walk up from the file's directory to the rootPath (or filesystem root)
	// The highest directory in the path that maps to a kind defines the object type.
	// This prevents subdirectories (e.g., command_specs/keystore) from incorrectly
	// overriding the parent kind (command_spec) just because their name matches another kind.
	for {
		base := filepath.Base(current)
		objType := objects.GetKindFromDirectory(base)
		if objType != emptyValue {
			highestMappedKind = objType
		}

		if current == s.rootPath || current == "." || current == "/" {
			break
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	if highestMappedKind != emptyValue {
		return highestMappedKind
	}

	// Default: use immediate directory name if no mapping found anywhere in path
	return filepath.Base(dir)
}

// extractIDAndKindFromFile reads the file once and returns id and kind so cache build does not re-read (avoids 2x I/O).
// Uses fast prefix scan (no full YAML parse) for performance in cache build.
func (s *YAMLScanner) extractIDAndKindFromFile(filePath string) (id, kind string) {
	id, kind = objects.ReadIDAndKindFromYAMLFile(filePath)
	if id != emptyValue {
		return id, kind
	}
	return "", ""
}

// extractObjectID extracts the object ID from the filename
func (s *YAMLScanner) extractObjectID(filename string) string {
	// Remove .yaml/.yml extension
	name := strings.TrimSuffix(filename, ".yaml")
	name = strings.TrimSuffix(name, ".yml")

	// Return the filename as the ID (e.g., "BLI-001" from "BLI-001.yaml")
	return name
}

// isHexString checks if a string contains only hexadecimal characters
func (s *YAMLScanner) isHexString(str string) bool {
	for _, c := range str {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}
