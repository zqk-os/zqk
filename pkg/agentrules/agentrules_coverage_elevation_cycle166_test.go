package agentrules

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestManifestPath_Composition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		root     string
		expected string
	}{
		{
			name:     "standard root",
			root:     "/workspace/project",
			expected: filepath.Join("/workspace/project", paths.ProcessInternalConfigsDir, ManifestFileName),
		},
		{
			name:     "root with trailing slash",
			root:     "/workspace/project/",
			expected: filepath.Join("/workspace/project", paths.ProcessInternalConfigsDir, ManifestFileName),
		},
		{
			name:     "relative root",
			root:     ".",
			expected: filepath.Join(".", paths.ProcessInternalConfigsDir, ManifestFileName),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ManifestPath(tc.root)
			if got != tc.expected {
				t.Fatalf("ManifestPath(%q) = %q; want %q", tc.root, got, tc.expected)
			}
		})
	}
}

func TestResolveRulesDir_Precedence(t *testing.T) {
	root := t.TempDir()

	// 1. Explicit flag takes top precedence
	pFlag := ResolveRulesDir(root, "custom/flag/path")
	expectedFlag := filepath.Join(root, "custom", "flag", "path")
	if pFlag != expectedFlag {
		t.Fatalf("ResolveRulesDir with flag: got %q, want %q", pFlag, expectedFlag)
	}

	// 2. Environment variable override when flag is empty
	t.Setenv(zqkenv.AgentRulesDir().Name(), "custom/env/rules")
	pEnv := ResolveRulesDir(root, "")
	expectedEnv := filepath.Join(root, "custom", "env", "rules")
	if pEnv != expectedEnv {
		t.Fatalf("ResolveRulesDir with env: got %q, want %q", pEnv, expectedEnv)
	}
	t.Setenv(zqkenv.AgentRulesDir().Name(), "")

	// 3. Candidate directory: .cursor/rules first
	cursorRules := filepath.Join(root, ".cursor", "rules")
	if err := fileutil.EnsureDir(cursorRules); err != nil {
		t.Fatalf("failed to create cursor rules dir: %v", err)
	}
	pCursor := ResolveRulesDir(root, "")
	if pCursor != cursorRules {
		t.Fatalf("ResolveRulesDir candidate 1: got %q, want %q", pCursor, cursorRules)
	}
	if err := os.RemoveAll(cursorRules); err != nil {
		t.Fatalf("failed to clean up cursor rules dir: %v", err)
	}

	// 4. Candidate directory: .ide/rules when .cursor/rules does not exist
	ideRules := filepath.Join(root, ".ide", "rules")
	if err := fileutil.EnsureDir(ideRules); err != nil {
		t.Fatalf("failed to create ide rules dir: %v", err)
	}
	pIDE := ResolveRulesDir(root, "")
	if pIDE != ideRules {
		t.Fatalf("ResolveRulesDir candidate 2: got %q, want %q", pIDE, ideRules)
	}
	if err := os.RemoveAll(ideRules); err != nil {
		t.Fatalf("failed to clean up ide rules dir: %v", err)
	}

	// 5. Fallback to default when neither candidate exists
	pDefault := ResolveRulesDir(root, "")
	expectedDefault := filepath.Join(root, ".cursor", "rules")
	if pDefault != expectedDefault {
		t.Fatalf("ResolveRulesDir fallback: got %q, want %q", pDefault, expectedDefault)
	}
}

