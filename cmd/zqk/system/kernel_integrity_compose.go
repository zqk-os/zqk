package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/kernelcas/compose"
	"github.com/zqk-os/zqk/pkg/objects"
	enumv "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/pipeline_definitions"
	"github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	"github.com/zqk-os/zqk/pkg/storage"
)

func newKernelIntegrityComposeCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemKernelIntegrityComposeCommandBuilder()
	cmd.RunE = runKernelIntegrityCompose
	return cmd
}

type composeMaterializeResult struct {
	Compiled int      `json:"compiled"`
	Created  int      `json:"created"`
	Updated  int      `json:"updated"`
	Titles   []string `json:"titles,omitempty"`
	Errors   []string `json:"errors,omitempty"`
	DryRun   bool     `json:"dry_run"`
}

func runKernelIntegrityCompose(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		dry, _ := cmd.Flags().GetBool("dry-run")
		c := compose.NewCompiler(proc.ProjectRoot())
		defs, err := c.CompileAllCritical()
		if err != nil {
			return err
		}
		_ = compose.WarmDefaultRegistry(proc.ProjectRoot())
		for _, d := range defs {
			compose.Default().Put(d)
		}
		out := composeMaterializeResult{Compiled: len(defs), DryRun: dry}
		if dry {
			for _, d := range defs {
				out.Titles = append(out.Titles, d.Key.Title())
			}
			return cli.FormatOutput(cmd, out)
		}
		// Materialize must not inherit a short-lived CLI op deadline — 150+ CAS updates
		// otherwise abort mid-batch with context canceled.
		matCtx := pkgctx.WithPromoteOnCreate(context.WithoutCancel(proc.OperationContext()))
		if matCtx.Err() != nil {
			matCtx = pkgctx.WithPromoteOnCreate(context.Background()) // Background: request-or-shutdown derived
		}
		res := materializePipelineDefinitions(matCtx, proc.Storage(), proc.SecurityContext(), defs)
		out.Created = res.Created
		out.Updated = res.Updated
		out.Titles = res.Titles
		out.Errors = res.Errors
		return cli.FormatOutput(cmd, out)
	})(cmd, args)
}

type matResult struct {
	Created int
	Updated int
	Titles  []string
	Errors  []string
}

func materializePipelineDefinitions(ctx context.Context, store storage.ObjectStorageProvider, sec *pkgctx.SecurityContext, defs []*compose.Definition) *matResult {
	res := &matResult{}
	if store == nil {
		res.Errors = append(res.Errors, "nil storage")
		return res
	}
	if sec == nil {
		sec = pkgctx.NewSystemSecurityContext()
	}
	if !pkgctx.GetPromoteOnCreate(ctx) {
		ctx = pkgctx.WithPromoteOnCreate(ctx)
	}
	existingByTitle := map[string]string{}
	existingByID := map[string]bool{}
	list, err := store.List(ctx, sec, pkgctx.NewStorageContext(), storage.ListFilter{Kind: objects.KindPipelineDefinition})
	if err == nil && list != nil {
		for _, obj := range list.Objects {
			title, _ := obj[objects.FieldKeyTitle].(string)
			id, _ := obj[objects.FieldKeyID].(string)
			if title != "" && id != "" {
				existingByTitle[title] = id
			}
			if id != "" {
				existingByID[id] = true
			}
		}
	}
	for _, def := range defs {
		if def == nil {
			continue
		}
		title := def.Key.Title()
		stableID := stablePipelineDefinitionID(def.Key)
		stages := composedStagesAsMaps(def)
		triggers := []any{
			map[string]any{
				objects.FieldKeyType:       "kernel_mutation",
				"composition_key":          def.Key.String(),
				objects.FieldKeyObjectKind: def.Key.ObjectKind,
				"pipeline_kind":            def.Key.PipelineKind,
				objects.FieldKeyIntent:     def.Key.Intent,
			},
		}
		desc := fmt.Sprintf("Composed mutation pipeline for %s intent=%s (Kernel Mutation Pipeline v2)", def.Key.ObjectKind, def.Key.Intent)
		updateID := ""
		if id, ok := existingByTitle[title]; ok {
			updateID = id
		} else if existingByID[stableID] {
			updateID = stableID
		} else if got, gerr := store.Read(ctx, sec, stableID); gerr == nil && got != nil {
			updateID = stableID
		}
		if updateID != "" {
			updates := map[string]any{
				objects.FieldKeyStages:      stages,
				objects.FieldKeyTriggers:    triggers,
				objects.FieldKeyDescription: desc,
				objects.FieldKeyTitle:       title,
			}
			if err := store.Update(ctx, sec, updateID, updates); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("%s update: %v", title, err))
				continue
			}
			res.Updated++
			res.Titles = append(res.Titles, title)
			continue
		}
		b := instance_builders.NewForKind(objects.KindPipelineDefinition, objects.DefaultSchemaVersion)
		b.SetID(stableID)
		b.SetField(objects.FieldKeyTitle, title)
		b.SetStatus(string(enumv.StatusApproved))
		b.SetField(objects.FieldKeyDescription, desc)
		b.SetField(objects.FieldKeyStages, stages)
		b.SetField(objects.FieldKeyTriggers, triggers)
		obj, err := b.Build()
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s build: %v", title, err))
			continue
		}
		if err := store.Create(ctx, sec, obj); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s create: %v", title, err))
			continue
		}
		res.Created++
		res.Titles = append(res.Titles, title)
	}
	return res
}

// stablePipelineDefinitionID derives a deterministic PLD-* id from the composition key
// so re-materialize is idempotent without relying on title-only matching for creates.
func stablePipelineDefinitionID(key compose.CompositionKey) string {
	sum := sha256.Sum256([]byte(key.String()))
	return "PLD-" + hex.EncodeToString(sum[:16])
}

func composedStagesAsMaps(def *compose.Definition) []any {
	out := make([]any, 0, len(def.Stages))
	for _, st := range def.Stages {
		m := map[string]any{objects.FieldKeyName: st.Name, objects.FieldKeyRole: st.Role}
		if len(st.Rules) > 0 {
			rules := make([]any, 0, len(st.Rules))
			for _, r := range st.Rules {
				rm := map[string]any{objects.FieldKeyID: r.ID, "op": r.Op}
				if len(r.Config) > 0 {
					rm["config"] = r.Config
				}
				rules = append(rules, rm)
			}
			m["rules"] = rules
		}
		out = append(out, m)
	}
	if len(out) > 0 && len(def.Sources) > 0 {
		srcs := make([]any, 0, len(def.Sources))
		for _, s := range def.Sources {
			srcs = append(srcs, map[string]any{objects.FieldKeyType: s.Type, "ref": s.Ref})
		}
		if m, ok := out[0].(map[string]any); ok {
			m["composition_sources"] = srcs
			m["composition_key"] = def.Key.String()
		}
	}
	return out
}
