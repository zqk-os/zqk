// Package testenvroot provides a minimal test project layout (docs/process + test-settings)
// without importing pkg/testing (import-cycle hygiene for packages like validation).
package testenvroot

import (
	"path/filepath"

	clctx "github.com/lanceman/zqk/internal/cli/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
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
		Dir(paths.ProcessInternalObjectSpecsDir, paths.DirPerm755).
		Dir(paths.ProcessInternalTraitsDir, paths.DirPerm755).
		Err(); err != nil {
		return "", errfmt.Newf("failed to create test project layout").Wrap(err)
	}
	p := filepath.Join(absRoot, paths.TestSettingsFilename)
	body := testSettingsShape{
		Version: clctx.DefaultBrandSettingsVersion,
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
