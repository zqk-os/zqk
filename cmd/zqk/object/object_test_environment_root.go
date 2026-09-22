package object

import (
	"path/filepath"

	clctx "github.com/zqk-os/zqk/internal/cli/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

type testSettingsYAMLShapeObject struct {
	Version string         `yaml:"version"`
	Paths   map[string]any `yaml:"paths"`
}

func writeMinimalTestSettingsYAMLObject(testRoot string) error {
	configDir := filepath.Join(testRoot, paths.ConfigDir)
	if err := fileutil.EnsureDir(configDir); err != nil {
		return err
	}
	p := filepath.Join(configDir, paths.ZqkTestConfigFileName)
	body := testSettingsYAMLShapeObject{
		Version: clctx.DefaultBrandSettingsVersion,
		Paths:   map[string]any{},
	}
	data, err := yaml.Marshal(body)
	if err != nil {
		return err
	}
	return fileutil.WriteSecureFile(p, data) //nolint:gosec // test file
}

// setupObjectTestEnvironmentRoot mirrors pkg/testing.SetupTestEnvironment (layout + config/zqk-test.yaml only).
func setupObjectTestEnvironmentRoot(testRoot string) (string, error) {
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
	if err := writeMinimalTestSettingsYAMLObject(absRoot); err != nil {
		return "", errfmt.Newf("failed to write test settings").Wrap(err)
	}
	return absRoot, nil
}
