package objects

import (
	"os"
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
	var missingRole []string
	var missingDesc []string
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if d.IsDir() || filepath.Ext(d.Name()) != ".yaml" {
			return nil
		}
		data, err := fileutil.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", d.Name(), err)
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
			t.Fatalf("parse %s: %v", d.Name(), err)
		}
		for _, s := range doc.Statuses {
			if s.Value == "" {
				continue
			}
			if s.Role == "" {
				missingRole = append(missingRole, d.Name()+":"+s.Value)
			} else if !IsKnownLifecycleRole(s.Role) {
				t.Errorf("%s status %q: unknown role %q", d.Name(), s.Value, s.Role)
			}
			if strings.TrimSpace(s.Description) == "" {
				missingDesc = append(missingDesc, d.Name()+":"+s.Value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(missingRole) > 0 {
		t.Fatalf("statuses missing role (%d): %v", len(missingRole), missingRole)
	}
	if len(missingDesc) > 0 {
		t.Fatalf("statuses missing description (%d): %v", len(missingDesc), missingDesc)
	}
}
