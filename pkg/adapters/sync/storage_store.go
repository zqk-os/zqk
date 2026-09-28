package sync

import (
	"context"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// StorageKernelStore adapts storage.ObjectStorageProvider to the KernelStore interface.
type StorageKernelStore struct {
	sp     storage.ObjectStorageProvider
	secCtx *pkgctx.SecurityContext
}

// NewStorageKernelStore creates a new StorageKernelStore.
func NewStorageKernelStore(sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext) *StorageKernelStore {
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	return &StorageKernelStore{
		sp:     sp,
		secCtx: secCtx,
	}
}

// GetBacklogItemByExternalID retrieves a backlog item matching source and external ID.
func (s *StorageKernelStore) GetBacklogItemByExternalID(ctx context.Context, source ExternalSource, extID string) (*BacklogItemSyncData, error) {
	if s.sp == nil {
		return nil, nil
	}

	filter := storage.ListFilter{
		Kind: objects.KindBacklogItem,
	}

	res, err := s.sp.List(ctx, s.secCtx, nil, filter)
	if err != nil {
		return nil, err
	}

	targetExtID := strings.TrimSpace(extID)
	for _, obj := range res.Objects {
		origSys, _ := obj[objects.FieldKeyOriginSystem].(string)
		id, _ := obj[objects.FieldKeyID].(string)

		if strings.EqualFold(origSys, string(source)) {
			// Check if ID matches convention (e.g. BLI-GH-123) or questions/notes contains extID
			if strings.HasSuffix(id, "-"+targetExtID) || id == targetExtID {
				return mapObjectToSyncData(obj, source, targetExtID), nil
			}
		}
	}

	return nil, nil
}

// UpsertBacklogItem creates or updates a BacklogItemSyncData in kernel storage.
func (s *StorageKernelStore) UpsertBacklogItem(ctx context.Context, item *BacklogItemSyncData) error {
	if s.sp == nil || item == nil {
		return nil
	}

	existing, err := s.sp.Read(ctx, s.secCtx, item.ID)
	if err != nil || existing == nil {
		// Create new
		newObj := map[string]any{
			objects.FieldKeyID:               item.ID,
			objects.FieldKeyKind:             objects.KindBacklogItem,
			objects.FieldKeyTitle:            item.Title,
			objects.FieldKeyDescription:      item.Description,
			objects.FieldKeyProblemStatement: item.ProblemStatement,
			objects.FieldKeyStatus:           item.Status,
			objects.FieldKeyPriority:         item.Priority,
			objects.FieldKeyPriorityTier:     item.PriorityTier,
			objects.FieldKeyOriginSystem:     string(item.ExternalSource),
			objects.FieldKeyEstimatedEffort:  "1h",
		}
		if item.PriorityPlanRef != "" {
			newObj[objects.FieldKeyPriorityPlanRef] = item.PriorityPlanRef
		}
		createCtx := pkgctx.WithPromoteOnCreate(ctx)
		return s.sp.Create(createCtx, s.secCtx, newObj)
	}

	// Update existing
	updates := map[string]any{
		objects.FieldKeyTitle:            item.Title,
		objects.FieldKeyDescription:      item.Description,
		objects.FieldKeyProblemStatement: item.ProblemStatement,
		objects.FieldKeyPriority:         item.Priority,
		objects.FieldKeyPriorityTier:     item.PriorityTier,
	}
	if item.PriorityPlanRef != "" {
		updates[objects.FieldKeyPriorityPlanRef] = item.PriorityPlanRef
	}
	return s.sp.Update(ctx, s.secCtx, item.ID, updates)
}

// ListBacklogItems lists all backlog items from kernel storage as BacklogItemSyncData.
func (s *StorageKernelStore) ListBacklogItems(ctx context.Context) ([]*BacklogItemSyncData, error) {
	if s.sp == nil {
		return nil, nil
	}

	filter := storage.ListFilter{
		Kind: objects.KindBacklogItem,
	}

	res, err := s.sp.List(ctx, s.secCtx, nil, filter)
	if err != nil {
		return nil, err
	}

	var results []*BacklogItemSyncData
	for _, obj := range res.Objects {
		source := ExternalSource(objects.GetString(obj, objects.FieldKeyOriginSystem))
		results = append(results, mapObjectToSyncData(obj, source, ""))
	}
	return results, nil
}

func mapObjectToSyncData(obj map[string]any, source ExternalSource, extID string) *BacklogItemSyncData {
	id := objects.GetString(obj, objects.FieldKeyID)
	if extID == "" {
		parts := strings.Split(id, "-")
		if len(parts) > 1 {
			extID = parts[len(parts)-1]
		}
	}

	updatedAt := time.Now()
	if tStr := objects.GetString(obj, objects.FieldKeyUpdatedAt); tStr != "" {
		if parsed, err := time.Parse(time.RFC3339, tStr); err == nil {
			updatedAt = parsed
		}
	}

	return &BacklogItemSyncData{
		ID:               id,
		Title:            objects.GetString(obj, objects.FieldKeyTitle),
		Description:      objects.GetString(obj, objects.FieldKeyDescription),
		ProblemStatement: objects.GetString(obj, objects.FieldKeyProblemStatement),
		Status:           objects.GetString(obj, objects.FieldKeyStatus),
		Priority:         objects.GetString(obj, objects.FieldKeyPriority),
		PriorityTier:     objects.GetString(obj, objects.FieldKeyPriorityTier),
		PriorityPlanRef:  objects.GetString(obj, objects.FieldKeyPriorityPlanRef),
		ExternalSource:   source,
		ExternalID:       extID,
		UpdatedAt:        updatedAt,
	}
}
