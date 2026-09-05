package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// Object draft plane: id-keyed mutable YAML for lifecycle-preliminary objects.
// Distinct from CLI template drafts under .zqk/drafts/ (paths.DraftsDir).
//
// The object-id-cache is the identity index (id → live path). Get/Exists follow
// that one path. List/Count use the CAS index and must omit draft-plane paths;
// a draft-plane list view is a separate API if we build one.
//
// TRACK: TDE-CEF-CAS-IDENTITY-TXN-001

const (
	objectDraftShardHexLen = 2
	// objectDraftPlaneSamplePerKindCap bounds examples under each kind in check output / JSON.
	// Prefer a few IDs in kind context over a flat cross-kind sausage.
	objectDraftPlaneSamplePerKindCap = 3
	// objectDraftPlaneSampleIDCap is the flattened SampleIDs ceiling (JSON/compat).
	objectDraftPlaneSampleIDCap = 20
)

// ObjectDraftPlaneInventory is a filesystem rollup of `.zqk/object_drafts` for system check / doctor.
// Normal object List/Count do not include these IDs.
type ObjectDraftPlaneInventory struct {
	Total  int            `json:"total"`
	ByKind map[string]int `json:"by_kind,omitempty"`
	// SampleIDsByKind holds up to objectDraftPlaneSamplePerKindCap ids per kind (sorted).
	SampleIDsByKind map[string][]string `json:"sample_ids_by_kind,omitempty"`
	// SampleIDs is a flattened, capped list for older consumers; prefer SampleIDsByKind.
	SampleIDs []string `json:"sample_ids,omitempty"`
	// DualPlaneIDs are draft-plane ids that also have a CAS index entry (split-brain).
	// Cache is SSOT for Get; this inventory flags the extra blob.
	// TRACK: BLI-CAS-HAND-DUP-CHECK-001 — dual-plane detect after kernel rot incident 2026-08-12.
	DualPlaneIDs []string `json:"dual_plane_ids,omitempty"`
}

// InventoryObjectDraftPlane walks the object draft plane and returns counts + sample IDs.
func InventoryObjectDraftPlane(projectRoot string) ObjectDraftPlaneInventory {
	inv := ObjectDraftPlaneInventory{
		ByKind:          make(map[string]int),
		SampleIDsByKind: make(map[string][]string),
	}
	if projectRoot == emptyValue {
		return inv
	}
	root := ObjectDraftPlaneRoot(projectRoot)
	entries, err := fileutil.ReadDir(root)
	if err != nil {
		return inv
	}
	var flat []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		kind := e.Name()
		ids, listErr := ListObjectDraftPlaneIDs(projectRoot, kind)
		if listErr != nil || len(ids) == 0 {
			continue
		}
		inv.ByKind[kind] = len(ids)
		inv.Total += len(ids)
		for _, id := range ids {
			if casBackedObjectID(projectRoot, kind, id) {
				inv.DualPlaneIDs = append(inv.DualPlaneIDs, id)
			}
		}
		n := len(ids)
		if n > objectDraftPlaneSamplePerKindCap {
			n = objectDraftPlaneSamplePerKindCap
		}
		kindSamples := append([]string(nil), ids[:n]...)
		inv.SampleIDsByKind[kind] = kindSamples
		flat = append(flat, kindSamples...)
	}
	sort.Strings(flat)
	if len(flat) > objectDraftPlaneSampleIDCap {
		inv.SampleIDs = append([]string(nil), flat[:objectDraftPlaneSampleIDCap]...)
	} else if len(flat) > 0 {
		inv.SampleIDs = flat
	}
	if len(inv.DualPlaneIDs) > 0 {
		sort.Strings(inv.DualPlaneIDs)
	}
	if inv.Total == 0 {
		inv.ByKind = nil
		inv.SampleIDsByKind = nil
	}
	return inv
}

// DraftPlaneHasTitle reports whether any draft-plane object of kind has this title
// (trimmed, case-insensitive). List/Count omit the draft plane, so automation
// that keys uniqueness on title must call this or it will remint forever.
func DraftPlaneHasTitle(projectRoot, kind, title string) bool {
	title = strings.TrimSpace(title)
	if projectRoot == emptyValue || kind == emptyValue || title == emptyValue {
		return false
	}
	ids, err := ListObjectDraftPlaneIDs(projectRoot, kind)
	if err != nil {
		return false
	}
	want := strings.ToLower(title)
	for _, id := range ids {
		data, readErr := fileutil.ReadFile(ObjectDraftPlanePath(projectRoot, kind, id))
		if readErr != nil {
			continue
		}
		var obj map[string]any
		if uErr := yaml.Unmarshal(data, &obj); uErr != nil {
			continue
		}
		got, _ := obj[objects.FieldKeyTitle].(string)
		if strings.ToLower(strings.TrimSpace(got)) == want {
			return true
		}
	}
	return false
}

