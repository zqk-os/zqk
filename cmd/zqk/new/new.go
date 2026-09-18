package newcmd

import (
	"path/filepath"
	"strings"
	"github.com/zqk-os/zqk/pkg/quick"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/cmd/zqk/workflow"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliexamples"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scenario"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/telemetry"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const (
	emptyValue          = ""
	commandSpecFilePerm = 0o644
	newRootIntro        = "Mint instances (`new object --title`) onto the draft plane; command DNA: `new command-spec`; YAML scaffolds: zqk object template <kind>. Bundles/object-specs still use .zqk/drafts/."
)

// NewNewCmd is the `zqk new` command tree. Root long/short help and subcommands are spec-driven;
// see .zqk/cli/specs/new/root_command.yaml and sibling *_command.yaml files;
// pkg/cli/bldr_cli_cmd_v1/new_*_command_builder.go (zqk system generate-command-builders --overwrite).
func NewNewCmd() *cobra.Command {
	root := bldr_cli_cmd_v1.NewNewRootCommandBuilder()
	// TRACK: BLI-1785930106857898000-94b9a5bc — retire stale internal-create help until new_* builders regenerate from specs.
	root.Long = strings.ReplaceAll(root.Long,
		"zqk internal create <kind>",
		"zqk object create <kind> --internal",
	)
	root.Long = strings.ReplaceAll(root.Long,
		"(privileged kinds; same last-draft pointer)",
		"(elevated kinds; Enterprise license or zqk-admin)",
	)
	if root.Long != "" {
		root.Long = newRootIntro + "\n\n" + root.Long
	} else {
		root.Long = newRootIntro
	}
	root.PersistentPreRunE = runNewKindValidatePreRun
	root.AddCommand(newObjectCmd())
	root.AddCommand(newObjectSpecKindCmd())
	root.AddCommand(newCommandSpecCmd())
	root.AddCommand(newBundleCmd())
	return root
}

func runNewKindValidatePreRun(cmd *cobra.Command, args []string) error {
	if cmd == nil || cmd.Annotations == nil {
		return nil
	}
	if strings.TrimSpace(cmd.Annotations[cli.AnnotationKeyObjectKindValidate]) != "" {
		return cli.ValidateAnnotatedKind(cli.KindAnnotKeysObject, cmd, args, nil)
	}
	return nil
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
	title, _ := cmd.Flags().GetString("title")
	title = strings.TrimSpace(title)
	file, _ := cmd.Flags().GetString("file")
	content, _ := cmd.Flags().GetString("content")

	var body string
	if file != emptyValue || content != emptyValue {
		var err error
		body, title, err = readTitleAndBody(file, content, title)
		if err != nil {
			return err
		}
	}

	if kind == objects.KindCommandSpec {
		return errfmt.Errorf("command_spec is file-authored CLI DNA; use `zqk new command-spec <command-path> --short ... --description ...`")
	}
	if title == emptyValue {
		return errfmt.Errorf("title is required (pass --title, --file, or --content). For printable YAML only: zqk object template %s", kind)
	}

	projectRoot := cli.ResolveProjectRoot(".")
	objData := map[string]any{
		objects.FieldKeyKind:  kind,
		objects.FieldKeyTitle: title,
	}
	if body != emptyValue {
		objData[objects.FieldKeyDescription] = body
	}
	if err := object.RunCreateWithData(cmd, kind, objData); err != nil {
		return err
	}
	id, _ := objData[objects.FieldKeyID].(string)
	if id == emptyValue {
		return errfmt.Errorf("mint succeeded but object id is empty")
	}

	if shouldAutoTracePipeline(cmd, kind) {
		if pipeErr := workflow.ApplyGeneratedTracePipelineFromCmd(cmd, id); pipeErr != nil {
			return errfmt.Newf("minted %s but trace pipeline failed; promote will fail closed without criteria_refs", id).Wrap(pipeErr)
		}
	}

	wantPromote, _ := cmd.Flags().GetBool("promote")
	if !wantPromote {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Info("minted onto draft plane; promote not enqueued (pass --promote to opt in)").
			ObjectID(id).
			Kind(kind).
			Log()
		return nil
	}
	jobID, enqErr := enqueueMintPromoteJob(projectRoot, id)
	if enqErr != nil {
		return errfmt.Newf("minted %s on draft plane but failed to enqueue background promote", id).Wrap(enqErr)
	}
	logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
		Info("minted onto draft plane; background promote enqueued").
		ObjectID(id).
		Kind(kind).
		JobID(jobID).
		Log()
	return nil
}

// shouldAutoTracePipeline is the mint-time fail-closed for REQ/GOAL/MIL.
// Test roots skip so unit tests do not apply a full scenario bundle.
// TRACK: POL-AGENT-TPM-TRACE-PIPELINE-001
func shouldAutoTracePipeline(cmd *cobra.Command, kind string) bool {
	return shouldAutoTracePipelineConfigured(cmd, kind, zqkenv.IsInTest(), strings.TrimSpace(zqkenv.TestRoot().Get()) != emptyValue)
}

func shouldAutoTracePipelineConfigured(cmd *cobra.Command, kind string, inTest bool, hasTestRoot bool) bool {
	switch kind {
	case objects.KindRequirement, objects.KindGoal, objects.KindMilestone:
	default:
		return false
	}
	if inTest || hasTestRoot {
		return false
	}
	if cmd != nil && cmd.Flags() != nil {
		skip, _ := cmd.Flags().GetBool("skip-trace-pipeline")
		if skip {
			return false
		}
	}
	return true
}

type commandSpecFile struct {
	Schema      string `yaml:"$schema"`
	Name        string `yaml:"name"`
	Short       string `yaml:"short"`
	Description string `yaml:"description"`
}

func newCommandSpecCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewNewCommandSpecCommandBuilder()
	cmd.RunE = runNewCommandSpec
	return cmd
}

