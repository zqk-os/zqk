package qa

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

// Signer is the interface for signing QA reports.
type Signer interface {
	Sign(data []byte) (string, error)
	PublicKey() string
}

// AuditorSigner implements the Signer interface using ECDSA.
type AuditorSigner struct {
	privateKey *ecdsa.PrivateKey
}

const (
	AuditorKeyID     = "KEY-AUDITOR-001"
	AuditorAccountID = objects.DefaultSystemAccountID + "-auditor"
)

// NewAuditorSigner creates a new signer. It attempts to load the private key from the provided path,
// or generates a new one if missing.
func NewAuditorSigner(keyPath string) (*AuditorSigner, error) {
	if keyPath != "" {
		if data, err := fileutil.ReadFile(keyPath); err == nil {
			block, rest := pem.Decode(data)
			if len(rest) > 0 {
				logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(validation.ConstMagic6d161184, keyPath), nil).Log()
			}
			if block != nil && block.Type == validation.ConstMagicExtracted_68 {
				priv, err := x509.ParseECPrivateKey(block.Bytes)
				if err == nil {
					return &AuditorSigner{privateKey: priv}, nil
				}
			}
		}
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	// Save new key if path provided
	if keyPath != "" {
		if err := fileutil.MkdirAll(filepath.Dir(keyPath), paths.DirPerm700); err != nil {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(validation.ConstMagic708b5a99, err), nil).Log()
		}
		der, err := x509.MarshalECPrivateKey(priv)
		if err != nil {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(validation.ConstMagice0d28224, err), nil).Log()
		} else {
			block := &pem.Block{Type: validation.ConstMagicExtracted_68, Bytes: der}
			if err := fileutil.WriteSecureFile(keyPath, pem.EncodeToMemory(block)); err != nil {
				logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(validation.ConstMagicc49c97b7, err), nil).Log()
			}
		}
	}

	return &AuditorSigner{privateKey: priv}, nil
}

// Sign signs the provided data and returns a hex-encoded signature.
func (s *AuditorSigner) Sign(data []byte) (string, error) {
	hash := sha256.Sum256(data)
	sig, err := ecdsa.SignASN1(rand.Reader, s.privateKey, hash[:])
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sig), nil
}

// PublicKey returns the hex-encoded public key.
func (s *AuditorSigner) PublicKey() string {
	pub := s.privateKey.PublicKey
	return fmt.Sprintf("%064x%064x", pub.X, pub.Y)
}

// PrivateKey returns the underlying ECDSA private key.
func (s *AuditorSigner) PrivateKey() *ecdsa.PrivateKey {
	return s.privateKey
}

// GetAuditorPublicKeyHex retrieves the public key hex for verification without generating a new private key.
func GetAuditorPublicKeyHex(keyPath string) (string, error) {
	if keyPath == "" {
		return "", fmt.Errorf("auditor key path is required")
	}
	data, err := fileutil.ReadFile(keyPath)
	if err != nil {
		return "", fmt.Errorf("read auditor key file: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM block from auditor key")
	}
	if block.Type == "PUBLIC KEY" {
		pubInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return "", fmt.Errorf("parse PKIX public key: %w", err)
		}
		if ecPub, ok := pubInterface.(*ecdsa.PublicKey); ok {
			return fmt.Sprintf("%064x%064x", ecPub.X, ecPub.Y), nil
		}
		return "", fmt.Errorf("auditor public key is not ECDSA")
	}
	if block.Type == validation.ConstMagicExtracted_68 {
		priv, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return "", fmt.Errorf("parse EC private key: %w", err)
		}
		pub := priv.PublicKey
		return fmt.Sprintf("%064x%064x", pub.X, pub.Y), nil
	}
	return "", fmt.Errorf("unsupported PEM block type: %s", block.Type)
}


// QASuccess object kind.
const KindQASuccess = "qa_success"

// QAReport represents the signed audit finding.
type QAReport struct {
	ItemID    string `json:"item_id" yaml:"item_id"`
	Status    string `json:"status" yaml:"status"`
	Signature string `json:"signature" yaml:"signature"`
	PublicKey string `json:"public_key" yaml:"public_key"`
}

// VerifySignature verifies that the given hex-encoded ASN.1 signature is valid
// for the given data bytes against a hex-encoded P-256 public key.
func VerifySignature(pubHex string, data []byte, sigHex string) error {
	sig, err := hex.DecodeString(sigHex)
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

	hash := sha256.Sum256(data)
	if !ecdsa.VerifyASN1(pub, hash[:], sig) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// VerifyQAReportSignature verifies that a QAReport's signature matches its ItemID and Status
// signed with the provided trusted public key hex.
func VerifyQAReportSignature(report QAReport, trustedPubHex string) error {
	data := []byte(report.ItemID + report.Status)
	if err := VerifySignature(trustedPubHex, data, report.Signature); err != nil {
		return fmt.Errorf(validation.ConstMagic9f963b73, report.ItemID)
	}
	return nil
}
