package qa

import (
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestAuditorSigner_Sign(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "auditor.priv")
	signer, err := NewAuditorSigner(keyPath)
	if err != nil {
		t.Fatalf(validation.ConstMagic2d053c17, err)
	}

	// Verify file was created
	if _, err := fileutil.Stat(keyPath); err != nil {
		t.Errorf(validation.ConstMagic2c41d347, err)
	}

	data := []byte(validation.ConstMagicc9dd2357)
	sig, err := signer.Sign(data)
	if err != nil {
		t.Fatalf(validation.ConstMagic9ecf0de4, err)
	}

	if sig == "" {
		t.Error(validation.ConstMagiced297c89)
	}

	// Reload signer from existing key
	signer2, err := NewAuditorSigner(keyPath)
	if err != nil {
		t.Fatalf(validation.ConstMagic6b2dda5f, err)
	}
	if signer2.PublicKey() != signer.PublicKey() {
		t.Error(validation.ConstMagic49fd2507)
	}

	t.Logf("Signature: %s", sig)
	t.Logf("Public Key: %s", signer.PublicKey())

	// Positive verification
	if err := VerifySignature(signer.PublicKey(), data, sig); err != nil {
		t.Fatalf("expected signature verification to pass, got: %v", err)
	}

	// Negative verification with tampered data
	tamperedData := []byte("tampered_content")
	if err := VerifySignature(signer.PublicKey(), tamperedData, sig); err == nil {
		t.Fatal("expected signature verification to fail on tampered data")
	}

	// Test VerifyQAReportSignature
	report := QAReport{
		ItemID:    "BLI-TEST-123",
		Status:    "success",
		Signature: "",
		PublicKey: signer.PublicKey(),
	}
	reportData := []byte(report.ItemID + report.Status)
	reportSig, err := signer.Sign(reportData)
	if err != nil {
		t.Fatalf("failed to sign report data: %v", err)
	}
	report.Signature = reportSig

	if err := VerifyQAReportSignature(report, signer.PublicKey()); err != nil {
		t.Fatalf("expected report signature verification to pass, got: %v", err)
	}

	report.Status = "failed"
	if err := VerifyQAReportSignature(report, signer.PublicKey()); err == nil {
		t.Fatal("expected report signature verification to fail on tampered status")
	}
}
