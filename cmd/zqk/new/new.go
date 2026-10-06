package newcmd

import (
	"fmt"
	"github.com/zqk-os/zqk/pkg/quick"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/cmd/zqk/workflow"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scenario"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/swarm/pack"
	"github.com/zqk-os/zqk/pkg/telemetry"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const (
	emptyValue          = ""
	commandSpecFilePerm = paths.FilePerm644
)

func newRootIntro() string {
	return "Mint instances (`new object --title`) onto the draft plane; command DNA: `new command-spec`; YAML scaffolds: `" + paths.CLIUsage("object", "template") + " <kind>`. Bundles/object-specs/swarms still use .zqk/drafts/."
}

// NewNewCmd is the `zqk new` command tree. Root long/short help and subcommands are spec-driven;
// see .zqk/cli/specs/new/root_command.yaml and sibling *_command.yaml files;
// pkg/cli/bldr_cli_cmd_v1/new_*_command_builder.go (zqk system generate-command-builders --overwrite).
func NewNewCmd() *cobra.Command {
	root := bldr_cli_cmd_v1.NewNewRootCommandBuilder()
	// retire stale internal-create help until new_* builders regenerate from specs.
	root.Long = strings.ReplaceAll(root.Long,
		paths.CLIUsage("internal", "create")+" <kind>",
		paths.CLIUsage("object", "create")+" <kind> --internal",
	)
	root.Long = strings.ReplaceAll(root.Long,
		"(privileged kinds; same last-draft pointer)",
		"(elevated kinds; Enterprise license or zqk-admin)",
	)
	if root.Long != "" {
		root.Long = newRootIntro() + "\n\n" + root.Long
	} else {
		root.Long = newRootIntro()
	}
	root.PersistentPreRunE = runNewKindValidatePreRun
	root.AddCommand(newObjectCmd())
	root.AddCommand(newObjectSpecKindCmd())
	root.AddCommand(newCommandSpecCmd())
	root.AddCommand(newBundleCmd())
	root.AddCommand(newSwarmCmd())

	// First-class kind convenience commands: zqk new <kind> [--title "..."]
	root.AddCommand(newKindConvenienceCmd(objects.KindGoal))
	root.AddCommand(newKindConvenienceCmd(objects.KindVision, "vis"))
	root.AddCommand(newKindConvenienceCmd(objects.KindMission, "mis"))
	root.AddCommand(newKindConvenienceCmd(objects.KindRoadmap))
	root.AddCommand(newKindConvenienceCmd(objects.KindWorkstream, "ws"))
	root.AddCommand(newKindConvenienceCmd(objects.KindEpic))
	root.AddCommand(newKindConvenienceCmd(objects.KindPriorityPlan, "plan", "pri"))
	root.AddCommand(newKindConvenienceCmd(objects.KindRequirement, "req"))
	root.AddCommand(newKindConvenienceCmd(objects.KindCriteria, "crit"))
	root.AddCommand(newKindConvenienceCmd(objects.KindTestCase, "tc"))
	root.AddCommand(newKindConvenienceCmd(objects.KindBacklogItem, "bli"))
	root.AddCommand(newKindConvenienceCmd(objects.KindMilestone, "mil"))
	root.AddCommand(newKindConvenienceCmd(objects.KindTechnicalDebt, "td", "techdebt"))
	root.AddCommand(newKindConvenienceCmd(objects.KindPolicy))
	root.AddCommand(newKindConvenienceCmd(objects.KindQuestion, "que"))
	root.AddCommand(newKindConvenienceCmd(objects.KindDecision, "dec"))
	return root
}

func newKindConvenienceCmd(kind string, aliases ...string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     kind,
		Aliases: aliases,
		Short:   fmt.Sprintf("Mint a %s object onto the draft plane", kind),
	}
	cli.BindAsyncProgress(cmd, func(c *cobra.Command, args []string) error {
		return runNewObject(c, append([]string{kind}, args...))
	})
	cmd.Flags().StringP("title", "t", "", "Object title")
	cmd.Flags().StringP("description", "d", "", "Object description")
	cmd.Flags().StringP("content", "c", "", "Inline content/body for description")
	cmd.Flags().String("file", "", "File to read title/description from")
	cmd.Flags().Bool("promote", false, "Enqueue background object promote after mint")
	cmd.Flags().Bool("cas", false, "Directly materialize the object into CAS storage")
	cmd.Flags().Bool("skip-trace-pipeline", false, "Do not auto-run workflow gen-trace-pipeline")
	return cmd
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
	desc, _ := cmd.Flags().GetString("description")

	var body string
	if file != emptyValue || content != emptyValue {
		var err error
		body, title, err = readTitleAndBody(file, content, title)
		if err != nil {
			return err
		}
	}
	if body == emptyValue && desc != emptyValue {
		body = strings.TrimSpace(desc)
	}

	if kind == objects.KindCommandSpec {
		return errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations("command_spec is file-authored CLI DNA; use `zqk new command-spec <command-path> --short ... --description ...`"))
	}
	if title == emptyValue {
		return errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("title is required (pass --title, --file, or --content). For printable YAML only: zqk object template %s", kind)))
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
			return errfmt.Newf("minted %s but trace pipeline failed; promote will fail closed without traceability links", id).Wrap(pipeErr)
		}
	}

	emitJITGuidanceTip(cmd, kind)

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

