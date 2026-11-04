// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package security

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSecurityErrors_TaxonomyAndFormatting(t *testing.T) {
	sentinels := []struct {
		err  error
		text string
	}{
		{ErrAuthnNoCredential, "security: fail-closed auth denied — no credential provided"},
		{ErrAuthnInvalidInput, "security: fail-closed auth denied — invalid input"},
		{ErrMCPInvalidToolName, "security: MCP bind — invalid tool name"},
		{ErrSandboxPathDenied, "security: file operation denied by sandbox allowlist"},
		{ErrZeroTimeout, "security: fail-closed timeouts require a positive non-zero deadline"},
		{ErrTimeoutBlocked, "execution blocked by enforced timeout"},
		{ErrPathTraversalDetected, "security: path traversal detected"},
		{ErrSecurityGateViolation, "security gate violation"},
	}

	for _, s := range sentinels {
		if s.err.Error() != s.text {
			t.Errorf("error text mismatch: got %q, want %q", s.err.Error(), s.text)
		}
		wrapped := fmt.Errorf("operation failed: %w", s.err)
		if !errors.Is(wrapped, s.err) {
			t.Errorf("expected wrapped error to match sentinel %v", s.err)
		}
	}
}

func TestNormalizeAndValidatePath_EdgeCasesAndTraversals(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		prefixes  []string
		wantPath  string
		expectErr bool
	}{
		{
			name:      "redundant slashes and relative parent within prefix",
			input:     "/opt/zqk//base/../base/bin/tool",
			prefixes:  []string{"/opt/zqk/base"},
			wantPath:  "/opt/zqk/base/bin/tool",
			expectErr: false,
		},
		{
			name:      "path traversal outside prefix",
			input:     "/opt/zqk/base/../../etc/shadow",
			prefixes:  []string{"/opt/zqk/base"},
			wantPath:  "",
			expectErr: true,
		},
		{
			name:      "slash prefix with traversal to root",
			input:     "/",
			prefixes:  []string{"/"},
			wantPath:  "",
			expectErr: true,
		},
		{
			name:      "double dot relative path without prefixes",
			input:     "..",
			prefixes:  nil,
			wantPath:  "",
			expectErr: true,
		},
		{
			name:      "single dot relative path without prefixes",
			input:     ".",
			prefixes:  nil,
			wantPath:  "",
			expectErr: true,
		},
		{
			name:      "valid relative path without prefixes",
			input:     "sub/folder/file.txt",
			prefixes:  nil,
			wantPath:  "sub/folder/file.txt",
			expectErr: false,
		},
		{
			name:      "embedded null byte injection",
			input:     "/opt/zqk/base/file\x00.txt",
			prefixes:  []string{"/opt/zqk/base"},
			wantPath:  "",
			expectErr: true,
		},
		{
			name:      "multiple prefixes where second prefix matches",
			input:     "/var/data/app/file.txt",
			prefixes:  []string{"/opt/zqk", "/var/data/app"},
			wantPath:  "/var/data/app/file.txt",
			expectErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeAndValidatePath(tc.input, tc.prefixes...)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error for input %q, got nil", tc.input)
				}
				if !errors.Is(err, ErrPathTraversalDetected) {
					t.Fatalf("expected ErrPathTraversalDetected, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantPath {
				t.Fatalf("got path %q, want %q", got, tc.wantPath)
			}
		})
	}
}

func TestSandbox_AddRootAndBoundaryContainment(t *testing.T) {
	sb := NewSandbox("", ".", "/valid/root/one")
	roots := sb.Roots()
	if len(roots) != 1 || roots[0] != "/valid/root/one" {
		t.Fatalf("expected 1 root [/valid/root/one], got %v", roots)
	}

	// Add empty or dot roots (should be ignored)
	sb.AddRoot("")
	sb.AddRoot(".")
	if len(sb.Roots()) != 1 {
		t.Fatalf("expected roots length to remain 1, got %d", len(sb.Roots()))
	}

	// Add duplicate root (should be ignored)
	sb.AddRoot("/valid/root/one")
	if len(sb.Roots()) != 1 {
		t.Fatalf("expected duplicate root to be ignored, got %d roots", len(sb.Roots()))
	}

	// Add second valid root
	sb.AddRoot("/valid/root/two")
	if len(sb.Roots()) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(sb.Roots()))
	}

	// Non-existent path under root still validates statically
	nonExistent := "/valid/root/one/nonexistent/file.txt"
	validPath, err := sb.ValidatePath(nonExistent)
	if err != nil {
		t.Fatalf("unexpected error validating non-existent path: %v", err)
	}
	if validPath != nonExistent {
		t.Fatalf("got %q, want %q", validPath, nonExistent)
	}
}

