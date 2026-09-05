package qa

import (
	"path/filepath"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/validation"
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
}
