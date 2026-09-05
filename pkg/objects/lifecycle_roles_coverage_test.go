package objects

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestLifecycleRoles_AllStatusesAnnotated ensures every status in every lifecycle
// YAML carries a cross-kind role (TRACK: [REDACTED-ID]).
func TestLifecycleRoles_AllStatusesAnnotated(t *testing.T) {
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := paths.ModuleRootFromPath(wd)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, paths.ProcessInternalLifecyclesDir)
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var missingRole []string
	var missingDesc []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		data, err := fileutil.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		var doc struct {
			ObjectType string `yaml:"object_type"`
			Statuses   []struct {
				Value       string `yaml:"value"`
				Role        string `yaml:"role"`
				Description string `yaml:"description"`
			} `yaml:"statuses"`
		}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		for _, s := range doc.Statuses {
			if s.Value == "" {
				continue
			}
			if s.Role == "" {
				missingRole = append(missingRole, e.Name()+":"+s.Value)
			} else if !IsKnownLifecycleRole(s.Role) {
				t.Errorf("%s status %q: unknown role %q", e.Name(), s.Value, s.Role)
			}
			if strings.TrimSpace(s.Description) == "" {
				missingDesc = append(missingDesc, e.Name()+":"+s.Value)
			}
		}
	}
	if len(missingRole) > 0 {
		t.Fatalf("statuses missing role (%d): %v", len(missingRole), missingRole)
	}
	if len(missingDesc) > 0 {
		t.Fatalf("statuses missing description (%d): %v", len(missingDesc), missingDesc)
	}
}