func TestSandbox_SymlinkInsideSandboxAllowed(t *testing.T) {
	root := t.TempDir()
	targetFile := filepath.Join(root, "target.txt")
	if err := fileutil.WriteFile(targetFile, []byte("contained data"), 0600); err != nil {
		t.Fatalf("failed to write target file: %v", err)
	}

	linkFile := filepath.Join(root, "symlink_to_target.txt")
	if err := fileutil.Symlink(targetFile, linkFile); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	sb := NewSandbox(root)
	if !sb.IsAllowed(linkFile) {
		t.Fatalf("expected symlink pointing inside sandbox to be allowed")
	}

	validated, err := sb.ValidatePath(linkFile)
	if err != nil {
		t.Fatalf("unexpected error validating internal symlink: %v", err)
	}
	if validated != linkFile {
		t.Fatalf("expected validated path %q, got %q", linkFile, validated)
	}
}

func TestReleaseSecurity_G11AllowlistedScriptMatrix(t *testing.T) {
	allowedScripts := []string{
		"check-public-release-payload.sh",
		"install-public-push-guard.sh",
		"package-community.sh",
		"install.sh",
		"generate-openvex.sh",
		"build-bootstrap-archive.sh",
		"open-core/deploy-docs-portal.sh",
		"starter_kernel_graph/seed.sh",
		"onboarding_roadmap/check.sh",
		"default_agent_skills/sync.sh",
		"default_policies/vet.sh",
		"demos/flight_check.sh",
	}

	for _, rel := range allowedScripts {
		if !g11AllowlistedScript(rel) {
			t.Errorf("expected script %q to be allowlisted under G11 rules", rel)
		}
	}

	disallowedScripts := []string{
		"arbitrary-hack.sh",
		"custom/test.sh",
		"ci/deploy.sh",
		"admin/purge.sh",
	}

	for _, rel := range disallowedScripts {
		if g11AllowlistedScript(rel) {
			t.Errorf("expected script %q to be disallowed under G11 rules", rel)
		}
	}
}

func TestReleaseSecurity_BinaryDetection(t *testing.T) {
	// 1. Data <= 1024 with zero byte
	shortWithZero := []byte("hello\x00world")
	if !isBinary(shortWithZero) {
		t.Errorf("expected shortWithZero to be classified as binary")
	}

	// 2. Data <= 1024 without zero byte
	shortClean := []byte("hello world simple ascii text")
	if isBinary(shortClean) {
		t.Errorf("expected shortClean to be classified as non-binary")
	}

	// 3. Data > 1024 with zero byte within first 1024
	longWithZero := make([]byte, 2048)
	for i := range longWithZero {
		longWithZero[i] = 'A'
	}
	longWithZero[512] = 0
	if !isBinary(longWithZero) {
		t.Errorf("expected longWithZero to be classified as binary")
	}

	// 4. Data > 1024 with zero byte ONLY AFTER 1024
	longZeroAfter1024 := make([]byte, 2048)
	for i := range longZeroAfter1024 {
		longZeroAfter1024[i] = 'B'
	}
	longZeroAfter1024[1500] = 0
	if isBinary(longZeroAfter1024) {
		t.Errorf("expected longZeroAfter1024 to be classified as non-binary because zero is beyond inspection window")
	}

	// 5. Data > 1024 completely clean
	longClean := make([]byte, 2048)
	for i := range longClean {
		longClean[i] = 'C'
	}
	if isBinary(longClean) {
		t.Errorf("expected longClean to be classified as non-binary")
	}
}

