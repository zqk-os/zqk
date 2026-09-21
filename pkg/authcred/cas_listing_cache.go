package authcred

import (
	"encoding/json"
	"maps"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type listingFile struct {
	hash      string
	yamlMtime int64
	raw       []byte
}

type listingSnap struct {
	mu         sync.Mutex
	indexMtime int64
	mappings   map[string]string
	files      map[string]listingFile
}

var listingSnaps sync.Map // projectRoot + "\x00" + indexPath -> *listingSnap

type listingIndexFile struct {
	Mappings map[string]string `json:"mappings"`
}

func listingCache(projectRoot, indexPath string) *listingSnap {
	key := projectRoot + "\x00" + indexPath
	if existing, ok := listingSnaps.Load(key); ok {
		return existing.(*listingSnap)
	}
	fresh := &listingSnap{}
	actual, _ := listingSnaps.LoadOrStore(key, fresh)
	return actual.(*listingSnap)
}

func (s *listingSnap) reset() {
	s.indexMtime = 0
	s.mappings = nil
	s.files = nil
}

func (s *listingSnap) reloadIndex(indexPath string) bool {
	info, err := fileutil.Stat(indexPath)
	if err != nil {
		s.reset()
		return false
	}
	mtime := info.ModTime().UnixNano()
	if s.indexMtime == mtime && s.mappings != nil {
		return true
	}
	data, err := fileutil.ReadFile(indexPath)
	if err != nil {
		s.reset()
		return false
	}
	var idx listingIndexFile
	if json.Unmarshal(data, &idx) != nil || idx.Mappings == nil {
		s.reset()
		return false
	}
	s.indexMtime = mtime
	s.mappings = idx.Mappings
	s.files = make(map[string]listingFile, len(idx.Mappings))
	return true
}

func casMappings(projectRoot, indexPath string) map[string]string {
	if projectRoot == "" || indexPath == "" {
		return nil
	}
	s := listingCache(projectRoot, indexPath)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.reloadIndex(indexPath) {
		return nil
	}
	return maps.Clone(s.mappings)
}

func casHash(projectRoot, objectID, indexPath string) (string, bool) {
	objectID = strings.TrimSpace(objectID)
	if projectRoot == "" || objectID == "" || indexPath == "" {
		return "", false
	}
	s := listingCache(projectRoot, indexPath)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.reloadIndex(indexPath) {
		return "", false
	}
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
	s := listingCache(projectRoot, indexPath)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.reloadIndex(indexPath) {
		return nil, false
	}
	hash, ok := s.mappings[objectID]
	if !ok || strings.TrimSpace(hash) == "" {
		return nil, false
	}
	yamlPath := filepath.Join(kindDir, hash+paths.YAMLExtension)
	info, err := fileutil.Stat(yamlPath)
	if err != nil {
		delete(s.files, objectID)
		return nil, false
	}
	yamlMtime := info.ModTime().UnixNano()
	if hit, ok := s.files[objectID]; ok && hit.hash == hash && hit.yamlMtime == yamlMtime {
		return hit.raw, true
	}
	raw, err := fileutil.ReadFile(yamlPath)
	if err != nil {
		delete(s.files, objectID)
		return nil, false
	}
	s.files[objectID] = listingFile{hash: hash, yamlMtime: yamlMtime, raw: raw}
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
