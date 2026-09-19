package tray

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

// IsPrivilegedArgv detects if argv contains restricted flags or destructive command verbs.
// Returns (true, matchedToken) or (false, "").
func IsPrivilegedArgv(argv []string) (bool, string) {
	restrictedFlags := []string{
		"--override",
		"--force",
		"--clear-cache",
		"--internal",
	}

	twoWordVerbs := []struct {
		w1, w2 string
		token  string
	}{
		{"object", "delete", "object delete"},
		{"system", "shutdown", "system shutdown"},
		{"system", "purge", "system purge"},
		{"io", "reap", "io reap"},
	}

	for i, arg := range argv {
		for _, flag := range restrictedFlags {
			if arg == flag || strings.HasPrefix(arg, flag+"=") {
				return true, flag
			}
		}

		if i+1 < len(argv) {
			for _, v := range twoWordVerbs {
				if arg == v.w1 && argv[i+1] == v.w2 {
					return true, v.token
				}
			}
		}

		for _, v := range twoWordVerbs {
			if arg == v.token {
				return true, v.token
			}
		}

		if arg == "delete" {
			return true, "delete"
		}
	}

	return false, ""
}

type canonicalPayload struct {
	Argv []string `json:"argv"`
	Name string   `json:"name"`
}

// CanonicalPayload deterministically marshals struct { Argv []string; Name string } to JSON.
func CanonicalPayload(name string, argv []string) ([]byte, error) {
	cleanArgv := argv
	if cleanArgv == nil {
		cleanArgv = []string{}
	}
	return json.Marshal(canonicalPayload{
		Argv: cleanArgv,
		Name: name,
	})
}

// ComputeArgvDigest computes the SHA-256 hash of CanonicalPayload.
func ComputeArgvDigest(name string, argv []string) ([32]byte, error) {
	payload, err := CanonicalPayload(name, argv)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(payload), nil
}

// SignEntry computes digest, signs with ecdsa.SignASN1, hex encodes result into e.Signature, and sets e.SignedBy.
func SignEntry(e *Entry, privKey *ecdsa.PrivateKey, accountID string) error {
	if e == nil {
		return fmt.Errorf("entry is nil")
	}
	if privKey == nil {
		return fmt.Errorf("private key is nil")
	}
	hash, err := ComputeArgvDigest(e.Name, e.Argv)
	if err != nil {
		return fmt.Errorf("compute argv digest: %w", err)
	}
	sig, err := ecdsa.SignASN1(rand.Reader, privKey, hash[:])
	if err != nil {
		return fmt.Errorf("sign digest: %w", err)
	}
	e.Signature = hex.EncodeToString(sig)
	e.SignedBy = accountID
	return nil
}

// ParsePublicKeyHex parses a public key from hex format (X and Y coordinates, elliptic.P256).
func ParsePublicKeyHex(pubHex string) (*ecdsa.PublicKey, error) {
	pubHex = strings.TrimSpace(pubHex)
	if strings.HasPrefix(pubHex, "04") && len(pubHex) == 130 {
		pubHex = pubHex[2:]
	}
	if len(pubHex) < 64 {
		return nil, fmt.Errorf("public key hex too short: length %d", len(pubHex))
	}
	half := len(pubHex) / 2
	x := new(big.Int)
	y := new(big.Int)
	if _, ok := x.SetString(pubHex[:half], 16); !ok {
		return nil, fmt.Errorf("invalid X coordinate in public key hex")
	}
	if _, ok := y.SetString(pubHex[half:], 16); !ok {
		return nil, fmt.Errorf("invalid Y coordinate in public key hex")
	}
	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}, nil
}

// VerifyEntry verifies the cryptographic signature of an entry against a public key hex.
// If e.Signature == "", returns error "entry is not signed".
// If signature does not match, returns error "cryptographic signature verification failed: signature does not match entry contents".
func VerifyEntry(e *Entry, pubHex string) error {
	if e == nil {
		return fmt.Errorf("entry is nil")
	}
	if e.Signature == "" {
		return fmt.Errorf("entry is not signed")
	}
	sig, err := hex.DecodeString(e.Signature)
	if err != nil {
		return fmt.Errorf("decode signature hex: %w", err)
	}
	pubKey, err := ParsePublicKeyHex(pubHex)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}
	hash, err := ComputeArgvDigest(e.Name, e.Argv)
	if err != nil {
		return fmt.Errorf("compute argv digest: %w", err)
	}
	if !ecdsa.VerifyASN1(pubKey, hash[:], sig) {
		return fmt.Errorf("cryptographic signature verification failed: signature does not match entry contents")
	}
	return nil
}
