package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// VerificationResult contains the result of checking a skill file's seal.
type VerificationResult struct {
	Valid   bool
	Error   string
	Version string
	Date    time.Time
	Issuer  string
}

// VerifySeal takes the raw content of a skill file (e.g., SKILL.md)
// and verifies its cryptographic seal. It checks if the hash matches the
// file body and parses the version and date.
func VerifySeal(content string) VerificationResult {
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return VerificationResult{Valid: false, Error: "invalid skill file format: missing frontmatter"}
	}

	frontmatter := parts[1]
	body := parts[2]

	var hashStr, versionStr, issuerStr, dateStr string
	lines := strings.Split(frontmatter, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "seal_hash:") {
			hashStr = strings.TrimSpace(strings.TrimPrefix(line, "seal_hash:"))
		} else if strings.HasPrefix(line, "seal_version:") {
			versionStr = strings.TrimSpace(strings.TrimPrefix(line, "seal_version:"))
		} else if strings.HasPrefix(line, "seal_issuer:") {
			issuerStr = strings.TrimSpace(strings.TrimPrefix(line, "seal_issuer:"))
		} else if strings.HasPrefix(line, "seal_date:") {
			dateStr = strings.TrimSpace(strings.TrimPrefix(line, "seal_date:"))
		}
	}

	if hashStr == "" {
		return VerificationResult{Valid: false, Error: "seal_hash not found in frontmatter"}
	}

	// Strip possible quotes
	hashStr = strings.Trim(hashStr, `"'`)
	versionStr = strings.Trim(versionStr, `"'`)
	issuerStr = strings.Trim(issuerStr, `"'`)
	dateStr = strings.Trim(dateStr, `"'`)

	if versionStr == "" {
		return VerificationResult{Valid: false, Error: "seal_version not found in frontmatter"}
	}
	if issuerStr == "" {
		return VerificationResult{Valid: false, Error: "seal_issuer not found in frontmatter"}
	}
	if dateStr == "" {
		return VerificationResult{Valid: false, Error: "seal_date not found in frontmatter"}
	}

	parsedDate, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		return VerificationResult{Valid: false, Error: fmt.Sprintf("seal_date invalid RFC3339: %v", err)}
	}

	// Compute actual hash of the body part (everything after the second ---)
	actualHash := sha256.Sum256([]byte(body))
	actualHashStr := hex.EncodeToString(actualHash[:])

	if hashStr != actualHashStr {
		return VerificationResult{
			Valid: false,
			Error: fmt.Sprintf("seal hash mismatch: expected %s, got %s", hashStr, actualHashStr),
		}
	}

	return VerificationResult{
		Valid:   true,
		Version: versionStr,
		Issuer:  issuerStr,
		Date:    parsedDate,
	}
}

// GenerateSealData returns the key-value strings to be injected into frontmatter
func GenerateSealData(body string, version string, issuer string, date time.Time) string {
	actualHash := sha256.Sum256([]byte(body))
	actualHashStr := hex.EncodeToString(actualHash[:])

	return fmt.Sprintf("seal_hash: %s\nseal_version: %s\nseal_issuer: %s\nseal_date: %s",
		actualHashStr, version, issuer, date.Format(time.RFC3339))
}
