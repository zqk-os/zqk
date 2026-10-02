package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/cas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Object draft plane: id-keyed mutable YAML for lifecycle-preliminary objects.
// Distinct from CLI template drafts under .zqk/drafts/ (paths.DraftsDir).
//
// The object-id-cache is the identity index (id → live path). Get/Exists follow
// that one path. List/Count use the CAS index and must omit draft-plane paths;
// a draft-plane list view is a separate API if we build one.
//

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
	// dual-plane detect after kernel rot incident 2026-08-12.
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
	if clean == filepath.Clean(root) || strings.HasPrefix(clean, prefix) {
		return true
	}
	if !filepath.IsAbs(path) {
		absPath := filepath.Clean(filepath.Join(projectRoot, path))
		return absPath == filepath.Clean(root) || strings.HasPrefix(absPath, prefix)
	}
	return false
}

// shouldUseObjectDraftPlane is true for CAS (non-stream) kinds in a preliminary lifecycle status.
// When the lifecycle loader errors, do not force draft solely because status equals origin —
// kinds whose origin is a terminal/active status (e.g. glossary_term origin=active) must use CAS.
// TRACK: (draft-plane create path); VDS glossary materialization.
func shouldUseObjectDraftPlane(kind, status string) bool {
	if StreamStorageEnabledForKind(kind) {
		return false
	}
	return cas.ShouldUseObjectDraftPlane(kind, status)
}

// ObjectDraftPlaneExists reports whether the object exists on the draft plane.
func ObjectDraftPlaneExists(projectRoot, kind, id string) bool {
	if projectRoot == emptyValue || kind == emptyValue || id == emptyValue {
		return false
	}
	path := ObjectDraftPlanePath(projectRoot, kind, id)
	_, err := fileutil.Stat(path)
	return err == nil
}

// CasBackedObjectID reports whether the CAS index already maps this id (post-membrane).
func CasBackedObjectID(projectRoot, kind, id string) bool {
	return casBackedObjectID(projectRoot, kind, id)
}

// IsDraftPlaneOnly reports whether an object exists on the draft plane and has not crossed into CAS.
// When kind is empty, checks all draft plane kinds.
func IsDraftPlaneOnly(projectRoot, kind, id string) bool {
	if projectRoot == emptyValue || id == emptyValue {
		return false
	}
	if kind == emptyValue {
		return isDraftPlaneOnlyAnyKind(projectRoot, id)
	}
	return ObjectDraftPlaneExists(projectRoot, kind, id) && !CasBackedObjectID(projectRoot, kind, id)
}

func isDraftPlaneOnlyAnyKind(projectRoot, id string) bool {
	if projectRoot == emptyValue || id == emptyValue {
		return false
	}
	shard := objectDraftShard(id)
	kinds, err := draftPlaneKinds(projectRoot, "")
	if err != nil {
		return false
	}
	cfg := GetStorageConfig()
	for _, kind := range kinds {
		path := filepath.Join(ObjectDraftPlaneRoot(projectRoot), kind, shard, id+cfg.YAMLExtension)
		if _, statErr := fileutil.Stat(path); statErr == nil {
			return !casBackedObjectID(projectRoot, kind, id)
		}
	}
	return false
}

// IsDraftPlaneOnly reports whether an object ID exists only on the draft plane for this storage instance.
func (f *FileObjectStorage) IsDraftPlaneOnly(id string) bool {
	return IsDraftPlaneOnly(f.projectRoot, "", id)
}

func (f *FileObjectStorage) objectDraftPlanePath(kind, id string) string {
	return ObjectDraftPlanePath(f.projectRoot, kind, id)
}

func (f *FileObjectStorage) objectDraftPlaneExists(kind, id string) bool {
	return ObjectDraftPlaneExists(f.projectRoot, kind, id)
}

// Draft plane next-status / lifecycle progression fields that must appear as editable placeholders
// on draft-plane land (BLI-DRAFT-LAND-TEMPLATE-FILL-001).
var draftPlaneNextStatusFields = map[string]map[string]any{
	objects.KindBacklogItem: {
		objects.FieldKeyProblemStatement:         "",
		objects.FieldKeyAcceptanceConsiderations: "",
		objects.FieldKeyPriority:                 "medium",
		objects.FieldKeyPriorityTier:             "P2",
		objects.FieldKeyPriorityPlanRef:          "",
		objects.FieldKeyMilestoneRefs:            []any{},
		objects.FieldKeyCriteriaRefs:             []any{},
		objects.FieldKeyRequirementRefs:          []any{},
		objects.FieldKeyStakeholders:             []any{},
		objects.FieldKeyEstimatedEffort:          "",
	},
	objects.KindCriteria: {
		objects.FieldKeyCategory:         "",
		objects.FieldKeyDescription:      "",
		objects.FieldKeyValidationMethod: "automated_test",
	},
	objects.KindTechnicalDebt: {
		objects.FieldKeyImpact:          "",
		"remediation":                   "",
		objects.FieldKeyPriority:        "medium",
		objects.FieldKeyEstimatedEffort: "",
	},
	objects.KindDocEntry: {
		objects.FieldKeyCategory: "",
		objects.FieldKeyPath:     "",
	},
	objects.KindRequirement: {
		objects.FieldKeyPriority:     "medium",
		objects.FieldKeyGoalRefs:     []any{},
		objects.FieldKeyStakeholders: []any{},
	},
}

var draftPlaneTemplateCache sync.Map // map[string]map[string]any

