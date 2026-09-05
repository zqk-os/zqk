package cli

import (
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// DefaultCommonFlagNamesForHelp are the logical names matched by internal/cli.AddCommonFlags and
// AddCommonFlagsExcluding when no flags are excluded.
var DefaultCommonFlagNamesForHelp = []string{"format", "output", "verbose", "quiet", "timeout", "columns"}

// LoadCommandSpecYAML loads a CommandSpec from a YAML file path.
func LoadCommandSpecYAML(path string) (*CommandSpec, error) {
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return nil, errfmt.Newf("read command spec YAML").Wrap(err)
	}
	var spec CommandSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, errfmt.Newf("parse command spec YAML").Wrap(err)
	}
	return &spec, nil
}

// ExpectedLocalFlagNamesFromCommandSpec returns the sorted set of flag names declared by the spec:
// explicit flags, optional list/count harness expansions, and common flags respecting help.exclude_flags.
func ExpectedLocalFlagNamesFromCommandSpec(spec *CommandSpec) []string {
	if spec == nil {
		return nil
	}
	set := make(map[string]struct{})
	for _, f := range spec.Flags {
		set[f.Name] = struct{}{}
	}
	if spec.QueryFlags {
		for _, n := range QueryFlagNames() {
			set[n] = struct{}{}
		}
	}
	if spec.ListHarnessFlags {
		for _, n := range ListHarnessFlagNames() {
			set[n] = struct{}{}
		}
	}
	if spec.CountHarnessFlags {
		for _, n := range CountHarnessFlagNames() {
			set[n] = struct{}{}
		}
	}
	if spec.FieldsHarnessFlags {
		for _, n := range FieldsHarnessFlagNames(spec.FieldsIncludeListKinds) {
			set[n] = struct{}{}
		}
	}
	excluded := excludedCommonFlagSet(spec.Help)
	if spec.CommonFlags {
		for _, n := range DefaultCommonFlagNamesForHelp {
			if !excluded[n] {
				set[n] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func excludedCommonFlagSet(help *HelpSpec) map[string]bool {
	ex := make(map[string]bool)
	if help == nil {
		return ex
	}
	for _, x := range help.ExcludeFlags {
		ex[x] = true
	}
	return ex
}

// BulkSubcommandSpec returns the merged CommandSpec for a nested subcommand under internal bulk:
// nested flags + parent's common_flags / help.exclude_flags inheritance when child omits them.
func BulkSubcommandSpec(parent *CommandSpec, subcommandName string) (*CommandSpec, bool) {
	if parent == nil {
		return nil, false
	}
	for _, sub := range parent.Subcommands {
		if sub.Name != subcommandName || sub.Spec == nil {
			continue
		}
		child := *sub.Spec
		if child.Help == nil {
			child.Help = parent.Help
		}
		if !child.CommonFlags {
			child.CommonFlags = parent.CommonFlags
		}
		return &child, true
	}
	return nil, false
}
