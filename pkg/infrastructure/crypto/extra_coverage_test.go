// BLI-STARTER-COMMUNITY-030 / PRI-STARTER-COMMUNITY-030 coverage elevation
package crypto

import (
	"encoding/hex"
	"testing"
)

func TestNewEd25519ManagerFromSeed_Errors(t *testing.T) {
	t.Parallel()
	if _, err := NewEd25519ManagerFromSeed("zz"); err == nil {
		t.Fatal("bad hex")
	}
	if _, err := NewEd25519ManagerFromSeed("abcd"); err == nil {
		t.Fatal("short seed")
	}
}

func TestEd25519Manager_VerifyErrors(t *testing.T) {
	t.Parallel()
	m, err := GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("x")
	sig, err := m.Sign(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Verify(data, "not-hex", m.PublicKey()); err == nil {
		t.Fatal("bad sig hex")
	}
	if _, err := m.Verify(data, sig, "not-hex"); err == nil {
		t.Fatal("bad pub hex")
	}
	if _, err := m.Verify(data, sig, hex.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("short pub")
	}
}
