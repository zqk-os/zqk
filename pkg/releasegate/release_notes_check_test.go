// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package releasegate_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/releasegate"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestReleaseNotesChecker_SyntheticNotes(t *testing.T) {
	tempDir := t.TempDir()

	validContent := `# Release v0.9.0 - Next Gen Capability

## Highlights & Strategic Capabilities
This release introduces high performance decentralized consensus, cryptographic audit ledgers,
and hardened peer verification across all distributed runtime topologies. We provide extensive
support for zero-copy stream processing, asynchronous checkpointing, and dynamic peer discovery.
` + strings.Repeat("Detailed architectural elaboration of production capabilities and runtime enhancements.\n", 30) + `
## Community Issue Burndown & Bug Fixes
- Fix race condition during peer election timeout.
- Resolve buffer overflow in UDP frame receiver under heavy ingress load.

## Upgrade Guide
Upgrade to v0.9.0 requires running migration scripts for persistent state partitions.
`

	validPath := filepath.Join(tempDir, "v0.9.0.md")
	if err := fileutil.WriteStandardFile(validPath, []byte(validContent)); err != nil {
		t.Fatalf("failed to write valid note: %v", err)
	}

	opts := releasegate.DefaultReleaseNotesOptions(tempDir)
	checker, err := releasegate.NewReleaseNotesChecker(opts)
	if err != nil {
		t.Fatalf("failed to init checker: %v", err)
	}

	res, err := checker.VerifyFile(validPath)
	if err != nil {
		t.Fatalf("unexpected error verifying file: %v", err)
	}
	if !res.Passed {
		t.Fatalf("expected valid file to pass, got issues: %v", res.Issues)
	}

	// Test thin release note (< 2000 chars)
	thinPath := filepath.Join(tempDir, "v0.9.1.md")
	thinContent := `# Release v0.9.1
## Highlights & Strategic Capabilities
Short note.
## Community Issue Burndown & Bug Fixes
None.
## Upgrade Guide
None.
`
	if err := fileutil.WriteStandardFile(thinPath, []byte(thinContent)); err != nil {
		t.Fatalf("failed to write thin note: %v", err)
	}

	resThin, err := checker.VerifyFile(thinPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resThin.Passed {
		t.Fatalf("expected thin release note to fail verification")
	}

	// Test missing header
	missingHeaderPath := filepath.Join(tempDir, "v0.9.2.md")
	missingHeaderContent := strings.Repeat("Elaborate content without required sections.\n", 60)
	if err := fileutil.WriteStandardFile(missingHeaderPath, []byte(missingHeaderContent)); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	resMissing, err := checker.VerifyFile(missingHeaderPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resMissing.Passed {
		t.Fatalf("expected missing header to fail")
	}
	if len(resMissing.MissingHeaders) != 3 {
		t.Fatalf("expected 3 missing headers, got %d", len(resMissing.MissingHeaders))
	}
}

func TestReleaseNotesChecker_RepoReleases(t *testing.T) {
	// Verify actual repository release notes
	repoReleasesDir := filepath.Join("..", "..", "docs", "releases")
	if !fileutil.Exists(repoReleasesDir) {
		t.Skip("docs/releases directory not found relative to package test root")
	}

	opts := releasegate.DefaultReleaseNotesOptions(repoReleasesDir)
	checker, err := releasegate.NewReleaseNotesChecker(opts)
	if err != nil {
		t.Fatalf("failed to init checker: %v", err)
	}

	res, err := checker.VerifyAll()
	if err != nil {
		t.Fatalf("error verifying repo release notes: %v", err)
	}

	for _, fr := range res.FileResults {
		if !fr.Passed {
			t.Errorf("repo release note failed verification: %s issues: %v", fr.Path, fr.Issues)
		}
	}
}
