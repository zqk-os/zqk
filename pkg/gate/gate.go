package gate

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/storage/audit"
)

// GenerateApprovalID creates a unique approval identifier.
func GenerateApprovalID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("APR-%d-%s", time.Now().UnixNano(), hex.EncodeToString(b))
}

// ComputePayloadDigest returns the hex SHA256 digest of payloadBytes.
func ComputePayloadDigest(payloadBytes []byte) string {
	sum := sha256.Sum256(payloadBytes)
	return hex.EncodeToString(sum[:])
}

// StageApproval stages a mutation approval request in the Knowledge Kernel CAS.
func StageApproval(
	ctx context.Context,
	store storage.ObjectStorageProvider,
	reqType string,
	target string,
	payloadBytes []byte,
	createdBy string,
) (*ApprovalRequest, error) {
	if store == nil {
		return nil, errfmt.Errorf("storage provider is required")
	}
	if reqType == "" {
		return nil, errfmt.Errorf("approval type is required")
	}
	if target == "" {
		return nil, errfmt.Errorf("approval target is required")
	}
	if len(payloadBytes) == 0 {
		return nil, errfmt.Errorf("payload cannot be empty")
	}
	if createdBy == "" {
		createdBy = "system"
	}

	digest := ComputePayloadDigest(payloadBytes)
	now := CurrentTimestamp()
	approvalID := GenerateApprovalID()

	req := &ApprovalRequest{
		ID:          approvalID,
		Kind:        KindApprovalRequest,
		Title:       fmt.Sprintf("Gate Approval Request: %s (%s)", reqType, target),
		Type:        reqType,
		Target:      target,
		Payload:     string(payloadBytes),
		PayloadHash: digest,
		Hash:        digest,
		Status:      StatusStaged,
		CreatedAt:   now,
		CreatedBy:   createdBy,
		UpdatedAt:   now,
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	if err := store.Create(ctx, secCtx, req.ToMap()); err != nil {
		return nil, errfmt.Newf("failed to stage approval in storage: %v", err)
	}

	return req, nil
}

// GetApproval retrieves an approval request by its ID.
func GetApproval(
	ctx context.Context,
	store storage.ObjectStorageProvider,
	approvalID string,
) (*ApprovalRequest, error) {
	if store == nil {
		return nil, errfmt.Errorf("storage provider is required")
	}
	if approvalID == "" {
		return nil, errfmt.Errorf("approval ID is required")
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	raw, err := store.Read(ctx, secCtx, approvalID)
	if err != nil {
		return nil, errfmt.Newf("approval request %q not found: %v", approvalID, err)
	}

	return FromMap(raw)
}

// Approve validates payload digest, marks status approved with reviewer and timestamp,
// and records an approval entry to the audit ledger.
func Approve(
	ctx context.Context,
	store storage.ObjectStorageProvider,
	projectRoot string,
	approvalID string,
	reviewer string,
) (*ApprovalRequest, error) {
	if store == nil {
		return nil, errfmt.Errorf("storage provider is required")
	}
	if approvalID == "" {
		return nil, errfmt.Errorf("approval ID is required")
	}
	if reviewer == "" {
		return nil, errfmt.Errorf("reviewer is required for approval")
	}

	appReq, err := GetApproval(ctx, store, approvalID)
	if err != nil {
		return nil, err
	}

	// Validate payload digest against stored payload (tamper prevention)
	currentDigest := ComputePayloadDigest([]byte(appReq.Payload))
	expectedDigest := appReq.PayloadHash
	if expectedDigest == "" {
		expectedDigest = appReq.Hash
	}
	if currentDigest != expectedDigest {
		return nil, errfmt.Errorf("tamper detected: stored payload digest %s does not match staged digest %s", currentDigest, expectedDigest)
	}

	if appReq.Status == StatusRejected {
		return nil, errfmt.Errorf("cannot approve rejected request %s (rejected by %s: %s)", approvalID, appReq.Reviewer, appReq.Reason)
	}

	now := CurrentTimestamp()
	updates := map[string]any{
		objects.FieldKeyStatus: StatusApproved,
		"reviewer":             reviewer,
		"reviewed_at":          now,
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	if err := store.Update(ctx, secCtx, approvalID, updates); err != nil {
		return nil, errfmt.Newf("failed to update approval status: %v", err)
	}

	appReq.Status = StatusApproved
	appReq.Reviewer = reviewer
	appReq.ReviewedAt = now
	appReq.UpdatedAt = now

	// Record approval entry to audit ledger
	recordAuditLedger(ctx, store, projectRoot, secCtx, AuditEventTypeGateApproval, fmt.Sprintf("Gate approval granted for %s", approvalID), reviewer, map[string]any{
		"approval_id":  approvalID,
		"reviewer":     reviewer,
		"type":         appReq.Type,
		"target":       appReq.Target,
		"payload_hash": appReq.PayloadHash,
		"reviewed_at":  now,
	})

	return appReq, nil
}

// Reject marks an approval request as rejected with reviewer and reason,
// and records a rejection entry to the audit ledger.
func Reject(
	ctx context.Context,
	store storage.ObjectStorageProvider,
	projectRoot string,
	approvalID string,
	reviewer string,
	reason string,
) (*ApprovalRequest, error) {
	if store == nil {
		return nil, errfmt.Errorf("storage provider is required")
	}
	if approvalID == "" {
		return nil, errfmt.Errorf("approval ID is required")
	}
	if reviewer == "" {
		return nil, errfmt.Errorf("reviewer is required for rejection")
	}
	if reason == "" {
		return nil, errfmt.Errorf("reason is required for rejection")
	}

	appReq, err := GetApproval(ctx, store, approvalID)
	if err != nil {
		return nil, err
	}

	now := CurrentTimestamp()
	updates := map[string]any{
		objects.FieldKeyStatus: StatusRejected,
		"reviewer":             reviewer,
		"reason":               reason,
		"reviewed_at":          now,
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	if err := store.Update(ctx, secCtx, approvalID, updates); err != nil {
		return nil, errfmt.Newf("failed to update rejection status: %v", err)
	}

	appReq.Status = StatusRejected
	appReq.Reviewer = reviewer
	appReq.Reason = reason
	appReq.ReviewedAt = now
	appReq.UpdatedAt = now

	// Record rejection entry to audit ledger
	recordAuditLedger(ctx, store, projectRoot, secCtx, AuditEventTypeGateRejection, fmt.Sprintf("Gate approval rejected for %s: %s", approvalID, reason), reviewer, map[string]any{
		"approval_id": approvalID,
		"reviewer":    reviewer,
		"reason":      reason,
		"type":        appReq.Type,
		"target":      appReq.Target,
		"reviewed_at": now,
	})

	return appReq, nil
}

// VerifyApproval implements fail-closed cryptographic mutation gate verification.
// Returns (true, nil) ONLY if:
// 1. Storage and inputs are non-empty.
// 2. The approval request exists.
// 3. Status is "approved".
// 4. SHA256 digest of payloadBytes strictly matches the staged digest.
// 5. Stored payload (if present) strictly matches payloadBytes.
func VerifyApproval(
	ctx context.Context,
	store storage.ObjectStorageProvider,
	approvalID string,
	payloadBytes []byte,
) (bool, error) {
	if store == nil {
		return false, errfmt.Errorf("fail-closed gate verification: nil storage provider")
	}
	if approvalID == "" {
		return false, errfmt.Errorf("fail-closed gate verification: empty approval ID")
	}
	if len(payloadBytes) == 0 {
		return false, errfmt.Errorf("fail-closed gate verification: empty payload bytes")
	}

	appReq, err := GetApproval(ctx, store, approvalID)
	if err != nil {
		return false, errfmt.Newf("fail-closed gate verification: failed to load approval %s: %v", approvalID, err)
	}

	if appReq.Status != StatusApproved {
		return false, errfmt.Errorf("fail-closed gate verification: approval %s has status %q (expected %q)", approvalID, appReq.Status, StatusApproved)
	}

	expectedHash := appReq.PayloadHash
	if expectedHash == "" {
		expectedHash = appReq.Hash
	}
	actualHash := ComputePayloadDigest(payloadBytes)
	if expectedHash != actualHash {
		return false, errfmt.Errorf("fail-closed gate verification: cryptographic digest mismatch for %s: expected %s, got %s", approvalID, expectedHash, actualHash)
	}

	if appReq.Payload != "" && appReq.Payload != string(payloadBytes) {
		return false, errfmt.Errorf("fail-closed gate verification: payload content mismatch for %s", approvalID)
	}

	return true, nil
}

// ListApprovals returns approval requests optionally filtered by status.
func ListApprovals(
	ctx context.Context,
	store storage.ObjectStorageProvider,
	statusFilter string,
) ([]*ApprovalRequest, error) {
	if store == nil {
		return nil, errfmt.Errorf("storage provider is required")
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	listFilter := storage.ListFilter{
		Kind:    KindApprovalRequest,
		SortBy:  objects.FieldKeyCreatedAt,
		SortAsc: false,
	}

	res, err := store.List(ctx, secCtx, nil, listFilter)
	if err != nil {
		return nil, errfmt.Newf("failed to list approvals: %v", err)
	}

	var out []*ApprovalRequest
	for _, raw := range res.Objects {
		parsed, err := FromMap(raw)
		if err == nil {
			if statusFilter == "" || parsed.Status == statusFilter {
				out = append(out, parsed)
			}
		}
	}
	return out, nil
}

// recordAuditLedger records an entry to the audit ledger using storage helper or direct store creation.
func recordAuditLedger(
	ctx context.Context,
	store storage.ObjectStorageProvider,
	projectRoot string,
	secCtx *pkgctx.SecurityContext,
	eventType string,
	operation string,
	actor string,
	metadata map[string]any,
) {
	if projectRoot != "" {
		_ = storage.CreateAuditEventWithBuilder(ctx, projectRoot, secCtx, store, &storage.AuditEventOptions{
			EventType:  eventType,
			Operation:  operation,
			Severity:   audit.SeverityHigh,
			TargetKind: KindApprovalRequest,
			CreatedBy:  actor,
			Metadata:   metadata,
		})
		return
	}

	// In-memory or direct fallback
	auditObj := map[string]any{
		objects.FieldKeyKind:      objects.KindAuditEvent,
		objects.FieldKeyTitle:     operation,
		objects.FieldKeyOperation: operation,
		objects.FieldKeyEventType: eventType,
		objects.FieldKeySeverity:  audit.SeverityHigh,
		objects.FieldKeyStatus:    "recorded",
		objects.FieldKeyCreatedBy: actor,
		objects.FieldKeyCreatedAt: CurrentTimestamp(),
		objects.FieldKeyMetadata:  metadata,
	}
	_ = store.Create(ctx, secCtx, auditObj)
}