func runNewCommandSpec(cmd *cobra.Command, args []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	specsDir := resolveCommandSpecsDir(projectRoot)

	outputPath, defaultUse, err := commandSpecOutputPath(specsDir, args[0])
	if err != nil {
		return err
	}
	use, _ := cmd.Flags().GetString("use")
	if use = strings.TrimSpace(use); use == emptyValue {
		use = defaultUse
	}
	short, _ := cmd.Flags().GetString("short")
	description, _ := cmd.Flags().GetString("description")
	force, _ := cmd.Flags().GetBool("force")
	if strings.TrimSpace(short) == emptyValue {
		return errfmt.Errorf("short is required (pass --short)")
	}
	if strings.TrimSpace(description) == emptyValue {
		return errfmt.Errorf("description is required (pass --description)")
	}

	if !force {
		if _, statErr := fileutil.Stat(outputPath); statErr == nil {
			return errfmt.Errorf("command spec already exists: %s (pass --force to replace)", outputPath)
		} else if !fileutil.IsNotExist(statErr) {
			return errfmt.Newf("inspect command spec path").Wrap(statErr)
		}
	}
	if err := paths.EnsureDir(filepath.Dir(outputPath), paths.DirPerm755); err != nil {
		return errfmt.Newf("create command spec directory").Wrap(err)
	}
	schemaPath := filepath.Join(specsDir, "schemas", "command_spec.schema.json")
	schemaRef, err := filepath.Rel(filepath.Dir(outputPath), schemaPath)
	if err != nil {
		return errfmt.Newf("resolve command spec schema").Wrap(err)
	}
	data, err := yaml.Marshal(commandSpecFile{
		Schema:      filepath.ToSlash(schemaRef),
		Name:        use,
		Short:       strings.TrimSpace(short),
		Description: strings.TrimSpace(description),
	})
	if err != nil {
		return errfmt.Newf("marshal command spec").Wrap(err)
	}
	if err := fileutil.WriteFile(outputPath, data, commandSpecFilePerm); err != nil {
		return errfmt.Newf("write command spec").Wrap(err)
	}
	return cli.FormatOutput(cmd, map[string]any{
		objects.FieldKeyPath: outputPath,
		"command_path":       args[0],
		objects.FieldKeyUse:  use,
	})
}

func resolveCommandSpecsDir(projectRoot string) string {
	configPath := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir, paths.PathsConfigFile)
	config, _ := validation.LoadPathsConfig(configPath)
	configuredPath := strings.TrimSpace(config.Paths[paths.PathKeyCommandSpecs])
	if configuredPath == emptyValue {
		configuredPath = paths.CLICommandSpecsDir
	}
	if filepath.IsAbs(configuredPath) {
		return configuredPath
	}
	return filepath.Join(projectRoot, configuredPath)
}

func commandSpecOutputPath(specsDir, commandPath string) (string, string, error) {
	segments := strings.FieldsFunc(strings.TrimSpace(commandPath), func(r rune) bool {
		return r == '/' || r == '\\' || r == ' ' || r == '\t'
	})
	if len(segments) == 0 {
		return emptyValue, emptyValue, errfmt.Errorf("command path is required")
	}
	diskSegments := make([]string, len(segments))
	for i, segment := range segments {
		if segment == "." || segment == ".." || !validCommandPathSegment(segment) {
			return emptyValue, emptyValue, errfmt.Errorf("invalid command path segment %q", segment)
		}
		diskSegments[i] = strings.ReplaceAll(segment, "-", "_")
	}
	filename := diskSegments[len(diskSegments)-1] + "_command.yaml"
	parent := append([]string{specsDir}, diskSegments[:len(diskSegments)-1]...)
	return filepath.Join(filepath.Join(parent...), filename), segments[len(segments)-1], nil
}

func validCommandPathSegment(segment string) bool {
	for i, r := range segment {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || ((r == '-' || r == '_') && i > 0) {
			continue
		}
		return false
	}
	return segment != emptyValue
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

func readTitleAndBody(filePath, content, titleOverride string) (body, title string, err error) {
	if filePath != emptyValue {
		data, err := fileutil.ReadFile(filePath)
		if err != nil {
			return "", "", errfmt.Newf("read file").Wrap(err)
		}
		parsed := quick.ParseFileContent(data)
		title = parsed.Title
		body = parsed.Body
	} else if content != emptyValue {
		parsed := quick.ParseMarkdownOrText(content)
		title = parsed.Title
		body = parsed.Body
	}
	if titleOverride != emptyValue {
		title = titleOverride
	}
	return body, title, nil
}

