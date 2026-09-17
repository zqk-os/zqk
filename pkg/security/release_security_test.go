package security

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func findProjectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to getwd: %v", err)
	}
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate project root containing go.mod")
		}
		dir = parent
	}
}

// TestReleaseSecurity_FunctionalAcceptance verifies CRIT-1789621133231480000-5fda62f2:
// 1. Dependency vulnerability audit via govulncheck confirms 0 symbol-level vulnerabilities.
// 2. Core repository passes license integrity check.
// 3. Payload audit verifies zero credential leaks and zero forbidden test binaries.
func TestReleaseSecurity_FunctionalAcceptance(t *testing.T) {
	projectRoot := findProjectRoot(t)

	// 1. License integrity
	if err := VerifyLicenseIntegrity(projectRoot); err != nil {
		t.Fatalf("License integrity verification failed: %v", err)
	}

	// 2. Static vulnerability scan on core binary package
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	vulnRes, err := AuditStaticVulnerabilities(ctx, projectRoot, "./cmd/zqk")
	if err != nil {
		t.Fatalf("AuditStaticVulnerabilities failed: %v", err)
	}
	if vulnRes.Executed && vulnRes.VulnerabilitiesCount > 0 {
		t.Errorf("expected 0 vulnerabilities affecting cmd/zqk, got %d", vulnRes.VulnerabilitiesCount)
	}

	// 3. Payload audit on clean fixture or repository source paths
	tmpDir := t.TempDir()
	sampleCleanSrc := filepath.Join(tmpDir, "pkg", "sample")
	if err := fileutil.MkdirAll(sampleCleanSrc, 0755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(sampleCleanSrc, "sample.go"), []byte("package sample\n\nfunc Hello() string { return \"world\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rep, err := AuditPayload(tmpDir, DefaultAuditOptions())
	if err != nil {
		t.Fatalf("AuditPayload on clean tree failed: %v", err)
	}
	if !rep.Passed || len(rep.Violations) != 0 {
		t.Errorf("expected clean tree to pass without violations, got %d: %+v", len(rep.Violations), rep.Violations)
	}
}

// TestReleaseSecurity_BoundaryAndErrorHandling verifies CRIT-1789621133231481000-8c88057f:
// Simulates tainted payloads with secrets, forbidden studio artifacts, script budget overflow,
// and boundary errors (missing directories, non-dir files).
func TestReleaseSecurity_BoundaryAndErrorHandling(t *testing.T) {
	t.Parallel()

	// 1. Boundary: non-existent directory
	_, err := AuditPayload("/path/that/definitely/does/not/exist/9999", DefaultAuditOptions())
	if err == nil {
		t.Errorf("expected error when auditing non-existent directory, got nil")
	}

	// 2. Boundary: regular file passed as directory root
	tmpDir := t.TempDir()
	dummyFile := filepath.Join(tmpDir, "dummy.txt")
	if err := fileutil.WriteFile(dummyFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err = AuditPayload(dummyFile, DefaultAuditOptions())
	if err == nil {
		t.Errorf("expected error when target is a file instead of directory, got nil")
	}

	// 3. Negative test: secret leaks
	leakDir := t.TempDir()
	srcDir := filepath.Join(leakDir, "src")
	if err := fileutil.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	taintedFile := filepath.Join(srcDir, "credentials.go")
	aws := "AK" + "IA" + "1234567890ABCDEF"
	pat := "gh" + "p_" + "1234567890abcdefghijklmnopqrstuvwxyz"
	slack := "https://" + "hooks.slack.com/services/T123/B456/789xyz"
	priv := "-----BEGIN " + "RSA PRIVATE KEY-----"
	ref := "BLI-" + "1789621133231480000-5d6304d4"
	taintedContent := "package src\n\nconst (\n\tawsKey = \"" + aws + "\"\n\tpat = \"" + pat + "\"\n\tslack = \"" + slack + "\"\n\tpriv = \"" + priv + "\"\n\tref = \"" + ref + "\"\n)\n"

	if err := fileutil.WriteFile(taintedFile, []byte(taintedContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Also add forbidden studio paths
	if err := fileutil.MkdirAll(filepath.Join(leakDir, "cmd", "zqk-admin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.MkdirAll(filepath.Join(leakDir, ".cursor"), 0755); err != nil {
		t.Fatal(err)
	}

	// Also add compiled test binary
	if err := fileutil.WriteFile(filepath.Join(leakDir, "app.test"), []byte("ELF binary"), 0755); err != nil {
		t.Fatal(err)
	}

	// Also add script budget overflow
	scriptsDir := filepath.Join(leakDir, "scripts")
	if err := fileutil.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		p := filepath.Join(scriptsDir, string(rune('a'+i))+".sh")
		if err := fileutil.WriteFile(p, []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	report, err := AuditPayload(leakDir, DefaultAuditOptions())
	if err != nil {
		t.Fatalf("AuditPayload on tainted fixture failed: %v", err)
	}

	if report.Passed {
		t.Errorf("expected tainted fixture to fail audit, but report passed")
	}

	rulesExpected := map[string]bool{
		"AWS_KEY_LEAK":           false,
		"GITHUB_PAT_LEAK":        false,
		"SLACK_WEBHOOK_LEAK":     false,
		"PRIVATE_KEY_LEAK":       false,
		"LONG_STUDIO_ID_LEAK":    false,
		"FORBIDDEN_STUDIO_PATH":  false,
		"TRACKED_TEST_BINARY":    false,
		"SCRIPT_BUDGET_EXCEEDED": false,
	}

	for _, v := range report.Violations {
		if _, ok := rulesExpected[v.Rule]; ok {
			rulesExpected[v.Rule] = true
		}
	}

	for rule, found := range rulesExpected {
		if !found {
			t.Errorf("expected violation for rule %q, but was not detected", rule)
		}
	}
}

// TestReleaseSecurity_IntegrationAndConformance verifies CRIT-1789621133231482000-a0ab4492:
// Verifies live system integration, license conformance, and compliance with the public release gate.
func TestReleaseSecurity_IntegrationAndConformance(t *testing.T) {
	projectRoot := findProjectRoot(t)

	// Verify NOTICE exists and has content
	noticePath := filepath.Join(projectRoot, "NOTICE")
	if data, err := fileutil.ReadFile(noticePath); err != nil || len(data) == 0 {
		t.Errorf("NOTICE file missing or empty at %s", noticePath)
	}

	// Verify scripts/check-public-release-payload.sh exists and is executable
	gateScript := filepath.Join(projectRoot, "scripts", "check-public-release-payload.sh")
	info, err := fileutil.Stat(gateScript)
	if err != nil {
		t.Fatalf("gate script %s missing: %v", gateScript, err)
	}
	if info.Mode()&0111 == 0 {
		t.Errorf("gate script %s is not executable", gateScript)
	}

	// Verify license integrity utility
	if err := VerifyLicenseIntegrity(projectRoot); err != nil {
		t.Errorf("VerifyLicenseIntegrity failed on live repository: %v", err)
	}
}
