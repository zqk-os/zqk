package supply

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/security/secretpatterns"
)

// Verifier provides supply-chain verification and release integrity checks.
type Verifier struct{}

// NewVerifier creates a new supply chain verifier.
func NewVerifier() *Verifier {
	return &Verifier{}
}

// OpenVEXStatement represents a single statement in an OpenVEX document.
type OpenVEXStatement struct {
	Vulnerability struct {
		Name string `json:"name"`
	} `json:"vulnerability"`
	Status          string `json:"status"`
	Justification   string `json:"justification,omitempty"`
	ImpactStatement string `json:"impact_statement,omitempty"`
	ActionStatement string `json:"action_statement,omitempty"`
}

// OpenVEXDocument represents an OpenVEX v0.2.0 document.
type OpenVEXDocument struct {
	Context    string             `json:"@context"`
	ID         string             `json:"@id"`
	Author     string             `json:"author"`
	Timestamp  string             `json:"timestamp"`
	Version    int                `json:"version"`
	Statements []OpenVEXStatement `json:"statements"`
}

// VerifyOpenVEXBytes verifies that data contains a valid OpenVEX v0.2.0 document.
func (v *Verifier) VerifyOpenVEXBytes(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("openvex document is empty")
	}
	var doc OpenVEXDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("failed to parse OpenVEX JSON: %w", err)
	}
	if !strings.HasPrefix(doc.Context, "https://openvex.dev/ns") {
		return fmt.Errorf("invalid @context: %s (must start with https://openvex.dev/ns)", doc.Context)
	}
	if strings.TrimSpace(doc.ID) == "" {
		return fmt.Errorf("missing @id in OpenVEX document")
	}
	if strings.TrimSpace(doc.Author) == "" {
		return fmt.Errorf("missing author in OpenVEX document")
	}
	if len(doc.Statements) == 0 {
		return fmt.Errorf("OpenVEX document must contain at least one statement")
	}
	validStatuses := map[string]bool{
		"not_affected":        true,
		"affected":            true,
		"fixed":               true,
		"under_investigation": true,
	}
	for i, stmt := range doc.Statements {
		if !validStatuses[stmt.Status] {
			return fmt.Errorf("statement[%d] has invalid status: %s", i, stmt.Status)
		}
	}
	return nil
}

// VerifyChecksums validates file hashes against a sha256 checksums manifest (format: "<hash>  <filename>").
func (v *Verifier) VerifyChecksums(manifestData []byte, readFile func(filename string) ([]byte, error)) error {
	lines := strings.Split(string(manifestData), "\n")
	verified := 0
	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return fmt.Errorf("line %d: invalid checksum line format: %q", lineNum+1, line)
		}
		expectedHash := strings.ToLower(fields[0])
		filename := fields[1]

		content, err := readFile(filename)
		if err != nil {
			return fmt.Errorf("file %q referenced in checksums manifest could not be read: %w", filename, err)
		}
		actualSum := sha256.Sum256(content)
		actualHash := hex.EncodeToString(actualSum[:])
		if actualHash != expectedHash {
			return fmt.Errorf("file %q checksum mismatch: expected %s, got %s", filename, expectedHash, actualHash)
		}
		verified++
	}
	if verified == 0 {
		return fmt.Errorf("no checksum entries found in manifest")
	}
	return nil
}

// DetectSecretPatterns scans content for known credential patterns using the canonical security definitions.
func (v *Verifier) DetectSecretPatterns(content string) []string {
	return secretpatterns.Detect(content)
}
