package caslist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type indexFile struct {
	Version      string            `json:"version"`
	Kind         string            `json:"kind"`
	Mappings     map[string]string `json:"mappings"`
	BucketKeys   map[string]string `json:"bucket_keys,omitempty"`
	ErasePending map[string]string `json:"erase_pending,omitempty"`
}

func indexFileName(kind string) string {
	return indexFilePrefix + kind + indexFileSuffix
}

func kindFromIndexName(name string) string {
	if !strings.HasPrefix(name, indexFilePrefix) || !strings.HasSuffix(name, indexFileSuffix) {
		return ""
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(name, indexFilePrefix), indexFileSuffix)
	if inner == "" || strings.Contains(inner, ".") {
		return ""
	}
	return inner
}

func loadIndex(kindDir, kind string) (*indexFile, error) {
	raw, err := os.ReadFile(filepath.Join(kindDir, indexFileName(kind)))
	if err != nil {
		return nil, err
	}
	var idx indexFile
	if err := json.Unmarshal(raw, &idx); err != nil {
		return nil, err
	}
	if idx.Mappings == nil {
		idx.Mappings = map[string]string{}
	}
	if idx.Kind == "" {
		idx.Kind = kind
	}
	return &idx, nil
}

func blobPath(kindDir, hash, bucket string) string {
	name := hash + yamlExt
	if bucket != "" {
		return filepath.Join(kindDir, bucket, name)
	}
	return filepath.Join(kindDir, name)
}

func loadObject(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var obj map[string]any
	if err := yaml.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func getByID(kindDir, kind, id string) (map[string]any, error) {
	idx, err := loadIndex(kindDir, kind)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if _, erased := idx.ErasePending[id]; erased {
		return nil, nil
	}
	hash, ok := idx.Mappings[id]
	if !ok || hash == "" {
		return nil, nil
	}
	bucket := ""
	if idx.BucketKeys != nil {
		bucket = idx.BucketKeys[id]
	}
	return loadObject(blobPath(kindDir, hash, bucket))
}

func listKind(kindDir, kind string) ([]map[string]any, error) {
	idx, err := loadIndex(kindDir, kind)
	if err != nil {
		if os.IsNotExist(err) {
			return listKindByScan(kindDir)
		}
		return nil, err
	}
	out := make([]map[string]any, 0, len(idx.Mappings))
	for id, hash := range idx.Mappings {
		if _, erased := idx.ErasePending[id]; erased {
			continue
		}
		if hash == "" {
			continue
		}
		bucket := ""
		if idx.BucketKeys != nil {
			bucket = idx.BucketKeys[id]
		}
		obj, err := loadObject(blobPath(kindDir, hash, bucket))
		if err != nil {
			continue
		}
		out = append(out, obj)
	}
	return out, nil
}

func listKindByScan(kindDir string) ([]map[string]any, error) {
	entries, err := os.ReadDir(kindDir)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, e := range entries {
		if e.IsDir() || !isHashYAML(e.Name()) {
			continue
		}
		obj, err := loadObject(filepath.Join(kindDir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, obj)
	}
	return out, nil
}

func isHashYAML(name string) bool {
	if !strings.HasSuffix(name, yamlExt) {
		return false
	}
	stem := strings.TrimSuffix(name, yamlExt)
	if len(stem) != hexHashLen {
		return false
	}
	for _, r := range stem {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
			continue
		default:
			return false
		}
	}
	return true
}

func findKindDir(processRoot, kind string) (string, bool) {
	want := indexFileName(kind)
	entries, err := os.ReadDir(processRoot)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == internalDirName {
			continue
		}
		dir := filepath.Join(processRoot, e.Name())
		if _, err := os.Stat(filepath.Join(dir, want)); err == nil {
			return dir, true
		}
	}
	return "", false
}

func eachIndexedKind(processRoot string, fn func(kind, kindDir string) error) error {
	entries, err := os.ReadDir(processRoot)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == internalDirName {
			continue
		}
		dir := filepath.Join(processRoot, e.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			kind := kindFromIndexName(f.Name())
			if kind == "" {
				continue
			}
			if err := fn(kind, dir); err != nil {
				return err
			}
		}
	}
	return nil
}
