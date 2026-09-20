package system

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewDiscoverGoalsCmd creates the system discover-goals command.
// Implements BLI-783: discover goals from vision/context per
// docs/architecture/project-discovery-and-strategic-alignment-v1.0.md.
func NewDiscoverGoalsCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Discover goals from vision and strategic context",
		"Lists existing goals and strategic context (vision, mission, strategic_plan) that can inform goal discovery.",
		"",
		"Use this to see current goals and context before adding or refining goals.",
	).
		AddExample("List goals and context", "%s system discover-goals").
		AddExample("JSON output", "%s system discover-goals --format json").
		ExcludeCommonFlags()

	discoverCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemDiscoverGoalsCommandBuilder(), &cobra.Command{
		Use: "discover-goals",
	})
	cli.BindAsyncProgress(discoverCmd, func(cmd *cobra.Command, args []string) error {
		return runDiscoverGoals(cmd)
	})

	helpBuilder.ApplyToCommand(discoverCmd)
	cli.AddCommonFlags(discoverCmd)

	return discoverCmd
}

func runDiscoverGoals(cmd *cobra.Command) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}

	projectRoot := ProjectRootOrResolveDot(ctx.ProjectRoot)
	if projectRoot == emptyValue {
		return errfmt.Errorf("not a ZQK project (no project root found)")
	}

	factory, err := storage.NewStorageFactory(cmd.Context(), projectRoot)
	if err != nil {
		return errfmt.Newf("storage factory").Wrap(err)
	}
	storageProvider := factory.GetStorage()
	if storageProvider != nil {
		defer func() { _ = storageProvider.Shutdown(context.Background()) }() // Background: request-or-shutdown derived
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// List goals
	goalsResult, err := storageProvider.List(cmd.Context(), secCtx, storageCtx, storage.ListFilter{
		Kind:    objects.KindGoal,
		Limit:   5000,
		SortBy:  "id",
		SortAsc: true,
	})
	if err != nil {
		return errfmt.Newf("list goals").Wrap(err)
	}

	// List vision and mission for context (optional kinds)
	contextKinds := []string{objects.KindVision, objects.KindMission, objects.KindStrategicPlan}
	contextByKind := make(map[string][]map[string]any)
	for _, kind := range contextKinds {
		listRes, err := storageProvider.List(cmd.Context(), secCtx, storageCtx, storage.ListFilter{
			Kind:    kind,
			Limit:   100,
			SortBy:  "id",
			SortAsc: true,
		})
		if err != nil || len(listRes.Objects) == 0 {
			continue
		}
		contextByKind[kind] = listRes.Objects
	}

	result := map[string]any{
		"goals":                 goalsResult.Objects,
		"goals_count":           len(goalsResult.Objects),
		objects.FieldKeyContext: contextByKind,
		"context_summary":       summarizeContext(contextByKind),
	}

	format := cli.GetFormat(cmd)
	switch format {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		if err := cli.FormatOutput(cmd, result); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(ctx.Profile)).Error("discover-goals format output", err).Log()
			return err
		}
		return nil
	default:
		return outputDiscoverGoalsTable(cmd, result)
	}
}

func summarizeContext(contextByKind map[string][]map[string]any) map[string]int {
	out := make(map[string]int)
	for k, objs := range contextByKind {
		out[k] = len(objs)
	}
	return out
}

func outputDiscoverGoalsTable(cmd *cobra.Command, result map[string]any) error {
	var buf strings.Builder

	goals, _ := result["goals"].([]map[string]any)
	goalsCount, _ := result["goals_count"].(int)
	summary, _ := result["context_summary"].(map[string]int)

	buf.WriteString("Goal discovery\n")
	buf.WriteString("-------------\n")
	fmt.Fprintf(&buf, "  Existing goals: %d\n", goalsCount)
	if len(summary) > 0 {
		buf.WriteString("  Strategic context:\n")
		for k, n := range summary {
			fmt.Fprintf(&buf, "    - %s: %d\n", k, n)
		}
	}
	buf.WriteString("\n")

	if goalsCount > 0 {
		buf.WriteString("Goals:\n")
		for _, obj := range goals {
			id, _ := obj[objects.FieldKeyID].(string)
			title, _ := obj[objects.FieldKeyTitle].(string)
			if title == emptyValue {
				title = "(no title)"
			}
			fmt.Fprintf(&buf, "  - %s %s\n", id, title)
		}
	} else {
		buf.WriteString("No goals found. Create goals with:\n")
		buf.WriteString("  zqk object create goal --file goal.yaml\n")
		buf.WriteString("Or add vision/mission/strategic_plan context to inform goal discovery.\n")
	}

	return cli.WriteOutput(cmd, []byte(buf.String()))
}
