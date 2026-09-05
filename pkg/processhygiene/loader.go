package processhygiene

import (
	"context"
	_ "embed"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

//go:embed default_rules.yaml
var embeddedDefaultRules []byte

var internalRulesRelative = filepath.Join(paths.ProcessInternalDir, "process_hygiene_rules.yaml")

// LoadRulesYAML parses rule definitions and returns executable rules.
func LoadRulesYAML(data []byte) ([]Rule, error) {
	var f RulesFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, errfmt.Newf("parse rules yaml").Wrap(err)
	}
	if len(f.Rules) == 0 {
		return nil, errfmt.Errorf("no rules defined")
	}
	out := make([]Rule, 0, len(f.Rules))
	for i, rc := range f.Rules {
		rule, err := compileRule(rc)
		if err != nil {
			return nil, errfmt.Errorf("rule %d (%s): %w", i, rc.ID, err)
		}
		out = append(out, rule)
	}
	return out, nil
}

// CompileRuleConfig builds an executable [Rule] from a declarative [RuleConfig] (YAML or storage row).
func CompileRuleConfig(rc RuleConfig) (Rule, error) {
	return compileRule(rc)
}

func compileRule(rc RuleConfig) (Rule, error) {
	if rc.ID == emptyValue {
		return nil, errfmt.Errorf("missing id")
	}
	m := rc.Match
	if m.Field == emptyValue {
		return nil, errfmt.Errorf("match.field is required")
	}
	nMatchers := 0
	if m.Prefix != emptyValue {
		nMatchers++
	}
	if m.Suffix != emptyValue {
		nMatchers++
	}
	if m.Equals != emptyValue {
		nMatchers++
	}
	if m.Regex != emptyValue {
		nMatchers++
	}
	if nMatchers != 1 {
		return nil, errfmt.Errorf("exactly one of prefix, suffix, equals, regex must be set")
	}

	d := &declarativeRule{
		id:          rc.ID,
		description: rc.Description,
		field:       m.Field,
		prefix:      m.Prefix,
		suffix:      m.Suffix,
		equals:      m.Equals,
	}
	if m.Regex != emptyValue {
		re, err := regexp.Compile(m.Regex)
		if err != nil {
			return nil, errfmt.Newf("compile regex").Wrap(err)
		}
		d.re = re
	}
	return d, nil
}

// LoadRulesFile reads rules from a filesystem path.
func LoadRulesFile(path string) ([]Rule, error) {
	data, err := fileutil.ReadFile(path) //nolint:gosec // path supplied by operator
	if err != nil {
		return nil, err
	}
	return LoadRulesYAML(data)
}

// LoadEmbeddedDefaultRules returns rules from the embedded default_rules.yaml.
func LoadEmbeddedDefaultRules() ([]Rule, error) {
	return LoadRulesYAML(embeddedDefaultRules)
}

// ResolveRules loads rules in order:
//  1. If rulesFile is non-empty, use that file only.
//  2. Else if docs/process/_internal/process_hygiene_rules.yaml exists under projectRoot, use it.
//  3. Else use embedded defaults.
//
// For storage-backed rules, use [ResolveRulesWithOptions] with StorageProvider, SecCtx, and ListCtx set.
func ResolveRules(projectRoot, rulesFile string) ([]Rule, string, error) {
	return ResolveRulesWithOptions(ResolveRulesOptions{
		ProjectRoot: projectRoot,
		RulesFile:   rulesFile,
	})
}

// ResolveRulesOptions configures [ResolveRulesWithOptions].
type ResolveRulesOptions struct {
	ProjectRoot     string
	RulesFile       string
	StorageProvider storage.ObjectStorageProvider
	SecCtx          *pkgctx.SecurityContext
	ListCtx         context.Context
}

// ResolveRulesWithOptions loads rules in order:
//  1. If RulesFile is non-empty, use that file only.
//  2. Else if StorageProvider, SecCtx, and ListCtx are set and storage returns at least one enabled rule, use those.
//  3. Else if docs/process/_internal/process_hygiene_rules.yaml exists under ProjectRoot, use it.
//  4. Else use embedded defaults.
func ResolveRulesWithOptions(opts ResolveRulesOptions) ([]Rule, string, error) {
	if opts.RulesFile != emptyValue {
		rules, err := LoadRulesFile(opts.RulesFile)
		return rules, opts.RulesFile, err
	}
	if opts.StorageProvider != nil && opts.SecCtx != nil && opts.ListCtx != nil {
		rules, err := loadRulesFromStorage(opts.ListCtx, opts.StorageProvider, opts.SecCtx)
		if err != nil {
			return nil, emptyValue, err
		}
		if len(rules) > 0 {
			return rules, "storage:process_hygiene_rule", nil
		}
	}
	if opts.ProjectRoot != emptyValue {
		candidate := filepath.Join(opts.ProjectRoot, internalRulesRelative)
		if _, err := fileutil.Stat(candidate); err == nil {
			rules, err := LoadRulesFile(candidate)
			return rules, candidate, err
		}
	}
	rules, err := LoadEmbeddedDefaultRules()
	return rules, "embedded:default_rules.yaml", err
}
