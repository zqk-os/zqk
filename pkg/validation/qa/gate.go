package qa

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
)

// Gate defines the interface for the mandatory quality gate.
type Gate interface {
	VerifyComplete(ctx context.Context, itemID string) error
}

// AuditorGate implements the mandatory quality gate.
type AuditorGate struct {
	storage storage.ObjectStorageProvider
}

func NewAuditorGate(s storage.ObjectStorageProvider) *AuditorGate {
	return &AuditorGate{storage: s}
}

// VerifyComplete verifies that a signed QASuccess object exists for the itemID
// AND that the signature is valid according to the trusted Auditor public key.
func (g *AuditorGate) VerifyComplete(ctx context.Context, itemID string) error {
	if g.storage == nil {
		return fmt.Errorf(validation.ConstMagicd1380660)
	}

	// 1. Load the TRUSTED public key from the Keystore
	keyObj, err := g.storage.Read(ctx, nil, AuditorKeyID)
	if err != nil {
		return fmt.Errorf(validation.ConstMagic6fab60b4, err)
	}

	trustedPubHex, ok := keyObj[objects.FieldKeyDescription].(string) // We're using description as the public key storage for now
	if !ok || trustedPubHex == "" || trustedPubHex == validation.ConstMagic57bc28f9 {
		return fmt.Errorf(validation.ConstMagic60ea95db, AuditorKeyID)
	}

	// 2. Query for QASuccess object referencing this itemID
	filter := storage.ListFilter{
		Kind: KindQASuccess,
		Filters: map[string]any{
			"item_id":              itemID,
			objects.FieldKeyStatus: "success",
		},
	}
	res, err := g.storage.List(ctx, nil, nil, filter)
	if err != nil {
		return fmt.Errorf(validation.ConstMagic55cde375, itemID, err)
	}

	if len(res.Objects) == 0 {
		return fmt.Errorf(validation.ConstMagicf244a94f, itemID)
	}

	// 3. Verify Cryptographic Signature against the TRUSTED key
	reportObj := res.Objects[0]
	report := QAReport{
		ItemID:    reportObj["item_id"].(string),
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
