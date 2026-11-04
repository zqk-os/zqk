// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package cleanup

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	testElevatedConfigFilename   = "test_cleanup_cycle186.yaml"
	testElevatedAltFilename      = "test_cleanup_alt.yaml"
	testElevatedMalformedContent = "working_directory: [\nunclosed_yaml_sequence:"
	testCustomParamKey           = "custom_key"
	testCustomParamVal           = "custom_val"
)

func TestStep_UnmarshalYAML_SuccessTypes(t *testing.T) {
	t.Parallel()

	stepTypes := []string{
		"delete_files",
		"truncate_files",
		"cli",
		"build_target",
		"archive",
		"reap_stale_locks",
		"reap_temp_files",
		"enforce_log_retention",
		"reap_orphaned_processes",
	}

	for _, stepType := range stepTypes {
		st := stepType
		t.Run(st, func(t *testing.T) {
			t.Parallel()
			yamlInput := fmt.Sprintf("type: %s\npath: /var/log/*.log\nolder_than: 7d\ncustom: value\n", st)
			var s Step
			if err := yaml.Unmarshal([]byte(yamlInput), &s); err != nil {
				t.Fatalf("unexpected unmarshal error for %s: %v", st, err)
			}
			if s.Type != st {
				t.Fatalf("expected step type %s, got %s", st, s.Type)
			}
			if s.Params == nil {
				t.Fatal("expected non-nil params")
			}
			if p, ok := s.Params["path"].(string); !ok || p != "/var/log/*.log" {
				t.Errorf("expected path param, got %v", s.Params["path"])
			}
			if c, ok := s.Params["custom"].(string); !ok || c != "value" {
				t.Errorf("expected custom param, got %v", s.Params["custom"])
			}
		})
	}
}

func TestStep_UnmarshalYAML_MissingOrNonStringType(t *testing.T) {
	t.Parallel()

	t.Run("missing type field", func(t *testing.T) {
		t.Parallel()
		yamlInput := "path: /tmp/scratch\nretention_days: 14\n"
		var s Step
		if err := yaml.Unmarshal([]byte(yamlInput), &s); err != nil {
			t.Fatalf("unexpected unmarshal error: %v", err)
		}
		if s.Type != "" {
			t.Errorf("expected empty step type, got %s", s.Type)
		}
		if p, ok := s.Params["path"].(string); !ok || p != "/tmp/scratch" {
			t.Errorf("expected path param, got %v", s.Params["path"])
		}
	})

	t.Run("non-string type field integer", func(t *testing.T) {
		t.Parallel()
		yamlInput := "type: 1234\npath: /tmp/scratch\n"
		var s Step
		if err := yaml.Unmarshal([]byte(yamlInput), &s); err != nil {
			t.Fatalf("unexpected unmarshal error: %v", err)
		}
		if s.Type != "" {
			t.Errorf("expected empty step type when type is non-string, got %s", s.Type)
		}
		if v, ok := s.Params["type"].(int); !ok || v != 1234 {
			t.Errorf("expected type in params to remain integer, got %v", s.Params["type"])
		}
	})
}

func TestStep_UnmarshalYAML_NonMappingNode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
	}{
		{name: "scalar string", input: "just_a_string"},
		{name: "sequence list", input: "- item1\n- item2"},
		{name: "integer scalar", input: "42"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var s Step
			err := yaml.Unmarshal([]byte(tc.input), &s)
			if err == nil {
				t.Fatalf("expected unmarshal error for non-mapping node %q, got nil", tc.name)
			}
		})
	}
}

func TestLoadFromPath_AbsolutePaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, testElevatedConfigFilename)
	content := []byte("working_directory: /opt/zqk\nsteps:\n  - type: delete_files\n    path: '*.tmp'\n")
	if err := fileutil.WriteFile(cfgPath, content, paths.FilePerm600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	t.Run("absolute path with project root set", func(t *testing.T) {
		dummyRoot := filepath.Join(dir, "dummy_root")
		cfg, err := LoadFromPath(dummyRoot, cfgPath)
		if err != nil {
			t.Fatalf("LoadFromPath failed: %v", err)
		}
		if cfg.WorkingDirectory != "/opt/zqk" {
			t.Errorf("expected working_directory /opt/zqk, got %s", cfg.WorkingDirectory)
		}
		if len(cfg.Steps) != 1 || cfg.Steps[0].Type != "delete_files" {
			t.Errorf("unexpected steps: %+v", cfg.Steps)
		}
	})

	t.Run("absolute path with empty project root", func(t *testing.T) {
		cfg, err := LoadFromPath("", cfgPath)
		if err != nil {
			t.Fatalf("LoadFromPath failed: %v", err)
		}
		if cfg.WorkingDirectory != "/opt/zqk" {
			t.Errorf("expected working_directory /opt/zqk, got %s", cfg.WorkingDirectory)
		}
	})
}

