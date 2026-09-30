package qa

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"path/filepath"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
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
		return fmt.Errorf(validation.ConstMagicd1380660)
	}

	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. LATCH 1 (Data Existence): If item exists in storage, verify required artifacts exist on disk.
	if obj, readErr := g.storage.Read(ctx, secCtx, itemID); readErr == nil && obj != nil {
		paths := extractArtifactPaths(obj[objects.FieldKeyArtifacts])
		if len(paths) == 0 {
			paths = extractArtifactPaths(obj[objects.FieldKeyCodeLocation])
		}
		if len(paths) == 0 && (obj[objects.FieldKeyKind] == objects.KindBacklogItem || obj[objects.FieldKeyKind] == objects.KindAgentTask) {
			return fmt.Errorf(errFmtLatch1MissingArtifacts, itemID)
		}
		for _, p := range paths {
			targetPath := p
			if !filepath.IsAbs(targetPath) && g.projectRoot != "" {
				targetPath = filepath.Join(g.projectRoot, targetPath)
			}
			info, statErr := fileutil.Stat(targetPath)
			if statErr != nil || info.IsDir() {
				return fmt.Errorf(errFmtLatch1ArtifactNotFound, p)
			}
		}
	}

	trustedPubHex, err := g.trustedPubHex(ctx, secCtx)
	if err != nil {
		return err
	}

	// 2. LATCH 2 (AST / Invariant Clean): Query for QASuccess object referencing this itemID
	filter := storage.ListFilter{
		Kind: KindQASuccess,
		Filters: map[string]any{
			objects.FieldKeyItemID: itemID,
			objects.FieldKeyStatus: objects.ObjectStatusSuccess,
		},
	}
	res, err := g.storage.List(ctx, secCtx, nil, filter)
	if err != nil {
		return fmt.Errorf(validation.ConstMagic55cde375, itemID, err)
	}

	if len(res.Objects) == 0 {
		return fmt.Errorf(validation.ConstMagicf244a94f, itemID)
	}

	// 3. Verify Cryptographic Signature against the TRUSTED key
	reportObj := res.Objects[0]
	report := QAReport{
		ItemID:    reportObj[objects.FieldKeyItemID].(string),
		Status:    reportObj[objects.FieldKeyStatus].(string),
		Signature: reportObj[objects.FieldKeySignature].(string),
		PublicKey: reportObj[objects.FieldKeyPublicKey].(string),
	}

	// SENSITIVE CHECK: Ensure the report's public key matches the TRUSTED public key
	if report.PublicKey != trustedPubHex {
		return fmt.Errorf(validation.ConstMagic50f3bb1d, itemID)
	}

	return g.verifySignature(report, trustedPubHex)
}

func (g *AuditorGate) trustedPubHex(ctx context.Context, secCtx *pkgctx.SecurityContext) (string, error) {
	keyObj, err := g.storage.Read(ctx, secCtx, AuditorKeyID)
	if err == nil {
		trustedPubHex, ok := keyObj[objects.FieldKeyDescription].(string)
		if ok && trustedPubHex != "" && trustedPubHex != validation.ConstMagic57bc28f9 {
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
			return "", fmt.Errorf(validation.ConstMagic6fab60b4, err)
		}
		return "", fmt.Errorf(validation.ConstMagic60ea95db, AuditorKeyID)
	}
	privPath := filepath.Join(g.projectRoot, paths.ProjectDataDir, "keystore", "auditor.priv")
	signer, signErr := NewAuditorSigner(privPath)
	if signErr != nil {
		if err != nil {
			return "", fmt.Errorf(validation.ConstMagic6fab60b4, err)
		}
		return "", fmt.Errorf(validation.ConstMagic60ea95db, AuditorKeyID)
	}
	return signer.PublicKey(), nil
}

func (g *AuditorGate) verifySignature(report QAReport, pubHex string) error {
	sig, err := hex.DecodeString(report.Signature)
	if err != nil {
		return fmt.Errorf(validation.ConstMagic2d0ec3c6, err)
	}

	if len(pubHex) < 64 {
		return fmt.Errorf(validation.ConstMagic37c27833)
	}

	x := new(big.Int)
	y := new(big.Int)
	x.SetString(pubHex[:len(pubHex)/2], 16)
	y.SetString(pubHex[len(pubHex)/2:], 16)

	pub := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}

	data := []byte(report.ItemID + report.Status)
	hash := sha256.Sum256(data)

	if !ecdsa.VerifyASN1(pub, hash[:], sig) {
		return fmt.Errorf(validation.ConstMagic9f963b73, report.ItemID)
	}

	return nil
}
