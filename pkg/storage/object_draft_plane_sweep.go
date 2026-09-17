package storage

import (
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TRACK: BLI-1785827957031623000-b08b9791 — draft-plane sweeper easy button.

// ObjectDraftPlaneSweepOptions filters draft-plane candidates for inventory/delete.
type ObjectDraftPlaneSweepOptions struct {
	Kind      string        // empty = all kinds under object_drafts
	IDPrefix  string        // optional id prefix match
	Status    string        // optional exact status match (from draft YAML)
	OlderThan time.Duration // 0 = no age filter; based on draft file mtime
	DryRun    bool          // list only; do not delete
	All       bool          // required to delete (with DryRun=false) unless Max > 0 with explicit filters
	Max       int           // optional cap on deletions (0 = unlimited when All)
}

// ObjectDraftPlaneSweepItem is one draft-plane id considered by SweepObjectDraftPlane.
type ObjectDraftPlaneSweepItem struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Status string `json:"status,omitempty"`
	Path   string `json:"path,omitempty"`
	Action string `json:"action"` // would_delete | deleted | skipped_cas_backed | skipped_filter | skipped_max
	Reason string `json:"reason,omitempty"`
}

// ObjectDraftPlaneSweepResult is the structured sweep outcome for CLI FormatOutput.
type ObjectDraftPlaneSweepResult struct {
	DryRun    bool                        `json:"dry_run"`
	Matched   int                         `json:"matched"`
	Deleted   int                         `json:"deleted"`
	Skipped   int                         `json:"skipped"`
	Items     []ObjectDraftPlaneSweepItem `json:"items"`
	DraftRoot string                      `json:"draft_root"`
	Inventory ObjectDraftPlaneInventory   `json:"inventory_before"`
}

// SweepObjectDraftPlane enumerates `.zqk/object_drafts` and optionally deletes draft files.
// CAS objects are never deleted; a duplicate draft for an already materialized ID is eligible
// so sweep can reconcile the dual-plane state reported as blocking by system check.
func SweepObjectDraftPlane(projectRoot string, opts ObjectDraftPlaneSweepOptions) (*ObjectDraftPlaneSweepResult, error) {
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("object draft plane sweep: empty project root")
	}
	out := &ObjectDraftPlaneSweepResult{
		DryRun:    opts.DryRun,
		DraftRoot: ObjectDraftPlaneRoot(projectRoot),
		Items:     nil,
	}
	if !opts.DryRun {
		if err := RequireDraftPlaneApplyGate(ObjectDraftPlaneMatchOptions{
			Kind: opts.Kind, IDPrefix: opts.IDPrefix, Status: opts.Status,
			OlderThan: opts.OlderThan, All: opts.All, Max: opts.Max,
		}, "sweep"); err != nil {
			return nil, err
		}
	}

	matched, skipped, inv, err := MatchObjectDraftPlane(projectRoot, ObjectDraftPlaneMatchOptions{
		Kind: opts.Kind, IDPrefix: opts.IDPrefix, Status: opts.Status,
		OlderThan: opts.OlderThan, All: opts.All, Max: opts.Max, IncludeCASBacked: true,
	})
	if err != nil {
		return nil, err
	}
	out.Inventory = inv
	out.Matched = len(matched)
	out.Skipped = len(skipped)

	for _, s := range skipped {
		out.Items = append(out.Items, ObjectDraftPlaneSweepItem{
			ID: s.ID, Kind: s.Kind, Status: s.Status, Path: s.Path,
			Action: s.Action, Reason: s.Reason,
		})
	}
	for _, m := range matched {
		item := ObjectDraftPlaneSweepItem{
			ID: m.ID, Kind: m.Kind, Status: m.Status, Path: m.Path,
		}
		if opts.DryRun {
			item.Action = "would_delete"
			out.Items = append(out.Items, item)
			continue
		}
		if err := fileutil.Remove(item.Path); err != nil && !fileutil.IsNotExist(err) {
			return out, errfmt.Newf("object draft plane sweep: delete %s", m.ID).Wrap(err)
		}
		_ = fileutil.Remove(filepath.Dir(item.Path))
		_ = fileutil.Remove(filepath.Dir(filepath.Dir(item.Path)))
		item.Action = "deleted"
		out.Deleted++
		out.Items = append(out.Items, item)
	}
	return out, nil
}
