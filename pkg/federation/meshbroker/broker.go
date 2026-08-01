package meshbroker

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/federation"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// SkillBroker finds and leases skills from the mesh marketplace.
type SkillBroker struct {
	storage     storage.ObjectStorageProvider
	handshaker  *federation.MCPHandshaker
	projectRoot string
}

// NewSkillBroker creates a new SkillBroker.
func NewSkillBroker(sp storage.ObjectStorageProvider, handshaker *federation.MCPHandshaker, projectRoot string) *SkillBroker {
	return &SkillBroker{
		storage:     sp,
		handshaker:  handshaker,
		projectRoot: projectRoot,
	}
}

// EnsureSkill attempts to find and lease a skill if it's not available locally.
func (b *SkillBroker) EnsureSkill(ctx context.Context, skillID string) (*federation.RemoteSkill, error) {
	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}

	// 1. Check local storage for existing lease
	leaseFilter := storage.ListFilter{
		Kind: objects.KindZqkSession,
		Filters: map[string]any{
			objects.FieldKeySessionMode: "federated_lease",
			objects.FieldKeyResourceRef: skillID,
			objects.FieldKeyStatus:      "implemented",
		},
	}

	leases, err := b.storage.List(ctx, secCtx, nil, leaseFilter)
	if err == nil && len(leases.Objects) > 0 {
		// Existing lease found
		return b.mapLeaseToRemoteSkill(ctx, leases.Objects[0]), nil
	}

	// 2. Search marketplace for an advertisement
	advFilter := storage.ListFilter{
		Kind: objects.KindCapacityAdvertisement,
		Filters: map[string]any{
			objects.FieldKeyResourceID:   skillID,
			objects.FieldKeyResourceType: "skill",
		},
	}

	advs, err := b.storage.List(ctx, secCtx, nil, advFilter)
	if err != nil || len(advs.Objects) == 0 {
		return nil, errfmt.Errorf("skill %s not found in local kernel or mesh market", skillID)
	}

	// 3. Establish lease with provider
	bestAdv := advs.Objects[0]
	providerKernelID, _ := bestAdv[objects.FieldKeyProviderKernelRef].(string)

	// Find provider endpoint
	kernel, err := b.storage.Read(ctx, secCtx, providerKernelID)
	if err != nil {
		return nil, errfmt.Newf("failed to find provider kernel %s", providerKernelID).Wrap(err)
	}
	endpoint, _ := kernel[objects.FieldKeyEndpoint].(string)

	token := fmt.Sprintf("TOK-%d", time.Now().UnixNano())

	idManager := federation.NewIdentityManager(b.projectRoot)
	localKernelID, _ := idManager.GetKernelID()

	leaseObj := map[string]any{
		objects.FieldKeyKind:              objects.KindZqkSession,
		objects.FieldKeyTitle:             fmt.Sprintf("Auto-Lease: %s", skillID),
		objects.FieldKeyAccountID:         "ACT-SYSTEM",
		objects.FieldKeySessionMode:       "federated_lease",
		objects.FieldKeyAgreementMode:     "grantor_enforced",
		objects.FieldKeyProviderKernelRef: providerKernelID,
		objects.FieldKeyConsumerKernelRef: localKernelID,
		objects.FieldKeyResourceRef:       skillID,
		objects.FieldKeyTokenID:           token,
		objects.FieldKeyStatus:            "implemented",
		objects.FieldKeyExpiresAt:         time.Now().Add(24 * time.Hour).Format(time.RFC3339),
	}

	if err := b.storage.Create(ctx, secCtx, leaseObj); err != nil {
		return nil, errfmt.Newf("failed to establish auto-lease").Wrap(err)
	}

	return &federation.RemoteSkill{
		ID:       skillID,
		Provider: providerKernelID,
		Endpoint: endpoint,
		Token:    token,
	}, nil
}

func (b *SkillBroker) mapLeaseToRemoteSkill(ctx context.Context, lease map[string]any) *federation.RemoteSkill {
	id, _ := lease[objects.FieldKeyResourceRef].(string)
	provider, _ := lease[objects.FieldKeyProviderKernelRef].(string)
	token, _ := lease[objects.FieldKeyTokenID].(string)

	endpoint := ""
	if provider != "" && b.storage != nil {
		secCtx := pkgctx.GetSecurityContext(ctx)
		if secCtx == nil {
			secCtx = pkgctx.NewSystemSecurityContext()
		}
		if kernel, err := b.storage.Read(ctx, secCtx, provider); err == nil {
			endpoint, _ = kernel[objects.FieldKeyEndpoint].(string)
		}
	}

	return &federation.RemoteSkill{
		ID:       id,
		Provider: provider,
		Endpoint: endpoint,
		Token:    token,
	}
}
