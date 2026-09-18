package storage

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// SemanticSnapshotFormatVersion is the current format version
	SemanticSnapshotFormatVersion = "1.0.0"
	// SemanticSnapshotFileExtension is the file extension for semantic snapshots
	SemanticSnapshotFileExtension = ".ssnap"
)

// SemanticLibrary defines the semantic vocabulary for compression
type SemanticLibrary struct {
	Version            string              `yaml:"version" json:"version"`
	Brands             map[string]BrandDef `yaml:"brands" json:"brands"`
	CompressionOptions *CompressionOptions `yaml:"compression_options,omitempty" json:"compression_options,omitempty"`
}

// BrandDef defines a brand with its semantic token
type BrandDef struct {
	ID            string `yaml:"id" json:"id"`
	Name          string `yaml:"name" json:"name"`
	SemanticToken string `yaml:"semantic_token" json:"semantic_token"`
}

// CompressionOptions defines options for semantic compression
type CompressionOptions struct {
	CaseSensitive bool `yaml:"case_sensitive" json:"case_sensitive"`
}

// SemanticSnapshot represents a semantically compressed snapshot of files
type SemanticSnapshot struct {
	Header  *SemanticSnapshotHeader `yaml:"header" json:"header"`
	Files   []SemanticFile          `yaml:"files" json:"files"`
	Library *SemanticLibrary        `yaml:"library" json:"library"`
}

// SemanticSnapshotHeader contains metadata about the semantic snapshot
type SemanticSnapshotHeader struct {
	FormatVersion  string    `yaml:"format_version" json:"format_version"`
	Timestamp      time.Time `yaml:"timestamp" json:"timestamp"`
	FileCount      int       `yaml:"file_count" json:"file_count"`
	LibraryVersion string    `yaml:"library_version" json:"library_version"`
	Checksum       string    `yaml:"checksum" json:"checksum"`
}

// SemanticFile represents a single file in the semantic snapshot
type SemanticFile struct {
	Path         string `yaml:"path" json:"path"`
	ContentDelta string `yaml:"content_delta" json:"content_delta"`
}

// CreateSemanticSnapshot creates a semantic snapshot from files by replacing brand names with semantic tokens
func CreateSemanticSnapshot(files map[string][]byte, library *SemanticLibrary, timestamp time.Time, logger logging.Logger) (*SemanticSnapshot, error) {
	if library == nil {
		return nil, errfmt.Errorf(ConstMiscSemanticLibraryIsRequired)
	}

	caseSensitive := true
	if library.CompressionOptions != nil {
		caseSensitive = library.CompressionOptions.CaseSensitive
	}

	snapshotFiles := make([]SemanticFile, 0, len(files))

	for filePath, content := range files {
		contentStr := string(content)
		compressed := contentStr

		// Apply semantic compression for each brand in the library
		for brandKey, brandDef := range library.Brands {
			if brandDef.Name == emptyValue || brandDef.SemanticToken == emptyValue {
				StorageLog(logger).Warn(LogEventStorageSemanticSnapshotBrandIncompleteWarn).
					String("brand_key", brandKey).
					Log()
				continue
			}

			compressed = compressBrandName(compressed, brandDef.Name, brandDef.SemanticToken, caseSensitive)
		}

		snapshotFiles = append(snapshotFiles, SemanticFile{
			Path:         filePath,
			ContentDelta: compressed,
		})
	}

	// Create header
	header := &SemanticSnapshotHeader{
		FormatVersion:  SemanticSnapshotFormatVersion,
		Timestamp:      timestamp,
		FileCount:      len(snapshotFiles),
		LibraryVersion: library.Version,
	}

	// Create snapshot
	snapshot := &SemanticSnapshot{
		Header:  header,
		Files:   snapshotFiles,
		Library: library,
	}

	// Calculate checksum
	checksum, err := calculateSemanticChecksum(snapshot)
	if err != nil {
		return nil, errfmt.Newf(ConstMiscFailedToCalculateChecksum).Wrap(err)
	}
	snapshot.Header.Checksum = checksum

	return snapshot, nil
}

