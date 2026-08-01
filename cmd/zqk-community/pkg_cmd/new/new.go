package newcmd

import (
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/cliexamples"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/scenario"
	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/telemetry"
)

const emptyValue = ""

// NewNewCmd is the `zqk new` command tree. Root long/short help and subcommands are spec-driven;
// see docs/process/command_specs/new/root_command.yaml and sibling *_command.yaml files;
// pkg/cli/bldr_cli_cmd_v1/new_*_command_builder.go (zqk system generate-command-builders --overwrite).
func NewNewCmd() *cobra.Command {
	root := bldr_cli_cmd_v1.NewNewRootCommandBuilder()
	root.PersistentPreRunE = runNewKindValidatePreRun
	root.AddCommand(newObjectCmd())
	root.AddCommand(newInternalCmd())
	root.AddCommand(newObjectSpecKindCmd())
	root.AddCommand(newBundleCmd())
	return root
}

func runNewKindValidatePreRun(cmd *cobra.Command, args []string) error {
	if cmd == nil || cmd.Annotations == nil {
		return nil
	}
	if strings.TrimSpace(cmd.Annotations[cli.AnnotationKeyInternalKindValidate]) != "" {
		return cli.ValidateAnnotatedKind(cli.KindAnnotKeysInternal, cmd, args, runNewInternalListOptionalHook)
	}
	if strings.TrimSpace(cmd.Annotations[cli.AnnotationKeyObjectKindValidate]) != "" {
		return cli.ValidateAnnotatedKind(cli.KindAnnotKeysObject, cmd, args, nil)
	}
	return nil
}

// runNewInternalListOptionalHook mirrors pkg/zqkcli internalListOptionalHook so lifecycle/object_spec
// validate without object-spec-index membership checks (same as internal list).
func runNewInternalListOptionalHook(raw string) (canonical string, skipResolve bool) {
	raw = strings.TrimSpace(raw)
	if raw == objects.KindLifecycle || raw == objects.KindObjectSpec {
		return raw, true
	}
	return "", false
}

func newObjectCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewNewObjectCommandBuilder()
	cli.BindAsyncProgress(cmd, runNewObject)
	cli.EnsureCmdAnnotations(cmd)
	cmd.Annotations[cli.AnnotationKeyObjectKindValidate] = cli.KindValidatePositional0
	return cmd
}

func runNewObject(cmd *cobra.Command, args []string) (err error) {
	_, endSpan := telemetry.GlobalManager().StartSpan(cmd.Context(), "zqk_new_object", map[string]string{"args": strings.Join(args, " ")})
	defer func() { endSpan(err) }()

	kind, ok := cli.KindCanonicalFromPRERun(cli.KindAnnotKeysObject, cmd)
	if !ok {
		var err error
		kind, err = objects.ResolveAndValidateKindForProject(cli.ResolveProjectRoot("."), args[0])
		if err != nil {
			return errfmt.Newf("invalid object kind").Wrap(err)
		}
	}
	out, _ := cmd.Flags().GetString("output")
	gen, err := cliexamples.New()
	if err != nil {
		return errfmt.Newf("cli examples").Wrap(err)
	}
	yamlStr, err := gen.GenerateYAMLExample(kind, "")
	if err != nil {
		return errfmt.Newf("template for kind %q", kind).Wrap(err)
	}
	header := "# Draft object (zqk new object) — edit placeholders, then:\n#   zqk object create " + kind + "\n\n"
	content := []byte(header + yamlStr)
	return writeDraft(cmd, out, kind, content, cli.LastDraftScopeObject, kind)
}

func newInternalCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewNewInternalCommandBuilder()
	cli.BindAsyncProgress(cmd, runNewInternal)
	cli.EnsureCmdAnnotations(cmd)
	cmd.Annotations[cli.AnnotationKeyInternalKindValidate] = cli.KindValidateInternalListOptional
	return cmd
}

func runNewInternal(cmd *cobra.Command, args []string) (err error) {
	_, endSpan := telemetry.GlobalManager().StartSpan(cmd.Context(), "zqk_new_internal", map[string]string{"args": strings.Join(args, " ")})
	defer func() { endSpan(err) }()

	kind, ok := cli.KindCanonicalFromPRERun(cli.KindAnnotKeysInternal, cmd)
	if !ok {
		var err error
		kind, err = objects.ResolveAndValidateKindForProject(cli.ResolveProjectRoot("."), args[0])
		if err != nil {
			return errfmt.Newf("invalid object kind").Wrap(err)
		}
	}
	out, _ := cmd.Flags().GetString("output")
	gen, err := cliexamples.New()
	if err != nil {
		return errfmt.Newf("cli examples").Wrap(err)
	}
	yamlStr, err := gen.GenerateYAMLExample(kind, "")
	if err != nil {
		return errfmt.Newf("template for kind %q", kind).Wrap(err)
	}
	header := "# Draft internal object (zqk new internal) — edit placeholders, then:\n#   zqk internal create " + kind + "\n\n"
	content := []byte(header + yamlStr)
	return writeDraft(cmd, out, "internal-"+kind, content, cli.LastDraftScopeInternal, kind)
}

func newObjectSpecKindCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewNewObjectSpecKindCommandBuilder()
	cli.BindAsyncProgress(cmd, runNewObjectSpecKind)
	return cmd
}

func runNewObjectSpecKind(cmd *cobra.Command, args []string) (err error) {
	_, endSpan := telemetry.GlobalManager().StartSpan(cmd.Context(), "zqk_new_object_spec_kind", map[string]string{"args": strings.Join(args, " ")})
	defer func() { endSpan(err) }()

	ontology := strings.TrimSpace(args[0])
	extendsName, _ := cmd.Flags().GetString("extends")
	out, _ := cmd.Flags().GetString("output")
	gen, err := cliexamples.New()
	if err != nil {
		return errfmt.Newf("cli examples").Wrap(err)
	}
	yamlStr, err := gen.GenerateObjectSpecKindDraft(ontology, extendsName)
	if err != nil {
		return errfmt.Newf("object kind spec draft").Wrap(err)
	}
	// No last-draft pointer: this file is copied into object_specs/, not consumed by object create.
	return writeDraft(cmd, out, ontology+"-object-spec", []byte(yamlStr), "", "")
}

func newBundleCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewNewBundleCommandBuilder()
	cli.BindAsyncProgress(cmd, runNewBundle)
	return cmd
}

func runNewBundle(cmd *cobra.Command, args []string) (err error) {
	_, endSpan := telemetry.GlobalManager().StartSpan(cmd.Context(), "zqk_new_bundle", map[string]string{"args": strings.Join(args, " ")})
	defer func() { endSpan(err) }()

	name, _ := cmd.Flags().GetString("name")
	desc, _ := cmd.Flags().GetString("description")
	out, _ := cmd.Flags().GetString("output")
	data, err := scenario.DraftBundleYAML(name, desc)
	if err != nil {
		return err
	}
	base := "bundle"
	if strings.TrimSpace(name) != emptyValue {
		base = name
	}
	return writeDraft(cmd, out, base, data, cli.LastDraftScopeBundle, "scenario_bundle")
}
