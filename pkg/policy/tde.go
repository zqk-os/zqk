package policy

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/lanceman/zqk/pkg/infrastructure/crypto"
)

// NeurologicalGovernance defines the cognitive state requirements for an execution.
type NeurologicalGovernance struct {
	Confidence     float64 `json:"confidence,omitempty"`
	IsStreaming    bool    `json:"is_streaming,omitempty"`
	DecisionBranch string  `json:"decision_branch,omitempty"`
}

// TrustDomainEnvelope represents a cryptographically signed boundary for execution.
type TrustDomainEnvelope struct {
	ID                string                  `json:"id"`
	Scope             string                  `json:"scope"`
	AllowedNamespaces []string                `json:"allowed_namespaces"`
	MaxRuntimeSeconds int                     `json:"max_runtime_seconds"`
	Payload           any                     `json:"payload"`
	MCPPermissions    []string                `json:"mcp_permissions"`
	Neurological      *NeurologicalGovernance `json:"neurological,omitempty"`
	Signature         string                  `json:"signature"`
	PublicKey         string                  `json:"public_key"`
}

// Hash returns the SHA-256 hash of the TDE (excluding the signature).
func (tde *TrustDomainEnvelope) Hash() ([]byte, error) {
	copyTDE := *tde
	copyTDE.Signature = "" // Strip signature before hashing
	copyTDE.PublicKey = "" // Strip public key before hashing

	data, err := json.Marshal(copyTDE)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(data)
	return hash[:], nil
}

// Sign uses the provided crypto.Signer to sign the TDE.
func (tde *TrustDomainEnvelope) Sign(signer crypto.Signer) error {
	hash, err := tde.Hash()
	if err != nil {
		return err
	}
	sig, err := signer.Sign(hash)
	if err != nil {
		return err
	}
	tde.Signature = sig
	tde.PublicKey = signer.PublicKey()
	return nil
}

// Verify validates the TDE's signature and core schema requirements.
func (tde *TrustDomainEnvelope) Verify() error {
	if tde.Signature == "" || tde.PublicKey == "" {
		return fmt.Errorf("TDE lacks signature or public key")
	}

	hash, err := tde.Hash()
	if err != nil {
		return err
	}

	valid, err := crypto.GlobalVerifier.Verify(hash, tde.Signature, tde.PublicKey)
	if err != nil {
		return fmt.Errorf("verification error: %v", err)
	}
	if !valid {
		return fmt.Errorf("invalid TDE signature")
	}

	if tde.Scope == "" {
		return fmt.Errorf("TDE lacks scope")
	}
	if len(tde.AllowedNamespaces) == 0 {
		return fmt.Errorf("TDE must specify at least one allowed namespace")
	}

	// Strict MCP Permissions checks
	if len(tde.MCPPermissions) == 0 {
		return fmt.Errorf("ABORT: TDE must strictly enforce MCP permissions (none provided)")
	}

	// Strict Neurological Governance Checks
	if tde.Neurological == nil {
		return fmt.Errorf("ABORT: Neurological governance payload missing in TDE transport")
	}
	if tde.Neurological.Confidence < 0.75 {
		return fmt.Errorf("ABORT: Neurological confidence too low for TDE transport")
	}
	if tde.Neurological.DecisionBranch == "" {
		return fmt.Errorf("ABORT: Neurological DecisionBranch required for TDE transport")
	}

	return nil
}