func emitJITGuidanceTip(cmd *cobra.Command, kind string) {
	format := cli.GetFormat(cmd)
	if format == cli.FormatJSON || format == cli.FormatYAML || format == cli.FormatJSONRPC || format == cli.FormatStream {
		return
	}
	var tip string
	switch kind {
	case objects.KindGoal:
		tip = "💡 Shift-Left Tip: Goals represent timeless strategic compass themes (outside of sprint/time boundaries) and decompose into 3–5 requirements (child requirement.goal_refs). Goals do NOT declare criteria_refs directly."
	case objects.KindRequirement:
		tip = "💡 Shift-Left Tip: Requirements are timeless functional contracts (never bound to time or priority plans). They own criteria_refs (>= 3 satisfying three-fold proof: static invariant, operational proof, negative boundary) verified 1:1 by a test_case."
	case objects.KindMilestone:
		tip = "💡 Shift-Left Tip: Milestones introduce chronological and time constraints — acting as time-bounded subgoals across delivered reality that priority plans aim toward."
	case objects.KindTestCase:
		tip = "💡 Shift-Left Tip: Test cases verify the criteria bundle for 1 requirement (1:1 feature contract verification)."
	case objects.KindBacklogItem:
		tip = "💡 Shift-Left Tip: Backlog items are atomic units of effort satisfying 1–3 criteria. Decompose work across 2–5 BLIs per plan to avoid scope-stacking."
	case objects.KindPriorityPlan:
		tip = "💡 Shift-Left Tip: Priority plans bound exactly 1 cycle of work (sprint/kanban batch, 2–5 BLIs aiming toward a milestone). Plans scope-lock upon entering in_progress to prevent drift."
	case objects.KindEpic:
		tip = "💡 Shift-Left Tip: Epics group multiple priority plans under a unifying theme. Permissive by default (plans can be added in progress unless execution_locked=true)."
	case objects.KindTechnicalDebt:
		tip = "💡 Shift-Left Tip: Technical debt provides a tactical entry point into the non-functional behavior plane (code smells, test flakes, performance hotpaths, bug rollups). Rollups of tech debt items tell the anti-pattern story to institute lasting architectural and coding best practices."
	}
	if tip != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), color.CyanString(tip))
	}
}

// shouldAutoTracePipeline is the mint-time fail-closed for REQ/GOAL/MIL.
// Test roots skip so unit tests do not apply a full scenario bundle.
// TRACK: POL-AGENT-TPM-TRACE-PIPELINE-001
func shouldAutoTracePipeline(cmd *cobra.Command, kind string) bool {
	return shouldAutoTracePipelineConfigured(cmd, kind, zqkenv.IsInTest(), strings.TrimSpace(zqkenv.TestRoot().Get()) != emptyValue)
}

func shouldAutoTracePipelineConfigured(cmd *cobra.Command, kind string, inTest bool, hasTestRoot bool) bool {
	switch kind {
	case objects.KindRequirement:
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
	Schema      string            `yaml:"$schema"`
	Name        string            `yaml:"name"`
	Short       string            `yaml:"short"`
	Description string            `yaml:"description"`
	Aliases     []string          `yaml:"aliases,omitempty"`
	Args        *clipkg.ArgsSpec  `yaml:"args,omitempty"`
	Flags       []clipkg.FlagSpec `yaml:"flags,omitempty"`
	Help        *clipkg.HelpSpec  `yaml:"help,omitempty"`
	RunE        string            `yaml:"run_e,omitempty"`
	Async       bool              `yaml:"async,omitempty"`
	CommonFlags *bool             `yaml:"common_flags,omitempty"`
}

func newCommandSpecCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewNewCommandSpecCommandBuilder()
	ensureCommandSpecFlags(cmd)
	cmd.RunE = runNewCommandSpec
	return cmd
}

