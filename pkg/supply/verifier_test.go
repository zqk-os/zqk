package supply

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerifier_VerifyOpenVEXBytes(t *testing.T) {
	t.Parallel()
	v := NewVerifier()

	// 1. Valid OpenVEX v0.2.0 document
	validVEX := []byte(`{
		"@context": "https://openvex.dev/ns/v0.2.0",
		"@id": "https://github.com/zqk-os/zqk/releases/tag/v0.1.0/openvex.json",
		"author": "ZQK Security Response Team",
		"timestamp": "2026-09-30T00:00:00Z",
		"version": 1,
		"statements": [
			{
				"vulnerability": {"name": "CVE-2026-0001"},
				"status": "not_affected",
				"justification": "component_not_present"
			}
		]
	}`)
	assert.NoError(t, v.VerifyOpenVEXBytes(validVEX))

	// 2. Empty data fails
	assert.Error(t, v.VerifyOpenVEXBytes(nil))

	// 3. Malformed JSON fails
	assert.Error(t, v.VerifyOpenVEXBytes([]byte(`{not-json`)))

	// 4. Invalid @context fails
	invalidContext := []byte(`{
		"@context": "https://invalid.com/ns",
		"@id": "id1",
		"author": "author",
		"statements": [{"status": "fixed"}]
	}`)
	assert.Error(t, v.VerifyOpenVEXBytes(invalidContext))

	// 5. Missing @id fails
	missingID := []byte(`{
		"@context": "https://openvex.dev/ns/v0.2.0",
		"@id": "",
		"author": "author",
		"statements": [{"status": "fixed"}]
	}`)
	assert.Error(t, v.VerifyOpenVEXBytes(missingID))

	// 6. Missing author fails
	missingAuthor := []byte(`{
		"@context": "https://openvex.dev/ns/v0.2.0",
		"@id": "id1",
		"author": "  ",
		"statements": [{"status": "fixed"}]
	}`)
	assert.Error(t, v.VerifyOpenVEXBytes(missingAuthor))

	// 7. Empty statements fails
	emptyStatements := []byte(`{
		"@context": "https://openvex.dev/ns/v0.2.0",
		"@id": "id1",
		"author": "author",
		"statements": []
	}`)
	assert.Error(t, v.VerifyOpenVEXBytes(emptyStatements))

	// 8. Invalid statement status fails
	invalidStatus := []byte(`{
		"@context": "https://openvex.dev/ns/v0.2.0",
		"@id": "id1",
		"author": "author",
		"statements": [{"status": "unknown_status"}]
	}`)
	assert.Error(t, v.VerifyOpenVEXBytes(invalidStatus))
}

func TestVerifier_VerifyChecksums(t *testing.T) {
	t.Parallel()
	v := NewVerifier()

	files := map[string][]byte{
		"zqk-darwin-arm64.tar.gz": []byte("binary content arm64"),
		"zqk-linux-amd64.tar.gz":  []byte("binary content amd64"),
	}

	hash1 := sha256.Sum256(files["zqk-darwin-arm64.tar.gz"])
	hash2 := sha256.Sum256(files["zqk-linux-amd64.tar.gz"])

	manifest := fmt.Sprintf("%s  zqk-darwin-arm64.tar.gz\n%s  zqk-linux-amd64.tar.gz\n",
		hex.EncodeToString(hash1[:]), hex.EncodeToString(hash2[:]))

	reader := func(filename string) ([]byte, error) {
		data, ok := files[filename]
		if !ok {
			return nil, fmt.Errorf("file not found: %s", filename)
		}
		return data, nil
	}

	// 1. Valid manifest passes
	assert.NoError(t, v.VerifyChecksums([]byte(manifest), reader))

	// 2. Mismatched checksum fails
	badManifest := fmt.Sprintf("%s  zqk-darwin-arm64.tar.gz\n", hex.EncodeToString(make([]byte, 32)))
	assert.Error(t, v.VerifyChecksums([]byte(badManifest), reader))

	// 3. Missing file fails
	missingManifest := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  missing-file.tar.gz\n"
	assert.Error(t, v.VerifyChecksums([]byte(missingManifest), reader))

	// 4. Empty manifest fails
	assert.Error(t, v.VerifyChecksums([]byte(""), reader))
}

func TestVerifier_DetectSecretPatterns(t *testing.T) {
	t.Parallel()
	v := NewVerifier()

	// Clean content
	clean := "func main() { fmt.Println(\"Hello world\") }"
	assert.Empty(t, v.DetectSecretPatterns(clean))

	// Injected dummy secrets (safely split to avoid git grep triggers)
	dummyPAT := "git" + "hub_pat_11AAAAAAA0123456789012345678901234567890123456789012345678901234567890123456789012"
	matches := v.DetectSecretPatterns("token = " + dummyPAT)
	assert.Len(t, matches, 1)
	assert.Equal(t, dummyPAT, matches[0])
}
