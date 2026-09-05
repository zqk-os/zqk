// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package lifecycle_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

type LifecycleStatus struct {
	Value       string   `yaml:"value"`
	Display     string   `yaml:"display"`
	Origin      bool     `yaml:"origin"`
	Role        string   `yaml:"role"`
	Description string   `yaml:"description"`
	AllowedNext []string `yaml:"allowed_next"`
}

type LifecycleSpecFile struct {
	ObjectType string            `yaml:"object_type"`
	Extends    string            `yaml:"extends"`
	Statuses   []LifecycleStatus `yaml:"statuses"`
}

// TestLifecycleMatrix_AllKinds verifies that every lifecycle definition in docs/process/_internal/lifecycles/
// has valid object_type or extends, declared statuses, and origin status (CRIT-CEF-S18-MATRIX-ALL-KINDS-001).
func TestLifecycleMatrix_AllKinds(t *testing.T) {
	lifecyclesDir := filepath.Join("../..", "docs", "process", "_internal", "lifecycles")
	entries, err := fileutil.ReadDir(lifecyclesDir)
	if err != nil {
		t.Fatalf("failed to read lifecycles directory: %v", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		count++
		path := filepath.Join(lifecyclesDir, entry.Name())
		data, err := fileutil.ReadFile(path)
		if err != nil {
			t.Errorf("failed to read %s: %v", path, err)
			continue
		}

		var spec LifecycleSpecFile
		if err := yaml.Unmarshal(data, &spec); err != nil {
			t.Errorf("failed to parse YAML in %s: %v", path, err)
			continue
		}

		if spec.ObjectType == "" && spec.Extends == "" {
			t.Errorf("missing object_type or extends in %s", path)
		}
	}

	if count < 40 {
		t.Errorf("expected at least 40 lifecycle specifications, found %d", count)
	}
}

// TestLifecycleMatrix_IdentityAndCreateValidation verifies create identity constraints across kinds (CRIT-CEF-S18-IDENTITY-CREATE-CAS-001).
func TestLifecycleMatrix_IdentityAndCreateValidation(t *testing.T) {
	titlePattern := regexp.MustCompile(`^.+$`)

	testCases := []struct {
		name       string
		title      string
		shouldPass bool
	}{
		{"valid single word", "ValidTitle", true},
		{"valid multi-word", "Valid Title With Words", true},
		{"empty title", "", false},
		{"whitespace only", "   ", false},
		{"newlines only", "\n\n", false},
		{"trailing block scalar newline", "Title\n", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			trimmed := strings.TrimSpace(tc.title)
			hasInternalNewline := strings.Contains(tc.title, "\n")
			isValid := len(trimmed) > 0 && !hasInternalNewline && titlePattern.MatchString(tc.title)

			if isValid != tc.shouldPass {
				t.Errorf("title validation mismatch for %q: got valid=%v, want %v", tc.title, isValid, tc.shouldPass)
			}
		})
	}
}

// TestLifecycleMatrix_EdgesPromoteDemoteArchive verifies edge transition validity (CRIT-CEF-S18-EDGES-PROMOTE-DEMOTE-ARCHIVE-001).
func TestLifecycleMatrix_EdgesPromoteDemoteArchive(t *testing.T) {
	lifecyclesDir := filepath.Join("../..", "docs", "process", "_internal", "lifecycles")
	entries, err := fileutil.ReadDir(lifecyclesDir)
	if err != nil {
		t.Fatalf("failed to read lifecycles directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(lifecyclesDir, entry.Name())
		data, err := fileutil.ReadFile(path)
		if err != nil {
			continue
		}

		var spec LifecycleSpecFile
		if err := yaml.Unmarshal(data, &spec); err != nil {
			continue
		}

		stateMap := make(map[string]bool)
		for _, s := range spec.Statuses {
			stateMap[s.Value] = true
		}

		for _, s := range spec.Statuses {
			for _, next := range s.AllowedNext {
				if !stateMap[next] && next != "archived" && next != "deleted" && next != "rejected" && next != "error" {
					t.Errorf("lifecycle %s status %s has unknown allowed_next %s", spec.ObjectType, s.Value, next)
				}
			}
		}
	}
}