func TestLoadFromPath_RelativePathEmptyProjectRoot(t *testing.T) {
	t.Parallel()

	// When projectRoot is empty, full = path. For a non-existent relative path,
	// it should return not found error.
	_, err := LoadFromPath("", "nonexistent_relative_config_file.yaml")
	if err == nil {
		t.Fatal("expected error loading non-existent relative path with empty project root")
	}
	expectedSub := "cleanup config not found: nonexistent_relative_config_file.yaml"
	if err.Error() != expectedSub {
		t.Errorf("expected error %q, got %q", expectedSub, err.Error())
	}
}

func TestLoadFromPath_ReadErrorsAndMalformedYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	t.Run("reading directory instead of regular file returns read error", func(t *testing.T) {
		t.Parallel()
		subDir := filepath.Join(dir, "sub_directory_as_file")
		if err := fileutil.MkdirAll(subDir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		_, err := LoadFromPath("", subDir)
		if err == nil {
			t.Fatal("expected error reading directory as config file")
		}
		expectedPrefix := "read cleanup config: "
		if len(err.Error()) < len(expectedPrefix) || err.Error()[:len(expectedPrefix)] != expectedPrefix {
			t.Errorf("expected prefix %q, got %q", expectedPrefix, err.Error())
		}
	})

	t.Run("malformed YAML syntax returns parse error", func(t *testing.T) {
		t.Parallel()
		malformedPath := filepath.Join(dir, "malformed.yaml")
		if err := fileutil.WriteFile(malformedPath, []byte(testElevatedMalformedContent), paths.FilePerm600); err != nil {
			t.Fatalf("failed to write malformed YAML: %v", err)
		}
		_, err := LoadFromPath("", malformedPath)
		if err == nil {
			t.Fatal("expected error parsing malformed YAML")
		}
		expectedPrefix := "parse cleanup config: "
		if len(err.Error()) < len(expectedPrefix) || err.Error()[:len(expectedPrefix)] != expectedPrefix {
			t.Errorf("expected prefix %q, got %q", expectedPrefix, err.Error())
		}
	})
}

func TestLoadFromPath_StampmemoCachingAndInvalidation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cached_config.yaml")
	initialContent := []byte("working_directory: cache_v1\nsteps: []\n")
	if err := fileutil.WriteFile(cfgPath, initialContent, paths.FilePerm600); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	cfg1, err1 := LoadFromPath("", cfgPath)
	if err1 != nil {
		t.Fatalf("first load failed: %v", err1)
	}

	cfg2, err2 := LoadFromPath("", cfgPath)
	if err2 != nil {
		t.Fatalf("second load failed: %v", err2)
	}

	if cfg1 != cfg2 {
		t.Errorf("expected memoized table to return identical pointer, got %p vs %p", cfg1, cfg2)
	}

	time.Sleep(10 * time.Millisecond)
	updatedContent := []byte("working_directory: cache_v2\nsteps:\n  - type: truncate_files\n")
	if err := fileutil.WriteFile(cfgPath, updatedContent, paths.FilePerm600); err != nil {
		t.Fatalf("failed to update config file: %v", err)
	}

	cfg3, err3 := LoadFromPath("", cfgPath)
	if err3 != nil {
		t.Fatalf("third load after update failed: %v", err3)
	}

	if cfg3.WorkingDirectory != "cache_v2" {
		t.Errorf("expected updated working directory cache_v2, got %s", cfg3.WorkingDirectory)
	}
	if len(cfg3.Steps) != 1 || cfg3.Steps[0].Type != "truncate_files" {
		t.Errorf("expected updated step truncate_files, got %+v", cfg3.Steps)
	}
}

func TestLoadFromPath_ConcurrencyStress(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "concurrent_cleanup.yaml")
	content := []byte(fmt.Sprintf("working_directory: .\nsteps:\n  - type: archive\n    %s: %s\n", testCustomParamKey, testCustomParamVal))
	if err := fileutil.WriteFile(cfgPath, content, paths.FilePerm600); err != nil {
		t.Fatalf("failed to write concurrent config: %v", err)
	}

	var wg sync.WaitGroup
	workers := 10
	iterations := 25

	for i := 0; i < workers; i++ {
		workerID := i
		wg.Add(1)
		builder := goroutinelabels.NewGoroutine(
			fmt.Sprintf("cleanup_concurrency_worker_%d", workerID),
			"concurrent load of cleanup config",
		)
		builder.StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				cfg, err := LoadFromPath("", cfgPath)
				if err != nil {
					t.Errorf("worker %d iteration %d failed: %v", workerID, j, err)
					return
				}
				if cfg == nil || len(cfg.Steps) != 1 || cfg.Steps[0].Type != "archive" {
					t.Errorf("worker %d iteration %d got invalid config: %+v", workerID, j, cfg)
					return
				}
			}
		})
	}

	wg.Wait()
}
