package authcred

import (
	"encoding/json"
	"errors"
	"maps"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type listingFile struct {
	hash      string
	yamlStamp stampmemo.Stamp
	raw       []byte
}

type listingValue struct {
	mu       sync.Mutex
	mappings map[string]string
	files    map[string]listingFile
}

// listingIndexes is keyed by listing-index path. Stamp is that file.
var listingIndexes stampmemo.Table[*listingValue]

type listingIndexFile struct {
	Mappings map[string]string `json:"mappings"`
}

var errListingIndex = errors.New("listing index missing or invalid")

func loadListingIndex(indexPath string) (*listingValue, error) {
	data, err := fileutil.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}
	var idx listingIndexFile
	if json.Unmarshal(data, &idx) != nil || idx.Mappings == nil {
		return nil, errListingIndex
	}
	return &listingValue{
		mappings: idx.Mappings,
		files:    make(map[string]listingFile, len(idx.Mappings)),
	}, nil
}

func listingFor(projectRoot, indexPath string) *listingValue {
	v, err := listingIndexes.Load(projectRoot+"\x00"+indexPath, stampmemo.Of(indexPath), func() (*listingValue, error) {
		return loadListingIndex(indexPath)
	})
	if err != nil {
		return nil
	}
	return v
}

func casMappings(projectRoot, indexPath string) map[string]string {
	if projectRoot == "" || indexPath == "" {
		return nil
	}
	s := listingFor(projectRoot, indexPath)
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return maps.Clone(s.mappings)
}

func casHash(projectRoot, objectID, indexPath string) (string, bool) {
	objectID = strings.TrimSpace(objectID)
	if projectRoot == "" || objectID == "" || indexPath == "" {
		return "", false
	}
	s := listingFor(projectRoot, indexPath)
	if s == nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	hash, ok := s.mappings[objectID]
	if !ok || strings.TrimSpace(hash) == "" {
		return "", false
	}
	return hash, true
}

func casYAML(projectRoot, objectID, indexPath, kindDir string) ([]byte, bool) {
	objectID = strings.TrimSpace(objectID)
	if projectRoot == "" || objectID == "" || indexPath == "" || kindDir == "" {
		return nil, false
	}
	s := listingFor(projectRoot, indexPath)
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	hash, ok := s.mappings[objectID]
	if !ok || strings.TrimSpace(hash) == "" {
		return nil, false
	}
	yamlPath := filepath.Join(kindDir, hash+paths.YAMLExtension)
	yamlStamp := stampmemo.Of(yamlPath)
	if yamlStamp == 0 {
		delete(s.files, objectID)
		return nil, false
	}
	if hit, ok := s.files[objectID]; ok && hit.hash == hash && hit.yamlStamp == yamlStamp {
		return hit.raw, true
	}
	raw, err := fileutil.ReadFile(yamlPath)
	if err != nil {
		delete(s.files, objectID)
		return nil, false
	}
	s.files[objectID] = listingFile{hash: hash, yamlStamp: yamlStamp, raw: raw}
	return raw, true
}

// AccountYAML returns cached CAS bytes for an ACC-* id.
func AccountYAML(projectRoot, accountID string) ([]byte, bool) {
	accountID = strings.TrimSpace(accountID)
	if projectRoot == "" || accountID == "" {
		return nil, false
	}
	return casYAML(projectRoot, accountID, paths.AccountIndexPath(projectRoot), paths.AccountsDirPath(projectRoot))
}

// WalkAccountYAML calls fn for each ACC-* mapping until fn returns false.
func WalkAccountYAML(projectRoot string, fn func(accountID string, raw []byte) bool) {
	if projectRoot == "" || fn == nil {
		return
	}
	indexPath := paths.AccountIndexPath(projectRoot)
	dir := paths.AccountsDirPath(projectRoot)
	for id := range casMappings(projectRoot, indexPath) {
		if !strings.HasPrefix(id, "ACC-") {
			continue
		}
		raw, ok := casYAML(projectRoot, id, indexPath, dir)
		if !ok {
			continue
		}
		if !fn(id, raw) {
			return
		}
	}
}