func ensureCommandSpecFlags(cmd *cobra.Command) {
	if cmd.Flags().Lookup("from-cmd") == nil {
		cmd.Flags().Bool("from-cmd", false, "Introspect registered Cobra command tree to extract flags, args, and usage")
	}
	if cmd.Flags().Lookup("aliases") == nil {
		cmd.Flags().String("aliases", "", "Comma-separated command aliases")
	}
	if cmd.Flags().Lookup("args-type") == nil {
		cmd.Flags().String("args-type", "", "Positional argument validation type: no_args, exact, minimum, maximum, range")
	}
	if cmd.Flags().Lookup("args-count") == nil {
		cmd.Flags().Int("args-count", 0, "Argument count for exact args")
	}
	if cmd.Flags().Lookup("args-min") == nil {
		cmd.Flags().Int("args-min", 0, "Minimum argument count")
	}
	if cmd.Flags().Lookup("args-max") == nil {
		cmd.Flags().Int("args-max", 0, "Maximum argument count")
	}
	if cmd.Flags().Lookup("flag") == nil {
		cmd.Flags().StringArray("flag", nil, "Custom flag definition (name:type:default:description[:shorthand])")
	}
	if cmd.Flags().Lookup("example") == nil {
		cmd.Flags().StringArray("example", nil, "Help example in comment:command format")
	}
	if cmd.Flags().Lookup("run-e") == nil {
		cmd.Flags().String("run-e", "", "RunE execution function name")
	}
	if cmd.Flags().Lookup("common-flags") == nil {
		cmd.Flags().Bool("common-flags", true, "Include standard common flags in the spec")
	}
	if cmd.Flags().Lookup("async") == nil {
		cmd.Flags().Bool("async", false, "Wrap RunE with BindAsyncProgress")
	}
}

func findCobraCommand(root *cobra.Command, segments []string) *cobra.Command {
	if root == nil || len(segments) == 0 {
		return nil
	}
	curr := root
	for _, seg := range segments {
		var found *cobra.Command
		for _, child := range curr.Commands() {
			if child.Name() == seg || child.HasAlias(seg) {
				found = child
				break
			}
		}
		if found == nil {
			return nil
		}
		curr = found
	}
	return curr
}

func parsePathSegments(commandPath string) []string {
	return strings.FieldsFunc(strings.TrimSpace(commandPath), func(r rune) bool {
		return r == '/' || r == '\\' || r == ' ' || r == '\t'
	})
}

func toPascalCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-' || r == '_' || r == ' '
	})
	var b strings.Builder
	for _, p := range parts {
		if len(p) > 0 {
			b.WriteString(strings.ToUpper(p[:1]) + strings.ToLower(p[1:]))
		}
	}
	return b.String()
}

