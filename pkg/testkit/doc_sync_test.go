// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package testkit_test

import (
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestArchitectureDocIndexIntegrity verifies that documents listed in docs/architecture/INDEX.md exist (L:F-DOC-01 / CRIT-CEF-R8L-DOC-01).
func TestArchitectureDocIndexIntegrity(t *testing.T) {
	indexPath := filepath.Join("../..", "docs", "architecture", "INDEX.md")
	data, err := fileutil.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", indexPath, err)
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.Contains(line, "](./") {
			start := strings.Index(line, "](./") + 4
			end := strings.Index(line[start:], ")")
			if end != -1 {
				docRel := line[start : start+end]
				docPath := filepath.Join("../..", "docs", "architecture", docRel)
				if _, err := fileutil.Stat(docPath); fileutil.IsNotExist(err) {
					t.Errorf("referenced document does not exist: %s", docPath)
				}
			}
		}
	}
}
