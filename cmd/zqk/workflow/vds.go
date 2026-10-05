package workflow

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/vds"
)

// NewVDSCmd returns the workflow vds group.
func NewVDSCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewVdsCommandBuilder()
	cmd.AddCommand(NewVDSChecklistCmd())
	cmd.AddCommand(NewVDSEvaluateCmd())
	cmd.AddCommand(NewVDSProjectCmd())
	cmd.AddCommand(NewVDSInitCmd())
	return cmd
}

// NewVDSChecklistCmd returns workflow vds checklist.
func NewVDSChecklistCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewWorkflowVdsChecklistCommandBuilder()
	cli.BindAsyncProgress(cmd, runVDSChecklist)
	return cmd
}

// NewVDSEvaluateCmd returns workflow vds evaluate.
func NewVDSEvaluateCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewWorkflowVdsEvaluateCommandBuilder()
	cli.BindAsyncProgress(cmd, runVDSEvaluate)
	cli.RequireStorage(cmd, false) // storage optional — object predicates skip if absent
	return cmd
}

// NewVDSProjectCmd returns workflow vds project.
func NewVDSProjectCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewWorkflowVdsProjectCommandBuilder()
	cli.BindAsyncProgress(cmd, runVDSProject)
	cli.RequireStorage(cmd, false)
	return cmd
}

// NewVDSInitCmd returns workflow vds init.
func NewVDSInitCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewWorkflowVdsInitCommandBuilder()
	cli.BindAsyncProgress(cmd, runVDSInit)
	return cmd
}

func resolveVDSProjectRoot(cmd *cobra.Command) (string, error) {
	ctx := cli.GetContext(cmd)
	root := ""
	if ctx != nil {
		root = ctx.ProjectRoot
	}
	if root == "" {
		root = cli.ResolveProjectRoot(".")
	}
	if root == "" {
		return "", errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations("project root not found; run from a project directory or zqk use"))
	}
	return root, nil
}

type vdsExecutionContext struct {
	root  string
	spine *vds.SpineProfile
	cust  *vds.Customization
}

func resolveVDSExecutionContext(cmd *cobra.Command, flags *clipkg.FlagBag) (*vdsExecutionContext, error) {
	root, err := resolveVDSProjectRoot(cmd)
	if err != nil {
		return nil, err
	}
	spineRel := flags.String(cmd, "spine")
	custRel := flags.String(cmd, "customization")
	if err := flags.Err(); err != nil {
		return nil, err
	}
	spine, cust, err := vds.ResolveProfiles(root, spineRel, custRel)
	if err != nil {
		return nil, err
	}
	return &vdsExecutionContext{
		root:  root,
		spine: spine,
		cust:  cust,
	}, nil
}

func runVDSChecklist(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		var flags clipkg.FlagBag
		execCtx, err := resolveVDSExecutionContext(cmd, &flags)
		if err != nil {
			return err
		}
		opCtx := context.Background() // Background: request-or-shutdown derived
		if c := cmd.Context(); c != nil {
			opCtx = c
		}
		gls := vds.ResolveGlossary(opCtx, execCtx.cust, vdsTitleLookup(proc))
		rep := vds.BuildChecklist(execCtx.spine, execCtx.cust, gls)
		return emitVDS(cmd, proc, rep, rep.AgentBrief)
	})(cmd, args)
}

func runVDSEvaluate(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		var flags clipkg.FlagBag
		fileFlag := flags.String(cmd, "file")
		runCmds := flags.Bool(cmd, "run-commands")
		persist := flags.Bool(cmd, "persist")
		applyVerify := flags.Bool(cmd, "apply-verify")
		execCtx, err := resolveVDSExecutionContext(cmd, &flags)
		if err != nil {
			return err
		}
		chunksPath := vds.ResolveChunksPath(execCtx.root, fileFlag)
		chunks, err := vds.LoadChunks(chunksPath)
		if err != nil {
			return err
		}

		opt := vds.EvalOptions{
			ProjectRoot: execCtx.root,
			RunCommands: runCmds,
			TitleLookup: vdsTitleLookup(proc),
		}
		if sp := proc.Storage(); sp != nil {
			sec := proc.SecurityContext()
			opt.Lookup = func(ctx context.Context, id string) (map[string]any, error) {
				return sp.Read(ctx, sec, id)
			}
		}

		opCtx := context.Background() // Background: request-or-shutdown derived
		if c := cmd.Context(); c != nil {
			opCtx = c
		}
		rep := vds.Evaluate(opCtx, chunks, execCtx.spine, execCtx.cust, opt)

		if persist {
			if err := persistVDSReport(execCtx.root, rep); err != nil {
				logging.Fluent(logging.GetLoggerFromProfile(proc.Context().Profile)).
					Warn("vds: persist failed").WithError(err).Log()
			}
		}
		if applyVerify && rep.Passed() {
			n, err := vds.ApplyIndependentVerifyYes(chunksPath, rep.PassedChunkIDs())
			if err != nil {
				return errfmt.Errorf("vds evaluate --apply-verify: %w", err)
			}
			if n > 0 {
				logging.Fluent(logging.GetLoggerFromProfile(proc.Context().Profile)).
					Info("vds: applied independent_verify=yes").
					Int("chunks_updated", n).
					String("path", chunksPath).
					Log()
			}
		}

		if err := emitVDS(cmd, proc, rep, rep.AgentBrief); err != nil {
			return err
		}
		if !rep.Passed() {
			return errfmt.Errorf("vds evaluate: %s — %d chunk(s) failed (%s)",
				rep.Verdict, len(rep.FailedChunkIDs), strings.Join(rep.FailedChunkIDs, ", "))
		}
		return nil
	})(cmd, args)
}

