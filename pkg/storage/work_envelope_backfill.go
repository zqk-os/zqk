package storage

import (
	"context"
	"sort"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// WorkEnvelopeBackfillHit is one effort_aware work-done object missing completed_at.
type WorkEnvelopeBackfillHit struct {
	ObjectID string `json:"object_id"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	Stamp    string `json:"completed_at"`
}

// WorkEnvelopeBackfillResult is the dry-run / apply report for completed_at backfill.
type WorkEnvelopeBackfillResult struct {
	DryRun  bool                      `json:"dry_run"`
	Scanned int                       `json:"scanned"`
	Planned int                       `json:"planned"`
	Applied int                       `json:"applied"`
	Kinds   []string                  `json:"kinds"`
	Errors  []string                  `json:"errors,omitempty"`
	Hits    []WorkEnvelopeBackfillHit `json:"hits"`
}

func effortAwareKindNames() []string {
	loader := objects.GetGlobalSpecLoader()
	names, err := loader.DiscoverOntologies()
	if err != nil {
		return nil
	}
	cfg := objects.GetGlobalKindMappingsConfig()
	out := make([]string, 0)
	for _, k := range names {
		if cfg != nil && cfg.ShouldSkipSpec(k) {
			continue
		}
		if objects.KindHasNamedTrait(k, "effort_aware") {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// BackfillWorkEnvelopeCompletedAt stamps completed_at from historical updated_at
// on effort_aware work-done objects that never received the work clock.
// Does not run on object get. TRACK: BLI-KERNEL-WORK-ENVELOPE-001
func BackfillWorkEnvelopeCompletedAt(
	ctx context.Context,
	sec *pkgctx.SecurityContext,
	store ObjectStorageProvider,
	dryRun bool,
	limit int,
) WorkEnvelopeBackfillResult {
	out := WorkEnvelopeBackfillResult{DryRun: dryRun, Hits: []WorkEnvelopeBackfillHit{}}
	if store == nil {
		out.Errors = append(out.Errors, "storage is nil")
		return out
	}
	kinds := effortAwareKindNames()
	out.Kinds = kinds
	storageCtx := pkgctx.NewStorageContext()
	for _, kind := range kinds {
		res, err := store.List(ctx, sec, storageCtx, ListFilter{Kind: kind})
		if err != nil {
			out.Errors = append(out.Errors, kind+": list: "+err.Error())
			continue
		}
		if res == nil {
			continue
		}
		for _, obj := range res.Objects {
			out.Scanned++
			id := strings.TrimSpace(objects.GetString(obj, objects.FieldKeyID))
			if id == emptyValue {
				continue
			}
			stamp, ok := CompletedAtBackfillStamp(kind, obj)
			if !ok {
				continue
			}
			hit := WorkEnvelopeBackfillHit{
				ObjectID: id,
				Kind:     kind,
				Status:   objects.GetString(obj, objects.FieldKeyStatus),
				Stamp:    stamp,
			}
			if limit > 0 && len(out.Hits) >= limit {
				break
			}
			out.Hits = append(out.Hits, hit)
			out.Planned++
			if dryRun {
				continue
			}
			if err := store.Update(ctx, sec, id, map[string]any{objects.FieldKeyCompletedAt: stamp}); err != nil {
				out.Errors = append(out.Errors, id+": update: "+err.Error())
				continue
			}
			out.Applied++
		}
		if limit > 0 && len(out.Hits) >= limit {
			break
		}
	}
	return out
}
