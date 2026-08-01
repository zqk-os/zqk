package meshbroker

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// ComputeBroker finds and dispatches heavy compute tasks to specialized Tool Pods.
type ComputeBroker struct {
	storage storage.ObjectStorageProvider
}

// NewComputeBroker creates a new ComputeBroker.
func NewComputeBroker(sp storage.ObjectStorageProvider) *ComputeBroker {
	return &ComputeBroker{
		storage: sp,
	}
}

// FindCapabilityPod searches for a pod that advertises the required capability.
func (b *ComputeBroker) FindCapabilityPod(ctx context.Context, capabilityType string) (map[string]any, error) {
	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}

	// Search marketplace for an advertisement for this capability
	filter := storage.ListFilter{
		Kind: objects.KindComputeAdvertisement, // Assuming KindComputeAdvertisement is added to objects
		Filters: map[string]any{
			objects.FieldKeyCapabilityType: capabilityType,
		},
	}

	advs, err := b.storage.List(ctx, secCtx, nil, filter)
	if err != nil || len(advs.Objects) == 0 {
		return nil, errfmt.Errorf("compute pod with capability %s not found in mesh market", capabilityType)
	}

	// For now, return the first available pod
	return advs.Objects[0], nil
}
