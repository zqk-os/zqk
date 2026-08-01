package qa

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/validation"
)

func TestAuditorGate_FullLoop(t *testing.T) {
	// 1. Setup Trusted Signer
	signer, _ := NewAuditorSigner("")

	itemID := "ITEM-H-001"
	status := "success"

	// 2. Setup Mock Storage with Trusted Key in Keystore
	reportData := []byte(itemID + status)
	sig, _ := signer.Sign(reportData)

	trustedKeyObj := map[string]any{
		objects.FieldKeyID:          AuditorKeyID,
		objects.FieldKeyKind:        "keystore_entry",
		objects.FieldKeyDescription: signer.PublicKey(), // Trusted public key
	}

	reportObj := map[string]any{
		"item_id":                 itemID,
		objects.FieldKeyStatus:    status,
		objects.FieldKeySignature: sig,
		objects.FieldKeyPublicKey: signer.PublicKey(),
	}

	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	maliciousSigner, _ := NewAuditorSigner("")
	maliciousSig, _ := maliciousSigner.Sign(reportData)

	reportObj[objects.FieldKeyKind] = "qa_report"
	maliciousReportObj := map[string]any{
		"item_id":                 "ITEM-SPOOFED",
		objects.FieldKeyKind:      "qa_report",
		objects.FieldKeyStatus:    status,
		objects.FieldKeySignature: maliciousSig,
		objects.FieldKeyPublicKey: maliciousSigner.PublicKey(),
	}

	_ = realStorage.Create(ctx, secCtx, trustedKeyObj)
	_ = realStorage.Create(ctx, secCtx, reportObj)
	_ = realStorage.Create(ctx, secCtx, maliciousReportObj)

	gate := NewAuditorGate(realStorage)

	// 3. Verify via Gate (PASS)
	err := gate.VerifyComplete(ctx, itemID)
	if err != nil {
		t.Fatalf(validation.ConstMagic3c4e5f09, err)
	}

	// 4. Test SPOOFING attempt (FAIL)
	err = gate.VerifyComplete(ctx, "ITEM-SPOOFED")
	if err == nil {
		t.Errorf(validation.ConstMagic7b9fc533)
	} else {
		t.Logf(validation.ConstMagicd4e0732d, err)
	}
}
