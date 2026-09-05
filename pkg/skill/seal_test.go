package skill

import (
	"strings"
	"testing"
	"time"
)

func TestVerifySeal_Valid(t *testing.T) {
	body := "\n# My Skill\nSome content."
	now := time.Now().Truncate(time.Second)
	sealLines := GenerateSealData(body, "v1", "zqk-kernel", now)

	content := "---\nname: my-skill\n" + sealLines + "\n---" + body

	res := VerifySeal(content)
	if !res.Valid {
		t.Fatalf("expected valid seal, got error: %s", res.Error)
	}
	if res.Version != "v1" {
		t.Errorf("expected v1, got %s", res.Version)
	}
	if res.Issuer != "zqk-kernel" {
		t.Errorf("expected zqk-kernel, got %s", res.Issuer)
	}
	if !res.Date.Equal(now) {
		t.Errorf("expected date %v, got %v", now, res.Date)
	}
}

func TestVerifySeal_Tampered(t *testing.T) {
	body := "\n# My Skill\nSome content."
	now := time.Now()
	sealLines := GenerateSealData(body, "v1", "zqk-kernel", now)

	// tamper body
	tamperedBody := "\n# My Skill\nSome tampered content."
	content := "---\nname: my-skill\n" + sealLines + "\n---" + tamperedBody

	res := VerifySeal(content)
	if res.Valid {
		t.Fatalf("expected invalid seal due to tampering")
	}
	if !strings.Contains(res.Error, "seal hash mismatch") {
		t.Errorf("expected hash mismatch error, got: %s", res.Error)
	}
}

func TestVerifySeal_MissingFrontmatter(t *testing.T) {
	content := "No frontmatter here"
	res := VerifySeal(content)
	if res.Valid {
		t.Fatalf("expected invalid seal")
	}
	if !strings.Contains(res.Error, "missing frontmatter") {
		t.Errorf("expected missing frontmatter error, got: %s", res.Error)
	}
}

func TestVerifySeal_MissingVersionIssuerDate(t *testing.T) {
	body := "\nbody"
	hashOnly := "---\nseal_hash: deadbeef\n---" + body
	res := VerifySeal(hashOnly)
	if res.Valid {
		t.Fatalf("expected invalid when version/issuer/date missing")
	}
	if !strings.Contains(res.Error, "seal_version") {
		t.Errorf("expected seal_version error, got: %s", res.Error)
	}
}