func runNewCommandSpec(cmd *cobra.Command, args []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	specsDir := resolveCommandSpecsDir(projectRoot)

	outputPath, defaultUse, err := commandSpecOutputPath(specsDir, args[0])
	if err != nil {
		return err
	}
	force, _ := cmd.Flags().GetBool("force")
	if !force {
		if _, statErr := fileutil.Stat(outputPath); statErr == nil {
			return errfmt.Errorf("command spec already exists: %s (pass --force to replace)", outputPath)
		} else if !fileutil.IsNotExist(statErr) {
			return errfmt.Newf("inspect command spec path").Wrap(statErr)
		}
	}

	segments := parsePathSegments(args[0])
	fromCmd, _ := cmd.Flags().GetBool("from-cmd")
	foundCmd := findCobraCommand(cmd.Root(), segments)

	if fromCmd && foundCmd == nil {
		return errfmt.Errorf("command %q not found in registered CLI command tree", args[0])
	}

	use, _ := cmd.Flags().GetString("use")
	short, _ := cmd.Flags().GetString("short")
	description, _ := cmd.Flags().GetString("description")
	aliasesStr, _ := cmd.Flags().GetString("aliases")
	argsType, _ := cmd.Flags().GetString("args-type")
	argsCount, _ := cmd.Flags().GetInt("args-count")
	argsMin, _ := cmd.Flags().GetInt("args-min")
	argsMax, _ := cmd.Flags().GetInt("args-max")
	flagDefs, _ := cmd.Flags().GetStringArray("flag")
	exampleDefs, _ := cmd.Flags().GetStringArray("example")
	runEName, _ := cmd.Flags().GetString("run-e")
	commonFlagsVal, _ := cmd.Flags().GetBool("common-flags")
	asyncVal, _ := cmd.Flags().GetBool("async")

	spec := commandSpecFile{}

	introspect := fromCmd || (foundCmd != nil && (short == "" || description == ""))
	if introspect && foundCmd != nil {
		if use == "" {
			use = foundCmd.Use
		}
		if short == "" {
			short = foundCmd.Short
		}
		if description == "" {
			if foundCmd.Long != "" {
				description = foundCmd.Long
			} else {
				description = foundCmd.Short
			}
		}
		if aliasesStr == "" && len(foundCmd.Aliases) > 0 {
			spec.Aliases = foundCmd.Aliases
		}
	}

	if use = strings.TrimSpace(use); use == emptyValue {
		use = defaultUse
	}
	if strings.TrimSpace(short) == emptyValue {
		return errfmt.Errorf("short is required (pass --short or --from-cmd)")
	}
	if strings.TrimSpace(description) == emptyValue {
		description = short
	}

	spec.Name = use
	spec.Short = strings.TrimSpace(short)
	spec.Description = strings.TrimSpace(description)

	if aliasesStr != "" {
		for _, a := range strings.Split(aliasesStr, ",") {
			if a = strings.TrimSpace(a); a != "" {
				spec.Aliases = append(spec.Aliases, a)
			}
		}
	}

	// Positional arguments
	if argsType != "" {
		spec.Args = &clipkg.ArgsSpec{
			Type: argsType,
		}
		if argsCount > 0 {
			spec.Args.Count = &argsCount
		}
		if argsMin > 0 {
			spec.Args.Min = &argsMin
		}
		if argsMax > 0 {
			spec.Args.Max = &argsMax
		}
	} else if introspect && foundCmd != nil {
		if strings.Contains(use, "<") {
			count := strings.Count(use, "<")
			spec.Args = &clipkg.ArgsSpec{
				Type:  "exact",
				Count: &count,
			}
		} else if strings.Contains(use, "[") && strings.Contains(use, "...") {
			zero := 0
			spec.Args = &clipkg.ArgsSpec{
				Type: "minimum",
				Min:  &zero,
			}
		} else {
			spec.Args = &clipkg.ArgsSpec{
				Type: "no_args",
			}
		}
	} else {
		spec.Args = &clipkg.ArgsSpec{
			Type: "no_args",
		}
	}

	// Flags
	commonFlagsMap := map[string]bool{
		"format": true, "output": true, "verbose": true, "quiet": true,
		"timeout": true, "columns": true, "context": true, "help": true,
		"ignore-scheduler-down": true, "profile": true,
	}

	if len(flagDefs) > 0 {
		for _, fDef := range flagDefs {
			parts := strings.SplitN(fDef, ":", 5)
			if len(parts) >= 2 {
				fs := clipkg.FlagSpec{
					Name: strings.TrimSpace(parts[0]),
					Type: strings.TrimSpace(parts[1]),
				}
				if len(parts) >= 3 && strings.TrimSpace(parts[2]) != "" {
					fs.Default = strings.TrimSpace(parts[2])
				}
				if len(parts) >= 4 {
					fs.Description = strings.TrimSpace(parts[3])
				}
				if len(parts) >= 5 {
					fs.Shorthand = strings.TrimSpace(parts[4])
				}
				spec.Flags = append(spec.Flags, fs)
			}
		}
	} else if introspect && foundCmd != nil {
		foundCmd.Flags().VisitAll(func(f *pflag.Flag) {
			if commonFlagsMap[f.Name] {
				return
			}
			fType := "string"
			switch f.Value.Type() {
			case "bool":
				fType = "bool"
			case "int", "int32", "int64":
				fType = "int"
			case "stringSlice", "stringArray":
				fType = "string_array"
			case "duration":
				fType = "duration"
			}
			var defVal any = f.DefValue
			if fType == "bool" {
				defVal = (f.DefValue == "true")
			} else if fType == "int" {
				if iv, err := strconv.Atoi(f.DefValue); err == nil {
					defVal = iv
				}
			} else if defVal == "" {
				defVal = nil
			}
			spec.Flags = append(spec.Flags, clipkg.FlagSpec{
				Name:        f.Name,
				Shorthand:   f.Shorthand,
				Type:        fType,
				Default:     defVal,
				Description: f.Usage,
			})
		})
	}

	// Help and Examples
	var examples []clipkg.HelpExampleSpec
	if len(exampleDefs) > 0 {
		for _, exDef := range exampleDefs {
			parts := strings.SplitN(exDef, ":", 2)
			if len(parts) == 2 {
				examples = append(examples, clipkg.HelpExampleSpec{
					Comment: strings.TrimSpace(parts[0]),
					Command: strings.TrimSpace(parts[1]),
				})
			} else {
				examples = append(examples, clipkg.HelpExampleSpec{
					Comment: "Run " + strings.Join(segments, " "),
					Command: strings.TrimSpace(parts[0]),
				})
			}
		}
	} else if introspect && foundCmd != nil && foundCmd.Example != "" {
		lines := strings.Split(foundCmd.Example, "\n")
		var currentComment string
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}
			if strings.HasPrefix(l, "#") || strings.HasPrefix(l, "//") {
				currentComment = strings.TrimSpace(strings.TrimLeft(l, "#/ "))
			} else {
				if currentComment == "" {
					currentComment = "Example invocation"
				}
				examples = append(examples, clipkg.HelpExampleSpec{
					Comment: currentComment,
					Command: l,
				})
				currentComment = ""
			}
		}
	}
	if len(examples) == 0 {
		cmdInvocation := "%s " + strings.Join(segments, " ")
		examples = append(examples, clipkg.HelpExampleSpec{
			Comment: "Run " + strings.Join(segments, " "),
			Command: cmdInvocation,
		})
	}
	spec.Help = &clipkg.HelpSpec{
		Examples: examples,
	}

	// RunE
	if runEName != "" {
		spec.RunE = runEName
	} else {
		var parts []string
		for _, seg := range segments {
			parts = append(parts, toPascalCase(seg))
		}
		spec.RunE = "run" + strings.Join(parts, "")
	}

	// CommonFlags
	spec.CommonFlags = &commonFlagsVal
	spec.Async = asyncVal

	if err := paths.EnsureDir(filepath.Dir(outputPath), paths.DirPerm755); err != nil {
		return errfmt.Newf("create command spec directory").Wrap(err)
	}

	schemaPath := filepath.Join(specsDir, "schemas", "command_spec.schema.json")
	if _, statErr := fileutil.Stat(schemaPath); statErr != nil {
		altSchemaPath := filepath.Join(specsDir, "_schemas", "command_spec.schema.json")
		if _, altErr := fileutil.Stat(altSchemaPath); altErr == nil {
			schemaPath = altSchemaPath
		}
	}
	schemaRef, err := filepath.Rel(filepath.Dir(outputPath), schemaPath)
	if err != nil {
		return errfmt.Newf("resolve command spec schema").Wrap(err)
	}
	spec.Schema = filepath.ToSlash(schemaRef)

	data, err := yaml.Marshal(spec)
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
		"flags_count":        len(spec.Flags),
		"examples_count":     len(examples),
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
	yamlStr, err := objects.GenerateObjectSpecKindDraft(nil, ontology, extendsName)
	if err != nil {
		return errfmt.Newf("object kind spec draft").Wrap(err)
	}
	// No last-draft pointer: this file is copied into object_specs/, not consumed by object create.
	return writeDraft(cmd, out, ontology+"-object-spec", []byte(yamlStr), "", "")
}

func newBundleCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewNewBundleCommandBuilder()
	cmd.Aliases = []string{"scenario"}
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

func newSwarmCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewNewSwarmCommandBuilder()
	cli.BindAsyncProgress(cmd, runNewSwarm)
	return cmd
}

func runNewSwarm(cmd *cobra.Command, args []string) (err error) {
	_, endSpan := telemetry.GlobalManager().StartSpan(cmd.Context(), "zqk_new_swarm", map[string]string{"args": strings.Join(args, " ")})
	defer func() { endSpan(err) }()

	name, _ := cmd.Flags().GetString("name")
	version, _ := cmd.Flags().GetString("version")
	desc, _ := cmd.Flags().GetString("description")
	author, _ := cmd.Flags().GetString("author")
	license, _ := cmd.Flags().GetString("license")
	cellType, _ := cmd.Flags().GetString("cell-type")
	goalMetric, _ := cmd.Flags().GetString("goal-metric")
	goalTarget, _ := cmd.Flags().GetString("goal-target")
	goalDesc, _ := cmd.Flags().GetString("goal-description")
	out, _ := cmd.Flags().GetString("output")

	data, err := pack.DraftSwarmManifestYAML(pack.SwarmDraftOptions{
		Name:            name,
		Version:         version,
		Description:     desc,
		Author:          author,
		License:         license,
		CellType:        cellType,
		GoalMetric:      goalMetric,
		GoalTarget:      goalTarget,
		GoalDescription: goalDesc,
	})
	if err != nil {
		return err
	}
	base := "swarm"
	if strings.TrimSpace(name) != emptyValue {
		base = name
	}
	return writeDraft(cmd, out, base, data, cli.LastDraftScopeSwarm, "swarm_package")
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
