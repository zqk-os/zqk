package docman

import (
	"context"
	"maps"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

const (
	kindDocEntry                          = objects.KindDocEntry
	logFieldID                            = "id"
	warnReadDocEntryMsg                   = "Failed to read doc_entry for update"
	warnUpdateDocEntryMsg                 = "Failed to update doc_entry"
	debugUpdatedMissingReferenceFieldsMsg = "Updated doc_entry with missing reference fields"
	errListDocEntriesFmt                  = "failed to list doc_entries: %w"
)

// UpdateExistingEntries updates all existing doc_entry objects to include missing reference fields
func (r *Registry) UpdateExistingEntries(ctx context.Context, profile string) (int, error) {
	logger := logging.GetLoggerFromProfile(profile)
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// List all doc_entry objects
	filter := storage.ListFilter{
		Kind: kindDocEntry,
	}
	results, err := r.storageProvider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return 0, errfmt.Errorf(errListDocEntriesFmt, err)
	}

	updated := 0
	for _, obj := range results.Objects {
		needsUpdate := false
		updates := make(map[string]any)

		// Check and add missing reference fields
		if _, hasGoalRefs := obj[objects.FieldKeyGoalRefs]; !hasGoalRefs {
			updates[objects.FieldKeyGoalRefs] = []string{}
			needsUpdate = true
		}
		if _, hasWorkstreamRefs := obj[objects.FieldKeyWorkstreamRefs]; !hasWorkstreamRefs {
			updates[objects.FieldKeyWorkstreamRefs] = []string{}
			needsUpdate = true
		}
		if _, hasMilestoneRefs := obj[objects.FieldKeyMilestoneRefs]; !hasMilestoneRefs {
			updates[objects.FieldKeyMilestoneRefs] = []string{}
			needsUpdate = true
		}
		if _, hasRequirementRefs := obj[objects.FieldKeyRequirementRefs]; !hasRequirementRefs {
			updates[objects.FieldKeyRequirementRefs] = []string{}
			needsUpdate = true
		}

		if needsUpdate {
			id, _ := obj[objects.FieldKeyID].(string)
			// Read existing object and merge updates
			existing, err := r.storageProvider.Read(ctx, secCtx, id)
			if err != nil {
				logging.Fluent(logger).Warn(warnReadDocEntryMsg).
					ObjectID(id).
					WithError(err).
					Log()
				continue
			}

			// Merge updates
			merged := mergeMaps(existing, updates)
			err = r.storageProvider.Update(ctx, secCtx, id, merged)
			if err != nil {
				logging.Fluent(logger).Warn(warnUpdateDocEntryMsg).
					ObjectID(id).
					WithError(err).
					Log()
				continue
			}

			updated++
			logging.Fluent(logger).Debug(debugUpdatedMissingReferenceFieldsMsg).
				ObjectID(id).
				Log()
		}
	}

	return updated, nil
}

// Helper function to merge maps
func mergeMaps(base, updates map[string]any) map[string]any {
	result := make(map[string]any)
	maps.Copy(result, base)
	maps.Copy(result, updates)
	return result
}
