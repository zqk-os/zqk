package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/federation"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// CapacityScalingHandler automatically updates capacity_advertisement objects
// based on real-time local resource availability.
type CapacityScalingHandler struct {
	storage     storage.ObjectStorageProvider
	projectRoot string
}

// NewCapacityScalingHandler creates a new CapacityScalingHandler.
// NewCapacityScalingHandler creates a new capacity scaling handler
func NewCapacityScalingHandler(sp storage.ObjectStorageProvider, projectRoot string) CapacityScalingHandlerInterface {
	return &CapacityScalingHandler{
		storage:     sp,
		projectRoot: projectRoot,
	}
}

// Execute performs the capacity scaling logic.
func (h *CapacityScalingHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	// 1. Resolve local identity
	kernelID, err := federation.ResolveKernelID(h.projectRoot)
	if err != nil {
		return err
	}

	// 2. Sample current resource availability
	stats := metrics.GetResourceAvailability()

	// 3. Update 'compute' advertisement
	if err := h.upsertAdvertisement(ctx, kernelID, "compute", float64(stats.AvailableCompute), "goroutines"); err != nil {
		return errfmt.Newf("update compute advertisement").Wrap(err)
	}

	// 4. Update 'storage' advertisement (Future: check real disk quota)
	if err := h.upsertAdvertisement(ctx, kernelID, "storage", 10.0, "GB"); err != nil {
		return errfmt.Newf("update storage advertisement").Wrap(err)
	}

	return nil
}

func (h *CapacityScalingHandler) upsertAdvertisement(ctx context.Context, kernelID, resType string, quantity float64, units string) error {
	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}

	// ID is deterministic for a given kernel and resource type to prevent duplicates
	id := fmt.Sprintf("CAP-%s-%s", strings.ToUpper(kernelID), strings.ToUpper(resType))

	// Check if already exists
	existing, err := h.storage.Read(ctx, secCtx, id)
	if err == nil {
		// Update existing
		existing[objects.FieldKeyQuantity] = quantity
		existing[objects.FieldKeyUnits] = units
		existing[objects.FieldKeyUpdatedAt] = time.Now().Format(time.RFC3339)
		return h.storage.Update(ctx, secCtx, id, existing)
	}

	// Create new
	obj := map[string]any{
		objects.FieldKeyID:                id,
		objects.FieldKeyKind:              objects.KindCapacityAdvertisement,
		objects.FieldKeyTitle:             fmt.Sprintf("Autonomous Offer: %s", resType),
		objects.FieldKeyProviderKernelRef: kernelID,
		objects.FieldKeyResourceType:      resType,
		objects.FieldKeyQuantity:          quantity,
		objects.FieldKeyUnits:             units,
		objects.FieldKeyStatus:            "implemented",
	}

	// Use synchronous creation
	syncCtx := storage.WithSyncCreateForKind(ctx, objects.KindCapacityAdvertisement)
	return h.storage.Create(syncCtx, secCtx, obj)
}
