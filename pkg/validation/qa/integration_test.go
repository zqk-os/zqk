package qa

import (
	"fmt"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/validation"
)

func TestAuditorGate_FullLoop(t *testing.T) {
	// 1. Setup Trusted Signer
	signer, _ := NewAuditorSigner("")

	itemID := fmt.Sprintf("BLI-H-001-%d", time.Now().UnixNano())
	status := "success"

	// 2. Setup Mock Storage with Trusted Key in Keystore
	reportData := []byte(itemID + status)
	sig, _ := signer.Sign(reportData)

	trustedKeyObj := map[string]any{
		objects.FieldKeyID:          AuditorKeyID,
		objects.FieldKeyKind:        "keystore_entry",
		objects.FieldKeyDescription: signer.PublicKey(), // Trusted public key
		objects.FieldKeyAccountID:   "ACC-SYSTEM",
	}

	reportObj := map[string]any{
		objects.FieldKeyItemID:    itemID,
		objects.FieldKeyTitle:     QASuccessTitle(itemID),
		objects.FieldKeyStatus:    status,
		objects.FieldKeySignature: sig,
		objects.FieldKeyPublicKey: signer.PublicKey(),
		objects.FieldKeyAccountID: "ACC-SYSTEM",
	}

	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	maliciousSigner, _ := NewAuditorSigner("")
	maliciousSig, _ := maliciousSigner.Sign(reportData)

	reportObj[objects.FieldKeyKind] = "qa_success"
	reportObj[objects.FieldKeyID] = fmt.Sprintf("QAS-%d", time.Now().UnixNano())
	spoofedID := fmt.Sprintf("BLI-SPOOFED-%d", time.Now().UnixNano())
	maliciousReportObj := map[string]any{
		objects.FieldKeyID:        fmt.Sprintf("QAS-MAL-%d", time.Now().UnixNano()),
		objects.FieldKeyItemID:    spoofedID,
		objects.FieldKeyTitle:     QASuccessTitle(spoofedID),
		objects.FieldKeyKind:      "qa_success",
		objects.FieldKeyStatus:    status,
		objects.FieldKeySignature: maliciousSig,
		objects.FieldKeyPublicKey: maliciousSigner.PublicKey(),
		objects.FieldKeyAccountID: "ACC-SYSTEM",
	}

	existingKeyObj, err := realStorage.Read(ctx, secCtx, AuditorKeyID)
	if err == nil {
		trustedKeyObj[objects.FieldKeyUpdatedAt] = existingKeyObj[objects.FieldKeyUpdatedAt]
		if err := realStorage.Update(ctx, secCtx, AuditorKeyID, trustedKeyObj); err != nil {
			t.Fatalf("Failed to update trusted key: %v", err)
		}
	} else {
		if err := realStorage.Create(ctx, secCtx, trustedKeyObj); err != nil {
			t.Fatalf("Failed to create trusted key: %v", err)
		}
	}
	if err := realStorage.Create(ctx, secCtx, reportObj); err != nil {
		t.Fatalf("failed to create report: %v", err)
	}
	if err := realStorage.Create(ctx, secCtx, maliciousReportObj); err != nil {
		t.Fatalf("failed to create mal report: %v", err)
	}

	gate := NewAuditorGate(realStorage)

	// 3. Verify via Gate (PASS)
	err = gate.VerifyComplete(ctx, itemID)
	if err != nil {
		t.Fatalf(validation.ConstMagic3c4e5f09, err)
	}

	// 4. Test SPOOFING attempt (FAIL)
	err = gate.VerifyComplete(ctx, spoofedID)
	if err == nil {
		t.Errorf(validation.ConstMagic7b9fc533)
	} else {
		t.Logf(validation.ConstMagicd4e0732d, err)
	}
}

// TRACK: BLI-CEF-QA-SUCCESS-SPEC-001 / CRIT-CEF-QA-SUCCESS-SPEC-001
func TestQASuccessSpec_BLI_CEF_QA_SUCCESS_SPEC_001(t *testing.T) {
	t.Parallel()
	itemID := "BLI-TEST-QA-001"
	title := QASuccessTitle(itemID)
	if title == "" {
		t.Fatal("expected non-empty QASuccessTitle")
	}
	signer, err := NewAuditorSigner("")
	if err != nil {
		t.Fatalf("NewAuditorSigner: %v", err)
	}
	if signer.PublicKey() == "" {
		t.Fatal("expected non-empty signer public key")
	}
}