func runVDSProject(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		var flags clipkg.FlagBag
		provider := flags.String(cmd, "provider")
		all := flags.Bool(cmd, "all")
		listOnly := flags.Bool(cmd, "list")
		write := flags.Bool(cmd, "write")
		check := flags.Bool(cmd, "check")
		outRel := flags.String(cmd, "out")
		execCtx, err := resolveVDSExecutionContext(cmd, &flags)
		if err != nil {
			return err
		}
		cfg := vds.ResolveVendorProviders(execCtx.cust)
		if listOnly {
			payload := map[string]any{
				"schema":    "zqk_vds_project_list_v1",
				"default":   cfg.Default,
				"providers": cfg.Providers,
			}
			return emitVDS(cmd, proc, payload, "")
		}
		opCtx := context.Background() // Background: request-or-shutdown derived
		if c := cmd.Context(); c != nil {
			opCtx = c
		}
		opt := vds.ProjectOptions{
			ProjectRoot:   execCtx.root,
			ProviderID:    provider,
			AllProviders:  all,
			OutRel:        outRel,
			TitleLookup:   vdsTitleLookup(proc),
			Customization: execCtx.cust,
			Spine:         execCtx.spine,
		}
		if sp := proc.Storage(); sp != nil {
			sec := proc.SecurityContext()
			opt.Lookup = func(ctx context.Context, id string) (map[string]any, error) {
				return sp.Read(ctx, sec, id)
			}
		}
		if all {
			batch, err := vds.WriteOrCheckAllProviders(opCtx, opt, write, check)
			if batch != nil {
				if emitErr := emitVDS(cmd, proc, batch, ""); emitErr != nil && err == nil {
					return emitErr
				}
			}
			return err
		}
		res, err := vds.WriteOrCheckProvider(opCtx, opt, write, check)
		if res != nil {
			if emitErr := emitVDS(cmd, proc, res, ""); emitErr != nil && err == nil {
				return emitErr
			}
		}
		return err
	})(cmd, args)
}

func runVDSInit(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		root, err := resolveVDSProjectRoot(cmd)
		if err != nil {
			return err
		}
		force, err := cmd.Flags().GetBool("force")
		if err != nil {
			return err
		}
		path, created, err := vds.InitChunksFile(root, force)
		if err != nil {
			return err
		}
		payload := map[string]any{
			"schema":             "zqk_vds_init_v1",
			objects.FieldKeyPath: path,
			"created":            created,
			"next":               paths.RewriteCanonicalCLIInvocations("Edit chunks, then: zqk workflow vds evaluate --format json"),
			"policy":             vds.PolicyID,
		}
		return emitVDS(cmd, proc, payload, "")
	})(cmd, args)
}

func emitVDS(cmd *cobra.Command, proc *cli.Processor, payload any, agentBrief string) error {
	format := cli.GetFormat(cmd)
	if format == cli.FormatAgentPrompt && strings.TrimSpace(agentBrief) != "" {
		return cli.WriteOutput(cmd, []byte(agentBrief))
	}
	_ = proc
	return cli.FormatOutput(cmd, payload)
}

func vdsTitleLookup(proc *cli.Processor) vds.TitleLookup {
	sp := proc.Storage()
	if sp == nil {
		return nil
	}
	sec := proc.SecurityContext()
	return func(ctx context.Context, kind, title string) (string, error) {
		stCtx := proc.StorageContext()
		res, err := sp.List(ctx, sec, stCtx, storage.ListFilter{
			Kind:    kind,
			Filters: map[string]any{objects.FieldKeyTitle: title},
			Limit:   5,
			Fields:  []string{objects.FieldKeyID, objects.FieldKeyTitle},
		})
		if err != nil {
			return "", err
		}
		if res == nil || len(res.Objects) == 0 {
			return "", nil
		}
		// Prefer exact title match; list filters may be approximate depending on backend.
		want := strings.TrimSpace(title)
		for _, obj := range res.Objects {
			if t, _ := obj[objects.FieldKeyTitle].(string); strings.TrimSpace(t) == want {
				return vds.IDFromObject(obj), nil
			}
		}
		return vds.IDFromObject(res.Objects[0]), nil
	}
}

func persistVDSReport(projectRoot string, rep *vds.Report) error {
	dir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, "vds")
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return err
	}
	path := filepath.Join(dir, "last_evaluate.json")
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFile(path, append(b, '\n'), paths.FilePerm644)
}
