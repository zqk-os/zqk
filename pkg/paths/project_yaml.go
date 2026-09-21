package paths

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/stampmemo"
)

// ProjectYAMLConfigRelatives is lookup order for committed project YAML.
// <project_root>/config/zqk.yaml is SSOT. Copies under ProjectDataDir are
// legacy fallbacks only — do not add new keys there.
func ProjectYAMLConfigRelatives() []string {
	return []string{
		filepath.Join(ConfigDir, ZqkConfigFileName),
		filepath.Join(ProjectDataDir, ConfigDir, ProjectConfigFile),
		filepath.Join(ProjectDataDir, ProjectConfigFile),
	}
}

// ProjectYAMLConfigPaths returns absolute candidates for projectRoot.
func ProjectYAMLConfigPaths(projectRoot string) []string {
	rels := ProjectYAMLConfigRelatives()
	out := make([]string, len(rels))
	for i, rel := range rels {
		out[i] = filepath.Join(projectRoot, rel)
	}
	return out
}

// FirstProjectYAMLConfig returns the first existing project YAML file.
// Empty projectRoot walks from cwd. Missing is "".
func FirstProjectYAMLConfig(projectRoot string) string {
	if projectRoot == "" {
		return FirstExistingFromCwdAny(ProjectYAMLConfigRelatives())
	}
	return stampmemo.FirstExisting(ProjectYAMLConfigPaths(projectRoot))
}