// compressBrandName replaces brand name occurrences with semantic tokens
// Handles: plain text, possessive ('s), camelCase (ZqkConfig), kebab-case (zqk-based)
// IMPORTANT: Process compound patterns BEFORE plain patterns to avoid double-replacement
func compressBrandName(content, brandName, semanticToken string, caseSensitive bool) string {
	result := content
	flags := ""
	if !caseSensitive {
		flags = "(?i)"
	}

	// Pattern 1: Possessive forms (ZQK's, ZQK') - BEFORE plain replacement
	// Must match the exact brand name before it gets replaced
	possPattern := regexp.MustCompile(flags + regexp.QuoteMeta(brandName) + "'s?")
	result = possPattern.ReplaceAllString(result, semanticToken+":poss")

	// Pattern 2: CamelCase compound words (ZqkConfig -> @b:zqk:camel:Config)
	// Must be case-sensitive to properly detect camelCase (capital letter after brand name)
	// Match BEFORE plain replacement to avoid matching "@b:zqkConfig"
	camelPattern := regexp.MustCompile(regexp.QuoteMeta(brandName) + "([A-Z]\\w*)")
	result = camelPattern.ReplaceAllStringFunc(result, func(match string) string {
		submatches := camelPattern.FindStringSubmatch(match)
		if len(submatches) > 1 {
			return semanticToken + ":camel:" + submatches[1]
		}
		return match
	})

	// Pattern 3: Kebab-case compound words (zqk-based -> @b:zqk:kebab:based)
	// Match BEFORE plain replacement to avoid matching "@b:zqk-based"
	kebabPattern := regexp.MustCompile(flags + regexp.QuoteMeta(brandName) + "-(\\w+)")
	result = kebabPattern.ReplaceAllStringFunc(result, func(match string) string {
		submatches := kebabPattern.FindStringSubmatch(match)
		if len(submatches) > 1 {
			return semanticToken + ":kebab:" + submatches[1]
		}
		return match
	})

	// Pattern 4: Plain brand name (LAST - only matches remaining instances)
	// Since compound patterns run first, only standalone instances remain
	// IMPORTANT: Always use case-SENSITIVE matching for plain pattern to avoid matching inside tokens
	// (tokens contain the brand name, e.g., "@b:zqk" contains "zqk")
	// Compound patterns already handle case-insensitive variants above
	escapeBrand := regexp.QuoteMeta(brandName)
	plainPattern := regexp.MustCompile(`\b` + escapeBrand + `\b`)
	result = plainPattern.ReplaceAllString(result, semanticToken)

	return result
}

// ExpandSemanticSnapshot expands a semantic snapshot back to original files by replacing tokens with brand names
func ExpandSemanticSnapshot(snapshot *SemanticSnapshot) (map[string][]byte, error) {
	if snapshot == nil || snapshot.Library == nil {
		return nil, errfmt.Errorf(ConstMiscSemanticSnapshotAndLibraryAreRequired)
	}

	caseSensitive := true
	if snapshot.Library.CompressionOptions != nil {
		caseSensitive = snapshot.Library.CompressionOptions.CaseSensitive
	}

	// Build token to brand name mapping
	tokenToBrand := make(map[string]string)
	for _, brandDef := range snapshot.Library.Brands {
		if brandDef.SemanticToken != emptyValue && brandDef.Name != emptyValue {
			tokenToBrand[brandDef.SemanticToken] = brandDef.Name
		}
	}

	files := make(map[string][]byte)

	for _, semanticFile := range snapshot.Files {
		content := semanticFile.ContentDelta

		// Expand tokens back to brand names
		for token, brandName := range tokenToBrand {
			content = expandTokens(content, token, brandName, caseSensitive)
		}

		files[semanticFile.Path] = []byte(content)
	}

	return files, nil
}

// expandTokens reverses the compression by replacing semantic tokens with brand names
func expandTokens(content, semanticToken, brandName string, _ bool) string {
	result := content

	// Expand kebab-case tokens (@b:zqk:kebab:based -> Zqk-based)
	kebabPattern := regexp.MustCompile(regexp.QuoteMeta(semanticToken+":kebab:") + ConstMiscAZaZ09)
	result = kebabPattern.ReplaceAllStringFunc(result, func(match string) string {
		word := strings.TrimPrefix(match, semanticToken+":kebab:")
		return brandName + "-" + word
	})

	// Expand camelCase tokens (@b:zqk:camel:Config -> ZqkConfig)
	camelPattern := regexp.MustCompile(regexp.QuoteMeta(semanticToken+":camel:") + "([A-Z]\\w*)")
	result = camelPattern.ReplaceAllStringFunc(result, func(match string) string {
		word := strings.TrimPrefix(match, semanticToken+":camel:")
		return brandName + word
	})

	// Expand possessive tokens (@b:zqk:poss -> Zqk's)
	possPattern := regexp.MustCompile(regexp.QuoteMeta(semanticToken + ":poss"))
	result = possPattern.ReplaceAllString(result, brandName+"'s")

	// Expand plain tokens (@b:zqk -> Zqk) - last to avoid conflicts
	plainPattern := regexp.MustCompile(regexp.QuoteMeta(semanticToken))
	result = plainPattern.ReplaceAllString(result, brandName)

	return result
}