func TestReleaseSecurity_AuditPayloadVariations(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Forbidden studio paths
	codegenDir := filepath.Join(tmpDir, "cmd", "codegen_runner")
	if err := fileutil.MkdirAll(codegenDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create codegen dir: %v", err)
	}
	patternDir := filepath.Join(tmpDir, "cmd", "pattern-cli")
	if err := fileutil.MkdirAll(patternDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create pattern dir: %v", err)
	}
	agentsDir := filepath.Join(tmpDir, ".agents")
	if err := fileutil.MkdirAll(agentsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create agents dir: %v", err)
	}

	// 2. Scripts directory with allowlisted and non-script files
	scriptsDir := filepath.Join(tmpDir, "scripts")
	if err := fileutil.MkdirAll(filepath.Join(scriptsDir, "open-core"), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create scripts sub-dir: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(scriptsDir, "open-core", "deploy.sh"), []byte("#!/bin/sh\n"), paths.FilePerm755); err != nil {
		t.Fatalf("failed to write allowlisted script: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(scriptsDir, "README.md"), []byte("# Scripts\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write readme in scripts: %v", err)
	}

	// 3. Binary file that should be skipped during secret scan
	binFile := filepath.Join(tmpDir, "sample.bin")
	binContent := append([]byte("fake_binary_prefix\x00"), make([]byte, 200)...)
	if err := fileutil.WriteFile(binFile, binContent, paths.FilePerm644); err != nil {
		t.Fatalf("failed to write binary file: %v", err)
	}

	// 4. File-level ignore prefix
	ignoreFile := filepath.Join(tmpDir, "ignored_file.txt")
	fakeKey := "AK" + "IA" + "1234567890ABCDEF"
	if err := fileutil.WriteFile(ignoreFile, []byte(fakeKey), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write ignore file: %v", err)
	}

	opts := DefaultAuditOptions()
	opts.IgnorePrefixes = append(opts.IgnorePrefixes, "ignored_file.txt")

	rep, err := AuditPayload(tmpDir, opts)
	if err != nil {
		t.Fatalf("AuditPayload failed: %v", err)
	}
	if rep.Passed {
		t.Fatalf("expected audit to fail due to forbidden studio paths")
	}

	rulesFound := make(map[string]int)
	for _, v := range rep.Violations {
		rulesFound[v.Rule]++
	}

	if rulesFound["FORBIDDEN_STUDIO_PATH"] < 3 {
		t.Fatalf("expected at least 3 FORBIDDEN_STUDIO_PATH violations, got %d", rulesFound["FORBIDDEN_STUDIO_PATH"])
	}

	// Disabled options check on a tree with secrets and extra scripts but no forbidden paths
	disabledDir := t.TempDir()
	taintedFile := filepath.Join(disabledDir, "leak.txt")
	taintedData := "AK" + "IA" + "1234567890ABCDEF\n" + "B" + "LI-1234567890123-abcdef"
	if err := fileutil.WriteFile(taintedFile, []byte(taintedData), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write tainted file: %v", err)
	}

	disabledOpts := AuditOptions{
		CheckSecrets:      false,
		CheckStudioLitter: false,
		CheckStudioIDs:    false,
		CheckScriptBudget: false,
	}
	repDisabled, err := AuditPayload(disabledDir, disabledOpts)
	if err != nil {
		t.Fatalf("AuditPayload with disabled options failed: %v", err)
	}
	if !repDisabled.Passed {
		t.Fatalf("expected disabled options payload to pass, got violations: %+v", repDisabled.Violations)
	}
}

func TestVerifyLicenseIntegrity_DetailedCases(t *testing.T) {
	// Case 1: Missing LICENSE
	emptyDir := t.TempDir()
	if err := VerifyLicenseIntegrity(emptyDir); err == nil {
		t.Fatalf("expected error for missing LICENSE, got nil")
	}

	// Case 2: Empty LICENSE
	emptyLicDir := t.TempDir()
	licPath := filepath.Join(emptyLicDir, "LICENSE")
	if err := fileutil.WriteFile(licPath, []byte("   \n\t"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write empty license: %v", err)
	}
	if err := VerifyLicenseIntegrity(emptyLicDir); err == nil || !strings.Contains(err.Error(), "LICENSE file is empty") {
		t.Fatalf("expected 'LICENSE file is empty' error, got %v", err)
	}

	// Case 3: Invalid license content (neither Apache nor MIT)
	invalidLicDir := t.TempDir()
	if err := fileutil.WriteFile(filepath.Join(invalidLicDir, "LICENSE"), []byte("Proprietary Software License 2026"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write invalid license: %v", err)
	}
	if err := VerifyLicenseIntegrity(invalidLicDir); err == nil || !strings.Contains(err.Error(), "does not declare Apache or MIT license") {
		t.Fatalf("expected 'does not declare Apache or MIT license' error, got %v", err)
	}

	// Case 4: Valid MIT License without NOTICE
	mitDir := t.TempDir()
	if err := fileutil.WriteFile(filepath.Join(mitDir, "LICENSE"), []byte("MIT License\nCopyright 2026"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write MIT license: %v", err)
	}
	if err := VerifyLicenseIntegrity(mitDir); err != nil {
		t.Fatalf("expected valid MIT license to pass, got: %v", err)
	}

	// Case 5: Valid Apache License with empty NOTICE
	apacheEmptyNoticeDir := t.TempDir()
	if err := fileutil.WriteFile(filepath.Join(apacheEmptyNoticeDir, "LICENSE"), []byte("Apache License, Version 2.0"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write Apache license: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(apacheEmptyNoticeDir, "NOTICE"), []byte("   \n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write empty notice: %v", err)
	}
	if err := VerifyLicenseIntegrity(apacheEmptyNoticeDir); err == nil || !strings.Contains(err.Error(), "NOTICE file exists but is empty") {
		t.Fatalf("expected 'NOTICE file exists but is empty' error, got %v", err)
	}

	// Case 6: Valid Apache License with valid NOTICE
	apacheValidNoticeDir := t.TempDir()
	if err := fileutil.WriteFile(filepath.Join(apacheValidNoticeDir, "LICENSE"), []byte("Apache License, Version 2.0"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write Apache license: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(apacheValidNoticeDir, "NOTICE"), []byte("ZQK System Copyright 2026"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write valid notice: %v", err)
	}
	if err := VerifyLicenseIntegrity(apacheValidNoticeDir); err != nil {
		t.Fatalf("expected valid Apache license with non-empty NOTICE to pass, got: %v", err)
	}
}

func createMockBinary(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := fileutil.WriteFile(p, []byte(content), 0755); err != nil {
		t.Fatalf("failed to write mock binary %s: %v", name, err)
	}
	return p
}

func TestAuditStaticVulnerabilities_Scenarios(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Scenario 1: Binary not found
	isolatedHome := t.TempDir()
	t.Setenv("HOME", isolatedHome)
	t.Setenv("PATH", isolatedHome) // No govulncheck

	res, err := AuditStaticVulnerabilities(ctx, isolatedHome, "./...")
	if err != nil {
		t.Fatalf("unexpected error when binary missing: %v", err)
	}
	if res.Executed {
		t.Fatalf("expected Executed to be false when govulncheck missing")
	}

	// Scenario 2: Mock govulncheck with 0 vulnerabilities
	binDir := t.TempDir()
	scriptClean := "#!/bin/sh\necho 'Scanning...'\necho 'No vulnerabilities found'\nexit 0\n"
	createMockBinary(t, binDir, "govulncheck", scriptClean)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	resClean, errClean := AuditStaticVulnerabilities(ctx, binDir, "./...")
	if errClean != nil {
		t.Fatalf("expected clean scan to succeed, got %v", errClean)
	}
	if !resClean.Executed {
		t.Fatalf("expected Executed to be true for mock govulncheck")
	}
	if resClean.VulnerabilitiesCount != 0 {
		t.Fatalf("expected 0 vulnerabilities, got %d", resClean.VulnerabilitiesCount)
	}

	// Scenario 3: Mock govulncheck with 2 vulnerabilities
	scriptVuln := "#!/bin/sh\necho 'Your code is affected by 2 vulnerabilities'\nexit 0\n"
	createMockBinary(t, binDir, "govulncheck", scriptVuln)

	resVuln, errVuln := AuditStaticVulnerabilities(ctx, binDir, "./...")
	if errVuln == nil {
		t.Fatalf("expected error when vulnerabilities detected, got nil")
	}
	if resVuln.VulnerabilitiesCount != 2 {
		t.Fatalf("expected 2 vulnerabilities count, got %d", resVuln.VulnerabilitiesCount)
	}

	// Scenario 4: Mock govulncheck failure (non-zero exit)
	scriptFail := "#!/bin/sh\necho 'internal fatal failure' >&2\nexit 1\n"
	createMockBinary(t, binDir, "govulncheck", scriptFail)

	_, errFail := AuditStaticVulnerabilities(ctx, binDir, "./...")
	if errFail == nil {
		t.Fatalf("expected error on govulncheck command failure, got nil")
	}
}

func TestValidateToolName_ComprehensiveBoundaries(t *testing.T) {
	// Length boundaries
	singleChar := "a"
	if err := ValidateToolName(singleChar); err != nil {
		t.Fatalf("expected single character %q to be valid, got: %v", singleChar, err)
	}

	maxChar := strings.Repeat("x", 256)
	if err := ValidateToolName(maxChar); err != nil {
		t.Fatalf("expected 256-character tool name to be valid, got: %v", err)
	}

	overMaxChar := strings.Repeat("x", 257)
	if err := ValidateToolName(overMaxChar); err == nil {
		t.Fatalf("expected 257-character tool name to fail, got nil")
	}

	// Allowed special characters in tool name
	allowedSpecial := "my-tool_name.sub:action123"
	if err := ValidateToolName(allowedSpecial); err != nil {
		t.Fatalf("expected tool name with allowed special chars %q to be valid, got: %v", allowedSpecial, err)
	}
	if !IsValidToolName(allowedSpecial) {
		t.Fatalf("expected IsValidToolName(%q) to return true", allowedSpecial)
	}

	// Disallowed special characters
	disallowedChars := []string{
		"tool@name", "tool#action", "tool$var", "tool%percent",
		"tool^caret", "tool&amp", "tool*star", "tool(paren",
		"tool)paren", "tool+plus", "tool=equals", "tool/slash",
		"tool\\backslash", "tool?question", "tool<left", "tool>right",
		"tool~tilde", "tool`backtick", "tool!excl", "tool,comma",
	}

	for _, invalid := range disallowedChars {
		if err := ValidateToolName(invalid); err == nil {
			t.Errorf("expected tool name %q to fail validation, got nil", invalid)
		}
		if IsValidToolName(invalid) {
			t.Errorf("expected IsValidToolName(%q) to return false", invalid)
		}
	}
}

func TestSecurity_ConcurrentStressAndRaceSafety(t *testing.T) {
	var wg sync.WaitGroup
	workers := 16
	iterations := 40

	sb := NewSandbox("/workspace/main", "/tmp/zqk_shared")

	for i := 0; i < workers; i++ {
		workerID := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("security_stress_worker_%d", workerID), "concurrent security stress testing").
			WithWaitGroup(&wg).
			StartSimple(func() {
				for j := 0; j < iterations; j++ {
					// 1. Tool name validation
					toolName := fmt.Sprintf("worker_%d:action.%d", workerID, j)
					if err := ValidateToolName(toolName); err != nil {
						t.Errorf("unexpected tool validation failure on %q: %v", toolName, err)
					}

					// 2. Path normalization & traversal check
					validSubPath := fmt.Sprintf("/workspace/main/sub_%d/item_%d.txt", workerID, j)
					cleanPath, err := NormalizeAndValidatePath(validSubPath, "/workspace/main")
					if err != nil || cleanPath != validSubPath {
						t.Errorf("unexpected traversal false positive on %q: %v", validSubPath, err)
					}

					traversalAttempt := fmt.Sprintf("/workspace/main/../../etc/passwd_%d", j)
					if _, err := NormalizeAndValidatePath(traversalAttempt, "/workspace/main"); err == nil {
						t.Errorf("expected traversal error on %q, got nil", traversalAttempt)
					}

					// 3. Sandbox isolation check
					if !sb.IsAllowed(validSubPath) {
						t.Errorf("expected sandbox to allow %q", validSubPath)
					}
					if sb.IsAllowed(traversalAttempt) {
						t.Errorf("expected sandbox to block %q", traversalAttempt)
					}
				}
			})
	}

	wg.Wait()
}
