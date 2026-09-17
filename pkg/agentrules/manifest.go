package agentrules

import (
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// Error messages for agent rules manifest operations.
const (
	errMsgReadManifest      = "read agent rules manifest %s"
	errMsgParseManifest     = "parse agent rules manifest"
	errMsgMissingVersion    = "agent rules manifest version missing or zero"
	errMsgReadRulesDir      = "read agent rules dir %s"
	errMsgManifestOutOfSync = `agent rules manifest out of sync with rules dir (manifest=%s, rules_dir=%s): missing_on_disk=%v extra_on_disk=%v; fix manifest or run: zqk system validate-agent-rules --write-manifest`
	errMsgMarshalManifest   = "marshal agent rules manifest"
	errMsgMkdirForManifest  = "mkdir for manifest"
)

func diffRuleSets(want []string, got []string) (missing []string, extra []string) {
	wm := make(map[string]bool, len(want))
	for _, n := range want {
		wm[n] = true
	}
	gm := make(map[string]bool, len(got))
	for _, n := range got {
		gm[n] = true
	}
	for n := range wm {
		if !gm[n] {
			missing = append(missing, n)
		}
	}
	for n := range gm {
		if !wm[n] {
			extra = append(extra, n)
		}
	}
	slices.Sort(missing)
	slices.Sort(extra)
	return missing, extra
}

// ManifestFileName is the basename for the canonical agent-rule registry under ProcessInternalConfigsDir.
const ManifestFileName = "agent_rules_manifest.yaml"

// DefaultAgentRulesRelativePath is the directory reported when no rules directory can be found.
// Override with ZQK_AGENT_RULES_DIR or --rules-dir when your agent stores rules elsewhere.
const DefaultAgentRulesRelativePath = ".cursor/rules"

// candidateRulesDirs are tried in order when neither the flag nor the environment names a
// directory. Different agents use different locations, and resolving to a fixed path that this
// repo does not have made the documented no-flag invocation fail with "no such file or
// directory" — which is why nothing wired the gate into pre-commit and the manifest was free to
// drift while two checklists claimed pre-commit enforced it.
var candidateRulesDirs = []string{".cursor/rules", ".ide/rules"}

// Manifest lists allowed agent rule files (basenames, typically *.mdc).
type Manifest struct {
	Version int      `yaml:"version"`
	Rules   []string `yaml:"rules"`
}

// ManifestPath returns the path to the manifest file under projectRoot.
func ManifestPath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProcessInternalConfigsDir, ManifestFileName)
}

// ResolveRulesDir returns the absolute path to the rules directory.
// Precedence: rulesDirFlag (non-empty) > ZQK_AGENT_RULES_DIR > first existing candidateRulesDirs
// > DefaultAgentRulesRelativePath.
//
// An explicit flag or environment value is returned as given, even if absent, so a caller that
// names a directory gets an error about that directory rather than a silent substitution.
func ResolveRulesDir(projectRoot string, rulesDirFlag string) string {
	if rulesDirFlag != "" {
		return filepath.Join(projectRoot, filepath.Clean(rulesDirFlag))
	}
	if v := zqkenv.AgentRulesDir().Get(); v != "" {
		return filepath.Join(projectRoot, filepath.Clean(v))
	}
	for _, c := range candidateRulesDirs {
		p := filepath.Join(projectRoot, filepath.Clean(c))
		if info, err := fileutil.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return filepath.Join(projectRoot, filepath.Clean(DefaultAgentRulesRelativePath))
}

// LoadManifest reads and parses the manifest YAML.
func LoadManifest(projectRoot string) (*Manifest, error) {
	p := ManifestPath(projectRoot)
	data, err := fileutil.ReadFile(p)
	if err != nil {
		return nil, errfmt.Newf(errMsgReadManifest, p).Wrap(err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, errfmt.Newf(errMsgParseManifest).Wrap(err)
	}
	if m.Version == 0 {
		return nil, errfmt.Errorf(errMsgMissingVersion)
	}
	return &m, nil
}

// ListRuleFiles returns sorted basenames of *.mdc under the resolved rules directory.
func ListRuleFiles(projectRoot string, rulesDirFlag string) ([]string, error) {
	dir := ResolveRulesDir(projectRoot, rulesDirFlag)
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		return nil, errfmt.Newf(errMsgReadRulesDir, dir).Wrap(err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".mdc") {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out, nil
}

// Validate checks manifest rules match exactly the *.mdc files on disk (set equality).
func Validate(projectRoot string, rulesDirFlag string) error {
	want, err := LoadManifest(projectRoot)
	if err != nil {
		return err
	}
	got, err := ListRuleFiles(projectRoot, rulesDirFlag)
	if err != nil {
		return err
	}
	missing, extra := diffRuleSets(want.Rules, got)
	if len(missing) > 0 || len(extra) > 0 {
		mPath := ManifestPath(projectRoot)
		rdir := ResolveRulesDir(projectRoot, rulesDirFlag)
		return errfmt.Errorf(errMsgManifestOutOfSync, mPath, rdir, missing, extra)
	}
	return nil
}

// WriteManifestFromDisk overwrites the manifest with sorted *.mdc basenames from the rules directory.
func WriteManifestFromDisk(projectRoot string, rulesDirFlag string) error {
	names, err := ListRuleFiles(projectRoot, rulesDirFlag)
	if err != nil {
		return err
	}
	m := Manifest{Version: 1, Rules: names}
	out, err := yaml.Marshal(&m)
	if err != nil {
		return errfmt.Newf(errMsgMarshalManifest).Wrap(err)
	}
	header := []byte(`# Canonical registry for IDE/agent rule files (*.mdc under the configured rules dir; default .cursor/rules).
# Validated by: zqk system validate-agent-rules (no flags from repo root; use --write-manifest to refresh this list).
# Override directory: ZQK_AGENT_RULES_DIR or --rules-dir.
`)
	body := append(header, out...)
	dest := ManifestPath(projectRoot)
	if err := fileutil.EnsureDir(filepath.Dir(dest)); err != nil {
		return errfmt.Newf(errMsgMkdirForManifest).Wrap(err)
	}
	if err := fileutil.WriteSecureFile(dest, body); err != nil {
		return errfmt.Errorf("write %s", dest)
	}
	return nil
}