func TestLoadManifest_SuccessAndEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("missing manifest file returns read error", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		_, err := LoadManifest(root)
		if err == nil {
			t.Fatal("expected error for non-existent manifest file, got nil")
		}
		if !strings.Contains(err.Error(), "read agent rules manifest") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("corrupt YAML returns parse error", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		manifestFile := ManifestPath(root)
		if err := fileutil.EnsureDir(filepath.Dir(manifestFile)); err != nil {
			t.Fatalf("EnsureDir failed: %v", err)
		}
		if err := fileutil.WriteSecureFile(manifestFile, []byte("version: [unclosed sequence\n")); err != nil {
			t.Fatalf("WriteSecureFile failed: %v", err)
		}

		_, err := LoadManifest(root)
		if err == nil {
			t.Fatal("expected parse error for malformed YAML, got nil")
		}
		if !strings.Contains(err.Error(), errMsgParseManifest) {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("zero or missing version returns missing version error", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		manifestFile := ManifestPath(root)
		if err := fileutil.EnsureDir(filepath.Dir(manifestFile)); err != nil {
			t.Fatalf("EnsureDir failed: %v", err)
		}
		if err := fileutil.WriteSecureFile(manifestFile, []byte("rules:\n  - test.mdc\n")); err != nil {
			t.Fatalf("WriteSecureFile failed: %v", err)
		}

		_, err := LoadManifest(root)
		if err == nil {
			t.Fatal("expected error for missing version, got nil")
		}
		if !strings.Contains(err.Error(), errMsgMissingVersion) {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("valid manifest parses successfully", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		manifestFile := ManifestPath(root)
		if err := fileutil.EnsureDir(filepath.Dir(manifestFile)); err != nil {
			t.Fatalf("EnsureDir failed: %v", err)
		}
		content := "version: 1\nrules:\n  - 01-rule.mdc\n  - 02-rule.mdc\n"
		if err := fileutil.WriteSecureFile(manifestFile, []byte(content)); err != nil {
			t.Fatalf("WriteSecureFile failed: %v", err)
		}

		m, err := LoadManifest(root)
		if err != nil {
			t.Fatalf("LoadManifest failed: %v", err)
		}
		if m.Version != 1 {
			t.Fatalf("expected Version 1, got %d", m.Version)
		}
		expectedRules := []string{"01-rule.mdc", "02-rule.mdc"}
		if !reflect.DeepEqual(m.Rules, expectedRules) {
			t.Fatalf("expected rules %v, got %v", expectedRules, m.Rules)
		}
	})
}

func TestListRuleFiles_FilteringAndSorting(t *testing.T) {
	t.Parallel()

	t.Run("missing directory returns read error", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		_, err := ListRuleFiles(root, "non_existent_rules_dir")
		if err == nil {
			t.Fatal("expected error for non-existent rules dir, got nil")
		}
		if !strings.Contains(err.Error(), "read agent rules dir") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("filters out non-mdc files and subdirectories and sorts basenames", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rulesDir := filepath.Join(root, ".cursor", "rules")
		if err := fileutil.EnsureDir(rulesDir); err != nil {
			t.Fatalf("EnsureDir failed: %v", err)
		}

		// Create files in unsorted order
		files := map[string]string{
			"z-rule.mdc":       "z content",
			"a-rule.mdc":       "a content",
			"m-rule.mdc":       "m content",
			"notes.txt":        "text file",
			"config.yaml":      "yaml file",
			"rule_without_ext": "no ext",
		}
		for name, content := range files {
			if err := fileutil.WriteSecureFile(filepath.Join(rulesDir, name), []byte(content)); err != nil {
				t.Fatalf("failed to write file %s: %v", name, err)
			}
		}

		// Create subdirectories, including one named with .mdc suffix to test IsDir() skip branch
		subDir1 := filepath.Join(rulesDir, "nested_folder")
		if err := fileutil.EnsureDir(subDir1); err != nil {
			t.Fatalf("EnsureDir failed for subDir1: %v", err)
		}
		subDir2 := filepath.Join(rulesDir, "fake_directory.mdc")
		if err := fileutil.EnsureDir(subDir2); err != nil {
			t.Fatalf("EnsureDir failed for subDir2: %v", err)
		}

		got, err := ListRuleFiles(root, "")
		if err != nil {
			t.Fatalf("ListRuleFiles failed: %v", err)
		}

		want := []string{"a-rule.mdc", "m-rule.mdc", "z-rule.mdc"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("ListRuleFiles result = %v; want %v", got, want)
		}
	})
}

func TestDiffRuleSets_EquivalenceAndDiscrepancies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		want        []string
		got         []string
		wantMissing []string
		wantExtra   []string
	}{
		{
			name:        "identical sets",
			want:        []string{"a.mdc", "b.mdc"},
			got:         []string{"a.mdc", "b.mdc"},
			wantMissing: nil,
			wantExtra:   nil,
		},
		{
			name:        "empty sets",
			want:        []string{},
			got:         []string{},
			wantMissing: nil,
			wantExtra:   nil,
		},
		{
			name:        "missing rules on disk",
			want:        []string{"c.mdc", "a.mdc", "b.mdc"},
			got:         []string{"a.mdc"},
			wantMissing: []string{"b.mdc", "c.mdc"},
			wantExtra:   nil,
		},
		{
			name:        "extra rules on disk",
			want:        []string{"a.mdc"},
			got:         []string{"z.mdc", "a.mdc", "x.mdc"},
			wantMissing: nil,
			wantExtra:   []string{"x.mdc", "z.mdc"},
		},
		{
			name:        "both missing and extra rules sorted",
			want:        []string{"z-missing.mdc", "a-common.mdc", "b-missing.mdc"},
			got:         []string{"y-extra.mdc", "a-common.mdc", "c-extra.mdc"},
			wantMissing: []string{"b-missing.mdc", "z-missing.mdc"},
			wantExtra:   []string{"c-extra.mdc", "y-extra.mdc"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			missing, extra := diffRuleSets(tc.want, tc.got)
			if !reflect.DeepEqual(missing, tc.wantMissing) {
				t.Fatalf("diffRuleSets missing = %v; want %v", missing, tc.wantMissing)
			}
			if !reflect.DeepEqual(extra, tc.wantExtra) {
				t.Fatalf("diffRuleSets extra = %v; want %v", extra, tc.wantExtra)
			}
		})
	}
}

func TestValidate_ErrorPropagationAndFormatting(t *testing.T) {
	t.Parallel()

	t.Run("missing manifest errors propagate", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		err := Validate(root, "")
		if err == nil {
			t.Fatal("expected error when manifest is missing")
		}
		if !strings.Contains(err.Error(), "read agent rules manifest") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("missing rules directory errors propagate", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		manifestFile := ManifestPath(root)
		if err := fileutil.EnsureDir(filepath.Dir(manifestFile)); err != nil {
			t.Fatalf("EnsureDir failed: %v", err)
		}
		if err := fileutil.WriteSecureFile(manifestFile, []byte("version: 1\nrules: []\n")); err != nil {
			t.Fatalf("WriteSecureFile failed: %v", err)
		}

		err := Validate(root, "non_existent_rules_dir")
		if err == nil {
			t.Fatal("expected error when rules directory does not exist")
		}
		if !strings.Contains(err.Error(), "read agent rules dir") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("out of sync reports missing and extra files with actionable remedy", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rulesDir := filepath.Join(root, ".cursor", "rules")
		if err := fileutil.EnsureDir(rulesDir); err != nil {
			t.Fatalf("EnsureDir failed: %v", err)
		}
		// Disk has extra.mdc
		if err := fileutil.WriteSecureFile(filepath.Join(rulesDir, "extra.mdc"), []byte("extra")); err != nil {
			t.Fatalf("WriteSecureFile failed: %v", err)
		}

		// Manifest expects missing.mdc
		manifestFile := ManifestPath(root)
		if err := fileutil.EnsureDir(filepath.Dir(manifestFile)); err != nil {
			t.Fatalf("EnsureDir failed: %v", err)
		}
		manifestContent := "version: 1\nrules:\n  - missing.mdc\n"
		if err := fileutil.WriteSecureFile(manifestFile, []byte(manifestContent)); err != nil {
			t.Fatalf("WriteSecureFile failed: %v", err)
		}

		err := Validate(root, "")
		if err == nil {
			t.Fatal("expected out of sync error, got nil")
		}
		errMsg := err.Error()
		if !strings.Contains(errMsg, "missing_on_disk=[missing.mdc]") {
			t.Fatalf("error missing missing_on_disk details: %s", errMsg)
		}
		if !strings.Contains(errMsg, "extra_on_disk=[extra.mdc]") {
			t.Fatalf("error missing extra_on_disk details: %s", errMsg)
		}
		if !strings.Contains(errMsg, "zqk system validate-agent-rules --write-manifest") {
			t.Fatalf("error missing remedy command: %s", errMsg)
		}
	})
}

func TestWriteManifestFromDisk_EnsureDirAndWriteFailures(t *testing.T) {
	t.Parallel()

	t.Run("directory creation failure propagates when parent path is a regular file", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()

		// Set up valid rules directory with a rule
		rulesDir := filepath.Join(root, ".cursor", "rules")
		if err := fileutil.EnsureDir(rulesDir); err != nil {
			t.Fatalf("EnsureDir failed: %v", err)
		}
		if err := fileutil.WriteSecureFile(filepath.Join(rulesDir, "test.mdc"), []byte("content")); err != nil {
			t.Fatalf("WriteSecureFile failed: %v", err)
		}

		// Create a regular file at the location of ".zqk" to block directory creation
		conflictFile := filepath.Join(root, ".zqk")
		if err := os.WriteFile(conflictFile, []byte("blocking directory creation"), 0600); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		err := WriteManifestFromDisk(root, "")
		if err == nil {
			t.Fatal("expected mkdir error when path component is a regular file, got nil")
		}
		if !strings.Contains(err.Error(), errMsgMkdirForManifest) {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("secure file write failure propagates when target file cannot be written", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()

		// Set up valid rules directory with a rule
		rulesDir := filepath.Join(root, ".cursor", "rules")
		if err := fileutil.EnsureDir(rulesDir); err != nil {
			t.Fatalf("EnsureDir failed: %v", err)
		}
		if err := fileutil.WriteSecureFile(filepath.Join(rulesDir, "test.mdc"), []byte("content")); err != nil {
			t.Fatalf("WriteSecureFile failed: %v", err)
		}

		// Create target manifest path as a directory so WriteSecureFile fails when trying to write to it
		dest := ManifestPath(root)
		if err := fileutil.EnsureDir(dest); err != nil {
			t.Fatalf("EnsureDir failed: %v", err)
		}

		err := WriteManifestFromDisk(root, "")
		if err == nil {
			t.Fatal("expected write error when dest path is a directory, got nil")
		}
		if !strings.Contains(err.Error(), "write ") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("successful write creates valid manifest with canonical header", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rulesDir := filepath.Join(root, "my-rules")
		if err := fileutil.EnsureDir(rulesDir); err != nil {
			t.Fatalf("EnsureDir failed: %v", err)
		}
		if err := fileutil.WriteSecureFile(filepath.Join(rulesDir, "01-alpha.mdc"), []byte("alpha")); err != nil {
			t.Fatalf("WriteSecureFile failed: %v", err)
		}
		if err := fileutil.WriteSecureFile(filepath.Join(rulesDir, "02-beta.mdc"), []byte("beta")); err != nil {
			t.Fatalf("WriteSecureFile failed: %v", err)
		}

		if err := WriteManifestFromDisk(root, "my-rules"); err != nil {
			t.Fatalf("WriteManifestFromDisk failed: %v", err)
		}

		dest := ManifestPath(root)
		data, err := fileutil.ReadFile(dest)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "# Canonical registry for IDE/agent rule files") {
			t.Fatal("manifest missing canonical header")
		}

		m, err := LoadManifest(root)
		if err != nil {
			t.Fatalf("LoadManifest failed: %v", err)
		}
		if m.Version != 1 {
			t.Fatalf("expected version 1, got %d", m.Version)
		}
		expectedRules := []string{"01-alpha.mdc", "02-beta.mdc"}
		if !reflect.DeepEqual(m.Rules, expectedRules) {
			t.Fatalf("expected rules %v, got %v", expectedRules, m.Rules)
		}
	})
}
