package inbox

import (
	"context"
	"fmt"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// StorageInbox implements Inbox using ObjectStorageProvider.
type StorageInbox struct {
	provider storage.ObjectStorageProvider
	secCtx   *pkgctx.SecurityContext
}

// NewStorageInbox creates a new inbox backed by an ObjectStorageProvider.
func NewStorageInbox(provider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext) Inbox {
	return &StorageInbox{
		provider: provider,
		secCtx:   secCtx,
	}
}

func (s *StorageInbox) Submit(env TDEEnvelope) error {
	ctx := context.Background()

	// Ensure the object doesn't already exist.
	exists, err := s.provider.Exists(ctx, s.secCtx, env.ID)
	if err != nil {
		return fmt.Errorf("failed to check existence: %w", err)
	}
	if exists {
		return ErrEnvelopeExists
	}

	env.Status = StatusPending

	obj := map[string]any{
		objects.FieldKeyID:      env.ID,
		objects.FieldKeyKind:    "tde_envelope",
		objects.FieldKeyStatus:  string(env.Status),
		"agent_id":              env.AgentID,
		"intent":                env.Intent,
		"capability_refs":       env.CapabilityRefs,
		objects.FieldKeyPayload: string(env.Payload),
	}

	if err := s.provider.Create(ctx, s.secCtx, obj); err != nil {
		return fmt.Errorf("failed to create envelope: %w", err)
	}

	return nil
}

func (s *StorageInbox) Get(id string) (TDEEnvelope, error) {
	ctx := context.Background()

	obj, err := s.provider.Read(ctx, s.secCtx, id)
	if err != nil {
		// Map the storage error to the inbox not found error
		return TDEEnvelope{}, ErrEnvelopeNotFound
	}

	return s.mapToEnvelope(obj), nil
}

func (s *StorageInbox) ListPending() []TDEEnvelope {
	ctx := context.Background()
	storageCtx := pkgctx.NewStorageContext()

	filter := storage.ListFilter{
		Kind: "tde_envelope",
		Filters: map[string]any{
			objects.FieldKeyStatus: string(StatusPending),
		},
	}

	result, err := s.provider.List(ctx, s.secCtx, storageCtx, filter)
	if err != nil {
		return nil
	}

	var envelopes []TDEEnvelope
	for _, obj := range result.Objects {
		envelopes = append(envelopes, s.mapToEnvelope(obj))
	}
	return envelopes
}

func (s *StorageInbox) Approve(id string) error {
	ctx := context.Background()

	obj, err := s.provider.Read(ctx, s.secCtx, id)
	if err != nil {
		return ErrEnvelopeNotFound
	}

	status, _ := obj[objects.FieldKeyStatus].(string)
	if status != string(StatusPending) {
		return ErrNotPending
	}

	updates := map[string]any{
		objects.FieldKeyStatus: string(StatusApproved),
	}

	if err := s.provider.Update(ctx, s.secCtx, id, updates); err != nil {
		return fmt.Errorf("failed to approve envelope: %w", err)
	}

	return nil
}

func (s *StorageInbox) Reject(id string, reason string) error {
	ctx := context.Background()

	obj, err := s.provider.Read(ctx, s.secCtx, id)
	if err != nil {
		return ErrEnvelopeNotFound
	}

	status, _ := obj[objects.FieldKeyStatus].(string)
	if status != string(StatusPending) {
		return ErrNotPending
	}

	updates := map[string]any{
		objects.FieldKeyStatus: string(StatusRejected),
		"reject_reason":        reason,
	}

	if err := s.provider.Update(ctx, s.secCtx, id, updates); err != nil {
		return fmt.Errorf("failed to reject envelope: %w", err)
	}

	return nil
}

func (s *StorageInbox) mapToEnvelope(obj map[string]any) TDEEnvelope {
	env := TDEEnvelope{}

	if id, ok := obj[objects.FieldKeyID].(string); ok {
		env.ID = id
	}
	if agentID, ok := obj["agent_id"].(string); ok {
		env.AgentID = agentID
	}
	if intent, ok := obj["intent"].(string); ok {
		env.Intent = intent
	}

	// capability_refs can come back as []any or []string depending on storage backend implementation
	if capRefsAny, ok := obj["capability_refs"].([]any); ok {
		for _, ref := range capRefsAny {
			if strRef, ok := ref.(string); ok {
				env.CapabilityRefs = append(env.CapabilityRefs, strRef)
			}
		}
	} else if capRefsStr, ok := obj["capability_refs"].([]string); ok {
		env.CapabilityRefs = capRefsStr
	}

	if payload, ok := obj[objects.FieldKeyPayload].(string); ok {
		env.Payload = []byte(payload)
	}
	if status, ok := obj[objects.FieldKeyStatus].(string); ok {
		env.Status = EnvelopeStatus(status)
	}
	if reason, ok := obj["reject_reason"].(string); ok {
		env.RejectReason = reason
	}

	return env
}
