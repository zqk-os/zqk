// Package testenvroot provides a minimal test project layout (.zqk/process + test-settings)
// without importing pkg/testing (import-cycle hygiene for packages like validation).
package testenvroot

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

type testSettingsShape struct {
	Version string         `yaml:"version"`
	Paths   map[string]any `yaml:"paths"`
}

// Setup creates project layout under testRoot and writes test-settings.yaml (same contract as pkg/testing.SetupTestEnvironment).
func Setup(testRoot string) (string, error) {
	absRoot, err := filepath.Abs(testRoot)
	if err != nil {
		return "", errfmt.Newf("failed to resolve test root path").Wrap(err)
	}
	if err := paths.LayoutUnder(absRoot).
		Dir(paths.ProjectDataDir, paths.DirPerm755).
		Dir(paths.ProcessDir, paths.DirPerm755).
		Dir(filepath.Join(paths.ProcessDir, "goals"), paths.DirPerm755).
		Dir(filepath.Join(paths.ProcessDir, "requirements"), paths.DirPerm755).
		Dir(filepath.Join(paths.ProcessDir, "architecture_decisions"), paths.DirPerm755).
		Dir(filepath.Join(paths.ProcessDir, "backlog_items"), paths.DirPerm755).
		Dir(filepath.Join(paths.ProcessDir, "agent_skills"), paths.DirPerm755).
		Dir(filepath.Join(paths.ProcessDir, "assessment_ratings"), paths.DirPerm755).
		Dir(filepath.Join(paths.ProcessDir, "personas"), paths.DirPerm755).
		Dir(filepath.Join(paths.ProcessDir, "remote_kernels"), paths.DirPerm755).
		Dir(filepath.Join(paths.ProcessDir, "scheduler_jobs"), paths.DirPerm755).
		Dir(filepath.Join(paths.ProcessDir, "watchdog_registrations"), paths.DirPerm755).
		Dir(filepath.Join(paths.ProcessDir, "test_bundles"), paths.DirPerm755).
		Dir(filepath.Join(paths.ProcessDir, "kind_synonyms"), paths.DirPerm755).
		Dir(paths.ProcessInternalObjectSpecsDir, paths.DirPerm755).
		Dir(paths.ProcessInternalLifecyclesDir, paths.DirPerm755).
		Dir(paths.ProcessInternalTraitsDir, paths.DirPerm755).
		Dir(paths.ProcessInternalConfigsDir, paths.DirPerm755).
		Err(); err != nil {
		return "", errfmt.Newf("failed to create test project layout").Wrap(err)
	}
	p := filepath.Join(absRoot, paths.TestSettingsFilename)
	body := testSettingsShape{
		Version: paths.DefaultBrandSettingsVersion,
		Paths:   map[string]any{},
	}
	data, err := yaml.Marshal(body)
	if err != nil {
		return "", errfmt.Newf("failed to marshal test settings").Wrap(err)
	}
	if err := fileutil.WriteSecureFile(p, data); err != nil { //nolint:gosec // test file
		return "", errfmt.Newf("failed to write test settings").Wrap(err)
	}
	return absRoot, nil
}