// SubstituteSemanticTokens substitutes one semantic token with another in the snapshot
func SubstituteSemanticTokens(snapshot *SemanticSnapshot, fromToken, toToken string) error {
	if snapshot == nil {
		return errfmt.Errorf(ConstMiscSemanticSnapshotIsRequired)
	}

	if fromToken == emptyValue || toToken == emptyValue {
		return errfmt.Errorf(ConstMiscFromtokenAndTotokenAreRequired)
	}

	// Replace tokens in all files
	for i := range snapshot.Files {
		content := snapshot.Files[i].ContentDelta
		// Replace all occurrences of fromToken with toToken
		// This handles plain tokens, compound tokens (camel, kebab, poss), etc.
		snapshot.Files[i].ContentDelta = strings.ReplaceAll(content, fromToken, toToken)
	}

	// Recalculate checksum
	checksum, err := calculateSemanticChecksum(snapshot)
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToRecalculateChecksum).Wrap(err)
	}
	snapshot.Header.Checksum = checksum

	return nil
}

// calculateSemanticChecksum calculates SHA256 checksum of semantic snapshot data
func calculateSemanticChecksum(snapshot *SemanticSnapshot) (string, error) {
	// Marshal files section to JSON for checksum
	filesJSON, err := json.Marshal(snapshot.Files)
	if err != nil {
		return "", errfmt.Newf(ConstMiscFailedToMarshalFilesForChecksum).Wrap(err)
	}

	hash := sha256.Sum256(filesJSON)
	return fmt.Sprintf("%x", hash), nil
}

// WriteSemanticSnapshot writes a semantic snapshot to a file
func WriteSemanticSnapshot(snapshot *SemanticSnapshot, filePath string) error {
	if snapshot == nil {
		return errfmt.Errorf(ConstMiscSemanticSnapshotIsRequired)
	}

	// Calculate checksum
	checksum, err := calculateSemanticChecksum(snapshot)
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToCalculateChecksum).Wrap(err)
	}
	snapshot.Header.Checksum = checksum

	// Marshal to YAML
	data, err := yaml.Marshal(snapshot)
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToMarshalSemanticSnapshot).Wrap(err)
	}

	// Create directory if needed
	dir := filepath.Dir(filePath)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return errfmt.Newf(ErrMsgCreateDir).Wrap(err)
	}

	// Write to file
	if err := fileutil.WriteFile(filePath, data, paths.FilePerm600); err != nil {
		return errfmt.Newf(ConstMiscFailedToWriteSemanticSnapshot).Wrap(err)
	}

	return nil
}

// ReadSemanticSnapshot reads a semantic snapshot from a file
func ReadSemanticSnapshot(filePath string) (*SemanticSnapshot, error) {
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return nil, errfmt.Newf(ConstMiscFailedToReadSemanticSnapshot).Wrap(err)
	}

	var snapshot SemanticSnapshot
	if err := yaml.Unmarshal(data, &snapshot); err != nil {
		return nil, errfmt.Newf(ConstMiscFailedToUnmarshalSemanticSnapshot).Wrap(err)
	}

	// Validate checksum
	expectedChecksum := snapshot.Header.Checksum
	actualChecksum, err := calculateSemanticChecksum(&snapshot)
	if err != nil {
		return nil, errfmt.Newf(ConstMiscFailedToCalculateChecksum).Wrap(err)
	}

	if expectedChecksum != actualChecksum {
		return nil, errfmt.Errorf(ConstMiscChecksumMismatchExpectedSGotS, expectedChecksum, actualChecksum)
	}

	return &snapshot, nil
}
