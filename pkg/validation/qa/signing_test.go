package qa

import (
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAuditorSigner_Sign(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "auditor.priv")
	signer, err := NewAuditorSigner(keyPath)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}

	// Verify file was created
	if _, err := fileutil.Stat(keyPath); err != nil {
		t.Errorf("private key file not created: %v", err)
	}

	data := []byte("audit-report-data")
	sig, err := signer.Sign(data)
	if err != nil {
		t.Fatalf("failed to sign data: %v", err)
	}

	if sig == "" {
		t.Error("expected non-empty signature")
	}

	// Reload signer from existing key
	signer2, err := NewAuditorSigner(keyPath)
	if err != nil {
		t.Fatalf("failed to reload signer: %v", err)
	}
	if signer2.PublicKey() != signer.PublicKey() {
		t.Error("reloaded signer has different public key")
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