// ListObjectDraftPlaneIDs returns object IDs on the draft plane for kind (any process; no FileObjectStorage).
func ListObjectDraftPlaneIDs(projectRoot, kind string) ([]string, error) {
	if projectRoot == emptyValue || kind == emptyValue {
		return nil, nil
	}
	kindRoot := filepath.Join(ObjectDraftPlaneRoot(projectRoot), kind)
	entries, err := fileutil.ReadDir(kindRoot)
	if fileutil.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errfmt.Newf("object draft plane: list kind %s", kind).Wrap(err)
	}
	cfg := GetStorageConfig()
	ext := cfg.YAMLExtension
	var ids []string
	for _, shard := range entries {
		if !shard.IsDir() {
			continue
		}
		shardPath := filepath.Join(kindRoot, shard.Name())
		files, readErr := fileutil.ReadDir(shardPath)
		if readErr != nil {
			continue
		}
		for _, fi := range files {
			if fi.IsDir() {
				continue
			}
			name := fi.Name()
			if !strings.HasSuffix(name, ext) {
				continue
			}
			ids = append(ids, strings.TrimSuffix(name, ext))
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// ObjectDraftPlaneRoot returns .zqk/object_drafts under projectRoot.
func ObjectDraftPlaneRoot(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.ObjectDraftsDir)
}

// objectDraftShard returns a 2-hex-char bucket so kind dirs stay under the ≤100 top-level budget.
func objectDraftShard(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])[:objectDraftShardHexLen]
}

// ObjectDraftPlanePath returns the on-disk path for a draft-plane object.
func ObjectDraftPlanePath(projectRoot, kind, id string) string {
	cfg := GetStorageConfig()
	return filepath.Join(ObjectDraftPlaneRoot(projectRoot), kind, objectDraftShard(id), id+cfg.YAMLExtension)
}

// IsObjectDraftPlanePath reports whether path is under the object draft plane root.
func IsObjectDraftPlanePath(projectRoot, path string) bool {
	if projectRoot == emptyValue || path == emptyValue {
		return false
	}
	root := ObjectDraftPlaneRoot(projectRoot)
	clean := filepath.Clean(path)
	prefix := filepath.Clean(root) + string(fileutil.PathSeparator)
	return clean == filepath.Clean(root) || strings.HasPrefix(clean, prefix)
}

// shouldUseObjectDraftPlane is true for CAS (non-stream) kinds in a preliminary lifecycle status.
// When the lifecycle loader errors, do not force draft solely because status equals origin —
// kinds whose origin is a terminal/active status (e.g. glossary_term origin=active) must use CAS.
// TRACK: [REDACTED-ID] (draft-plane create path); VDS glossary materialization.
func shouldUseObjectDraftPlane(kind, status string) bool {
	if kind == emptyValue || status == emptyValue {
		return false
	}
	if StreamStorageEnabledForKind(kind) {
		return false
	}
	checker := objects.GetGlobalStatusChecker()
	if checker.IsPreliminary(kind, status) {
		return true
	}
	loader := objects.GetGlobalLifecycleLoader()
	if loader == nil {
		return false
	}
	origin, err := loader.GetOriginStatus(kind)
	if err != nil || origin == emptyValue {
		return false
	}
	// Park at origin only when that origin status is itself preliminary (draft/proposed/…).
	return status == origin && checker.IsPreliminary(kind, origin)
}

func (f *FileObjectStorage) objectDraftPlanePath(kind, id string) string {
	return ObjectDraftPlanePath(f.projectRoot, kind, id)
}

func (f *FileObjectStorage) objectDraftPlaneExists(kind, id string) bool {
	if f.projectRoot == emptyValue || kind == emptyValue || id == emptyValue {
		return false
	}
	_, err := fileutil.Stat(f.objectDraftPlanePath(kind, id))
	return err == nil
}

func (f *FileObjectStorage) WriteObjectToDraftPlane(id, kind string, data []byte) error {
	if f.projectRoot == emptyValue {
		return errfmt.Errorf("object draft plane: empty project root")
	}
	if len(data) == 0 {
		return errfmt.Errorf("object draft plane: empty payload for %s", id)
	}
	path := f.objectDraftPlanePath(kind, id)
	cfg := GetStorageConfig()
	if err := fileutil.MkdirAll(filepath.Dir(path), cfg.DefaultDirPerm); err != nil {
		return errfmt.Newf("object draft plane: mkdir").Wrap(err)
	}
	dsia := NewDSIAStorageProvider()
	if err := dsia.AtomicWriteFile(path, data, cfg.DefaultFilePerm); err != nil {
		return errfmt.Newf("object draft plane: write %s", id).Wrap(err)
	}
	return nil
}

func (f *FileObjectStorage) readObjectDraftPlane(kind, id string) (map[string]any, error) {
	data, err := f.readObjectDraftPlaneBytes(kind, id)
	if err != nil {
		return nil, err
	}
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil, errfmt.Newf("object draft plane: unmarshal %s", id).Wrap(err)
	}
	obj[objects.FieldKeyKind] = kind
	return obj, nil
}

func (f *FileObjectStorage) readObjectDraftPlaneBytes(kind, id string) ([]byte, error) {
	path := f.objectDraftPlanePath(kind, id)
	data, err := fileutil.ReadFile(path)
	if fileutil.IsNotExist(err) {
		return nil, ErrObjectNotFound
	}
	if err != nil {
		return nil, errfmt.Newf("object draft plane: read %s", id).Wrap(err)
	}
	return data, nil
}

func (f *FileObjectStorage) deleteObjectDraftPlane(kind, id string) error {
	path := f.objectDraftPlanePath(kind, id)
	if err := fileutil.Remove(path); err != nil && !fileutil.IsNotExist(err) {
		return errfmt.Newf("object draft plane: delete %s", id).Wrap(err)
	}
	// Best-effort prune empty shard / kind dirs (ignore errors).
	shardDir := filepath.Dir(path)
	_ = fileutil.Remove(shardDir)
	_ = fileutil.Remove(filepath.Dir(shardDir))
	return nil
}
