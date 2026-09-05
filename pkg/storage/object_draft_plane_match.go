package storage

import (
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/process"
	"github.com/lanceman/zqk/pkg/storage/filecas"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TRACK: REDACTED — shared draft-plane match for sweep + promote.

// ObjectDraftPlaneMatchOptions filters draft-plane candidates (shared by sweep and promote).
type ObjectDraftPlaneMatchOptions struct {
	Kind      string
	IDPrefix  string
	Status    string
	OlderThan time.Duration
	All       bool
	Max       int // cap on matched candidates (0 = unlimited when All)
	// IncludeCASBacked includes draft copies whose ID is already materialized in CAS.
	// Sweep enables this to reconcile invalid dual-plane state; promote leaves it false.
	IncludeCASBacked bool
}

// ObjectDraftPlaneCandidate is one draft-plane row after filters (or a skip record).
type ObjectDraftPlaneCandidate struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Status string `json:"status,omitempty"`
	Path   string `json:"path,omitempty"`
	Action string `json:"action,omitempty"` // matched | skipped_filter | skipped_cas_backed | skipped_max
	Reason string `json:"reason,omitempty"`
}

// MatchObjectDraftPlane enumerates drafts with the same filter shape as sweep/promote.
// Fail-closed apply gates (All / Max+filters) are enforced by callers that mutate.
func MatchObjectDraftPlane(projectRoot string, opts ObjectDraftPlaneMatchOptions) (matched []ObjectDraftPlaneCandidate, skipped []ObjectDraftPlaneCandidate, inv ObjectDraftPlaneInventory, err error) {
	if projectRoot == emptyValue {
		return nil, nil, ObjectDraftPlaneInventory{}, errfmt.Errorf("object draft plane match: empty project root")
	}
	inv = InventoryObjectDraftPlane(projectRoot)
	kinds, err := draftPlaneKinds(projectRoot, opts.Kind)
	if err != nil {
		return nil, nil, inv, err
	}
	cutoff := time.Time{}
	if opts.OlderThan > 0 {
		cutoff = time.Now().Add(-opts.OlderThan)
	}
	for _, kind := range kinds {
		ids, listErr := ListObjectDraftPlaneIDs(projectRoot, kind)
		if listErr != nil {
			return nil, nil, inv, listErr
		}
		for _, id := range ids {
			process.TouchMeaningfulActivity()
			item := ObjectDraftPlaneCandidate{ID: id, Kind: kind, Path: ObjectDraftPlanePath(projectRoot, kind, id)}
			if opts.IDPrefix != emptyValue && !strings.HasPrefix(id, opts.IDPrefix) {
				item.Action = "skipped_filter"
				item.Reason = "id_prefix"
				skipped = append(skipped, item)
				continue
			}
			status, mtime, metaErr := readDraftPlaneMeta(projectRoot, kind, id)
			if metaErr != nil {
				item.Action = "skipped_filter"
				item.Reason = metaErr.Error()
				skipped = append(skipped, item)
				continue
			}
			item.Status = status
			if opts.Status != emptyValue && status != opts.Status {
				item.Action = "skipped_filter"
				item.Reason = "status"
				skipped = append(skipped, item)
				continue
			}
			if !cutoff.IsZero() && (mtime.IsZero() || mtime.After(cutoff)) {
				item.Action = "skipped_filter"
				item.Reason = "older_than"
				skipped = append(skipped, item)
				continue
			}
			if casBackedObjectID(projectRoot, kind, id) && !opts.IncludeCASBacked {
				item.Action = "skipped_cas_backed"
				item.Reason = "cas_index_has_id"
				skipped = append(skipped, item)
				continue
			}
			if opts.Max > 0 && len(matched) >= opts.Max {
				item.Action = "skipped_max"
				item.Reason = "max"
				skipped = append(skipped, item)
				continue
			}
			item.Action = "matched"
			matched = append(matched, item)
		}
	}
	return matched, skipped, inv, nil
}

// RequireDraftPlaneApplyGate fails closed when mutating without --all (or --max + filters).
func RequireDraftPlaneApplyGate(opts ObjectDraftPlaneMatchOptions, verb string) error {
	if opts.All {
		return nil
	}
	if opts.Max <= 0 {
		return errfmt.Errorf("object draft plane %s: refusing apply without --dry-run or --all (or --max with filters)", verb)
	}
	if opts.Kind == emptyValue && opts.IDPrefix == emptyValue && opts.Status == emptyValue && opts.OlderThan <= 0 {
		return errfmt.Errorf("object draft plane %s: --max without filters still requires --all (fail closed)", verb)
	}
	return nil
}

func draftPlaneKinds(projectRoot, onlyKind string) ([]string, error) {
	if onlyKind != emptyValue {
		return []string{onlyKind}, nil
	}
	root := ObjectDraftPlaneRoot(projectRoot)
	entries, err := fileutil.ReadDir(root)
	if fileutil.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errfmt.Newf("object draft plane match: list root").Wrap(err)
	}
	var kinds []string
	for _, e := range entries {
		if e.IsDir() {
			kinds = append(kinds, e.Name())
		}
	}
	return kinds, nil
}

func readDraftPlaneMeta(projectRoot, kind, id string) (status string, mtime time.Time, err error) {
	path := ObjectDraftPlanePath(projectRoot, kind, id)
	fi, statErr := fileutil.Stat(path)
	if statErr != nil {
		return "", time.Time{}, statErr
	}
	mtime = fi.ModTime()
	data, readErr := fileutil.ReadFile(path)
	if readErr != nil {
		return "", mtime, readErr
	}
	var obj map[string]any
	if uErr := yaml.Unmarshal(data, &obj); uErr != nil {
		return "", mtime, uErr
	}
	status, _ = obj[objects.FieldKeyStatus].(string)
	return status, mtime, nil
}

// casBackedObjectID reports whether the CAS index already maps this id (post-membrane).
func casBackedObjectID(projectRoot, kind, id string) bool {
	if projectRoot == emptyValue || kind == emptyValue || id == emptyValue {
		return false
	}
	if StreamStorageEnabledForKind(kind) {
		return false
	}
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		return false
	}
	kindDir := filepath.Join(datacell.ProcessPrimaryDir(projectRoot), dirName)
	if rel := paths.GetPathAlias(projectRoot, dirName); rel != emptyValue {
		kindDir = filepath.Join(projectRoot, rel)
	}
	cas := filecas.NewContentAddressableStorage(kindDir, kind)
	if cas == nil {
		return false
	}
	if _, err := cas.GetHashForID(id); err != nil {
		return false
	}
	filePath, err := cas.GetFilePathForID(id)
	if err != nil {
		return false
	}
	_, statErr := fileutil.Stat(filePath)
	return statErr == nil
}