// getDraftPlaneTemplateForKind returns a cached in-memory template skeleton for the kind.
// Pre-computing and caching the specialized fields eliminates repetitive reflection and schema traversals.
func getDraftPlaneTemplateForKind(kind string) map[string]any {
	if val, ok := draftPlaneTemplateCache.Load(kind); ok {
		cached := val.(map[string]any)
		cp := make(map[string]any, len(cached))
		for k, v := range cached {
			cp[k] = v
		}
		return cp
	}

	merged := make(map[string]any)
	var kf *objects.KindFields
	if fr := objects.GetGlobalFieldRegistry(); fr != nil {
		if loaded, ok := fr.GetFieldsForKindIfLoaded(kind); ok && loaded != nil {
			kf = loaded
		} else if loaded, err := fr.GetFieldsForKind(kind); err == nil {
			kf = loaded
		}
	}
	if kf != nil {
		merged[objects.FieldKeyTitle] = ""
		merged[objects.FieldKeyDescription] = ""
		merged[ConstVersionContext] = "default"

		for _, field := range kf.SpecializedFields {
			if isDraftPlaneMembraneField(field.Name) || field.Name == objects.FieldKeyKind || field.Name == objects.FieldKeyID || field.Name == objects.FieldKeySchemaVersion {
				continue
			}
			if !field.Required && (field.Type == "enum" || len(field.EnumValues) > 0) {
				continue
			}
			placeholder := draftFieldPlaceholder(field)
			if placeholder != nil {
				merged[field.Name] = placeholder
			}
		}
	}

	if nextFields, ok := draftPlaneNextStatusFields[kind]; ok {
		for k, v := range nextFields {
			merged[k] = v
		}
	}

	if kf != nil {
		toStore := make(map[string]any, len(merged))
		for k, v := range merged {
			toStore[k] = v
		}
		draftPlaneTemplateCache.Store(kind, toStore)
	}

	return merged
}

// isDraftPlaneMembraneField identifies fields that belong strictly to CAS origination/provenance
// and must NOT be written to preliminary drafts on the draft plane.
func isDraftPlaneMembraneField(fieldName string) bool {
	switch fieldName {
	case objects.FieldKeyCreatedAt,
		objects.FieldKeyCreatedBy,
		objects.FieldKeyUpdatedAt,
		objects.FieldKeyUpdatedBy,
		"hash",
		"cas_address":
		return true
	default:
		return false
	}
}

func draftFieldPlaceholder(field objects.FieldInfo) any {
	if len(field.EnumValues) > 0 {
		return field.EnumValues[0]
	}
	switch field.Type {
	case "datetime", "date":
		return nil // Dates/datetimes cannot be empty strings in schema validation; omit until provided
	case "string", "text", "reference":
		return ""
	case "integer", "number":
		return 0
	case "boolean", "bool":
		return false
	case "list", "array":
		return []any{}
	case "map", "object":
		return map[string]any{}
	case "enum":
		if len(field.EnumValues) > 0 {
			return field.EnumValues[0]
		}
		return ""
	default:
		return ""
	}
}

func (f *FileObjectStorage) mergeDraftPlaneTemplate(kind, id string, data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	var incoming map[string]any
	if err := yaml.Unmarshal(data, &incoming); err != nil || incoming == nil {
		return data
	}

	merged := getDraftPlaneTemplateForKind(kind)

	// 3. Seed canonical lifecycle origin status if not already populated
	if originStatus, err := f.GetLifecycleLoader().GetOriginStatus(kind); err == nil && originStatus != "" {
		merged[objects.FieldKeyStatus] = originStatus
	} else if _, hasStatus := merged[objects.FieldKeyStatus]; !hasStatus {
		merged[objects.FieldKeyStatus] = objects.ObjectStatusConceptual
	}

	// 4. Layer incoming user-supplied fields on top (user values strictly win)
	for k, v := range incoming {
		if isDraftPlaneMembraneField(k) {
			continue // Do not allow premature CAS timestamps on the draft plane
		}
		merged[k] = v
	}

	// Ensure core identity fields
	if _, ok := merged[objects.FieldKeyKind]; !ok || merged[objects.FieldKeyKind] == "" {
		merged[objects.FieldKeyKind] = kind
	}
	if _, ok := merged[objects.FieldKeyID]; !ok || merged[objects.FieldKeyID] == "" {
		merged[objects.FieldKeyID] = id
	}
	if _, ok := merged[objects.FieldKeySchemaVersion]; !ok {
		merged[objects.FieldKeySchemaVersion] = objects.DefaultSchemaVersion
	}

	// Strip any membrane provenance fields that might have leaked into merged
	delete(merged, objects.FieldKeyCreatedAt)
	delete(merged, objects.FieldKeyCreatedBy)
	delete(merged, objects.FieldKeyUpdatedAt)
	delete(merged, objects.FieldKeyUpdatedBy)
	delete(merged, "hash")
	delete(merged, "cas_address")

	marshaled, err := f.yamlMarshalForPersistence(merged)
	if err != nil {
		return data
	}
	return marshaled
}

func (f *FileObjectStorage) WriteObjectToDraftPlane(id, kind string, data []byte) error {
	path := f.objectDraftPlanePath(kind, id)
	if f.projectRoot == emptyValue {
		return errfmt.Errorf("object draft plane: empty project root")
	}
	if len(data) == 0 {
		return errfmt.Errorf("object draft plane: empty payload for %s", id)
	}

	data = f.mergeDraftPlaneTemplate(kind, id, data)

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
