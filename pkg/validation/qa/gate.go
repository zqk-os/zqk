package qa

import (
	"context"
	"fmt"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// Gate defines the interface for the mandatory quality gate.
type Gate interface {
	VerifyComplete(ctx context.Context, itemID string) error
}

// AuditorGate implements the mandatory quality gate.
type AuditorGate struct {
	storage        storage.ObjectStorageProvider
	projectRoot    string
	strictFallback bool
}

func NewAuditorGate(s storage.ObjectStorageProvider) *AuditorGate {
	return &AuditorGate{storage: s}
}

// NewAuditorGateForProject is VerifyComplete with a file fallback to
// .zqk/keystore/auditor.priv when KEY-AUDITOR-001 is not in CAS.
func NewAuditorGateForProject(s storage.ObjectStorageProvider, projectRoot string) *AuditorGate {
	return &AuditorGate{
		storage:        s,
		projectRoot:    projectRoot,
		strictFallback: zqkenv.ProductionKeystoreStrict().Get() == "1" || zqkenv.ProductionKeystoreStrict().Get() == "true",
	}
}

// WithStrictFallback configures whether unencrypted disk-based private key fallback
// is disallowed when KEY-AUDITOR-001 is absent from CAS.
func (g *AuditorGate) WithStrictFallback(strict bool) *AuditorGate {
	g.strictFallback = strict
	return g
}

const (
	errFmtLatch1MissingArtifacts = "latch 1 failed (data existence): missing required deliverable artifacts for %s"
	errFmtLatch1ArtifactNotFound = "latch 1 failed (data existence): artifact file does not exist or cannot be read: %s"
)

// VerifyComplete verifies that a signed QASuccess object exists for the itemID
// AND that the signature is valid according to the trusted Auditor public key.
func (g *AuditorGate) VerifyComplete(ctx context.Context, itemID string) error {
	if g.storage == nil {
		return fmt.Errorf("QA Auditor Gate storage not initialized")
	}

	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. LATCH 1 (Data Existence): If item exists in storage, verify required artifacts exist on disk.
	if obj, readErr := g.storage.Read(ctx, secCtx, itemID); readErr == nil && obj != nil {
		if _, err := ValidateDeliverableArtifacts(obj, g.projectRoot); err != nil {
			return fmt.Errorf("latch 1 failed (data existence): %w", err)
		}
	}

	trustedPubHex, err := g.trustedPubHex(ctx, secCtx)
	if err != nil {
		return err
	}

	// 2. LATCH 2 (AST / Invariant Clean): Query for QASuccess object referencing this itemID
	filter := QASuccessFilter(itemID)
	res, err := g.storage.List(ctx, secCtx, nil, filter)
	if err != nil {
		return fmt.Errorf("failed to query QA success for %s: %w", itemID, err)
	}

	if len(res.Objects) == 0 {
		return fmt.Errorf("QA Auditor has not issued a verified QASuccess for %s", itemID)
	}

	// 3. Verify Cryptographic Signature against the TRUSTED key
	report := ExtractQAReport(res.Objects[0])

	// SENSITIVE CHECK: Ensure the report's public key matches the TRUSTED public key
	if report.PublicKey != trustedPubHex {
		return fmt.Errorf("CRITICAL SECURITY VULNERABILITY: report signed with UNTRUSTED key for %s", itemID)
	}

	return g.verifySignature(report, trustedPubHex)
}

// QASuccessFilter returns a storage.ListFilter for finding verified QASuccess records for itemID.
func QASuccessFilter(itemID string) storage.ListFilter {
	return storage.ListFilter{
		Kind: KindQASuccess,
		Filters: map[string]any{
			objects.FieldKeyItemID: itemID,
			objects.FieldKeyStatus: objects.ObjectStatusSuccess,
		},
	}
}

// ExtractQAReport extracts a typed QAReport from an untyped object map.
func ExtractQAReport(reportObj map[string]any) QAReport {
	if reportObj == nil {
		return QAReport{}
	}
	itemID, _ := reportObj[objects.FieldKeyItemID].(string)
	status, _ := reportObj[objects.FieldKeyStatus].(string)
	sig, _ := reportObj[objects.FieldKeySignature].(string)
	pubKey, _ := reportObj[objects.FieldKeyPublicKey].(string)
	return QAReport{
		ItemID:    itemID,
		Status:    status,
		Signature: sig,
		PublicKey: pubKey,
	}
}

func (g *AuditorGate) trustedPubHex(ctx context.Context, secCtx *pkgctx.SecurityContext) (string, error) {
	keyObj, err := g.storage.Read(ctx, secCtx, AuditorKeyID)
	if err == nil {
		trustedPubHex, ok := keyObj[objects.FieldKeyDescription].(string)
		if ok && trustedPubHex != "" && trustedPubHex != "Master public key for the QA Auditor" {
			return trustedPubHex, nil
		}
	}
	if g.strictFallback {
		if err != nil {
			return "", fmt.Errorf("%s: %w", ErrMsgDiskFallbackDisallowed, err)
		}
		return "", fmt.Errorf("%s: key %s not found in CAS", ErrMsgDiskFallbackDisallowed, AuditorKeyID)
	}
	if g.projectRoot == "" {
		if err != nil {
			return "", fmt.Errorf("failed to load trusted auditor key: %w", err)
		}
		return "", fmt.Errorf("trusted auditor public key not initialized in %s", AuditorKeyID)
	}
	privPath := AuditorPrivateKeyPath(g.projectRoot)
	signer, signErr := NewAuditorSigner(privPath)
	if signErr != nil {
		if err != nil {
			return "", fmt.Errorf("failed to load trusted auditor key: %w", err)
		}
		return "", fmt.Errorf("trusted auditor public key not initialized in %s", AuditorKeyID)
	}
	return signer.PublicKey(), nil
}

func (g *AuditorGate) verifySignature(report QAReport, pubHex string) error {
	return VerifyQAReportSignature(report, pubHex)
}
