package community

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var (
	ErrManifestInvalid    = errors.New("provider manifest is invalid or missing required fields")
	ErrIntegrityMismatch  = errors.New("provider integrity verification failed")
	ErrUnsupportedVersion = errors.New("provider requires higher engine version")
)

var idRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)

// ProviderManifest represents the verifiable specification of a community provider plugin.
type ProviderManifest struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	Author           string   `json:"author,omitempty"`
	MinEngineVersion string   `json:"min_engine_version"`
	Capabilities     []string `json:"capabilities"`
	ChecksumSHA256   string   `json:"checksum_sha256,omitempty"`
	Signature        string   `json:"signature,omitempty"`
}

// Validate ensures all structural invariant constraints are met.
func (m *ProviderManifest) Validate() error {
	if m == nil {
		return ErrManifestInvalid
	}
	if strings.TrimSpace(m.ID) == "" || !idRegex.MatchString(m.ID) {
		return fmt.Errorf("%w: invalid or missing provider id", ErrManifestInvalid)
	}
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("%w: missing provider name", ErrManifestInvalid)
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("%w: missing provider version", ErrManifestInvalid)
	}
	if len(m.Capabilities) == 0 {
		return fmt.Errorf("%w: capabilities must declare at least one capability", ErrManifestInvalid)
	}
	for _, cap := range m.Capabilities {
		if strings.TrimSpace(cap) == "" {
			return fmt.Errorf("%w: declared capability cannot be empty", ErrManifestInvalid)
		}
	}
	return nil
}

// ParseManifest parses and validates raw JSON manifest data.
func ParseManifest(data []byte) (*ProviderManifest, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: manifest payload is empty", ErrManifestInvalid)
	}
	var manifest ProviderManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("%w: malformed JSON: %v", ErrManifestInvalid, err)
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// VerifyManifestIntegrity verifies that the computed SHA256 matches the manifest checksum.
func VerifyManifestIntegrity(manifest *ProviderManifest, payload []byte) error {
	if manifest == nil {
		return ErrManifestInvalid
	}
	if manifest.ChecksumSHA256 == "" {
		return nil // Checksum optional unless specified
	}
	hasher := sha256.New()
	hasher.Write(payload)
	computed := hex.EncodeToString(hasher.Sum(nil))

	if !strings.EqualFold(computed, manifest.ChecksumSHA256) {
		return fmt.Errorf("%w: expected %s, got %s", ErrIntegrityMismatch, manifest.ChecksumSHA256, computed)
	}
	return nil
}

// ProviderDiscoveryScanner scans directories to discover and parse provider manifests.
type ProviderDiscoveryScanner struct {
	mu          sync.RWMutex
	searchPaths []string
}

// NewProviderDiscoveryScanner creates a scanner configured with search root paths.
func NewProviderDiscoveryScanner(paths ...string) *ProviderDiscoveryScanner {
	return &ProviderDiscoveryScanner{
		searchPaths: append([]string(nil), paths...),
	}
}

// AddSearchPath appends a search root to the scanner.
func (s *ProviderDiscoveryScanner) AddSearchPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.searchPaths = append(s.searchPaths, path)
}

// Scan scans all configured search paths for providers containing a provider.json manifest.
func (s *ProviderDiscoveryScanner) Scan(ctx context.Context) ([]*ProviderManifest, error) {
	s.mu.RLock()
	paths := append([]string(nil), s.searchPaths...)
	s.mu.RUnlock()

	var discovered []*ProviderManifest
	seen := make(map[string]bool)

	for _, root := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, err := fileutil.Stat(root); fileutil.IsNotExist(err) {
			continue
		}

		entries, err := fileutil.ReadDir(root)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			providerDir := filepath.Join(root, entry.Name())
			manifestPath := filepath.Join(providerDir, "provider.json")

			data, err := fileutil.ReadFile(manifestPath)
			if err != nil {
				continue // directory does not contain provider.json
			}

			manifest, err := ParseManifest(data)
			if err != nil {
				continue // skip invalid manifests during broad sweep
			}

			if !seen[manifest.ID] {
				seen[manifest.ID] = true
				discovered = append(discovered, manifest)
			}
		}
	}

	return discovered, nil
}

// ScanSingleDirectory inspects a single directory for provider.json and returns the manifest.
func (s *ProviderDiscoveryScanner) ScanSingleDirectory(dir string) (*ProviderManifest, error) {
	manifestPath := filepath.Join(dir, "provider.json")
	data, err := fileutil.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("%w: unable to read provider.json in %s", ErrProviderNotFound, dir)
	}
	return ParseManifest(data)
}
