package system

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func newDashboardCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemDashboardCommandBuilder()
	cmd.Short = "First-run kernel pulse: plan, backlog, criteria, and test cases"
	cmd.Long = "Shows the community first-run scoreboard. Lineage detail lives on `test dashboard`, not this command."
	cmd.RunE = runDashboard
	return cmd
}

func runDashboard(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		watchStr, _ := cmd.Flags().GetString("watch")
		var watchInterval time.Duration
		if watchStr != "0" && watchStr != "" {
			watchInterval, _ = time.ParseDuration(watchStr)
		}
		bgCtx := context.Background()
		if watchInterval > 0 {
			if err := cli.WriteOutput(cmd, []byte("\033[2J")); err != nil {
				return err
			}
			for {
				if err := cli.WriteOutput(cmd, []byte("\033[H")); err != nil {
					return err
				}
				if err := displayDashboard(cmd, proc, bgCtx); err != nil {
					return err
				}
				if err := cli.WriteOutput(cmd, []byte("\033[J")); err != nil {
					return err
				}
				time.Sleep(watchInterval)
			}
		}
		return displayDashboard(cmd, proc, bgCtx)
	})(cmd, args)
}

type kernelPulse struct {
	LeadPlan     map[string]any            `json:"lead_plan,omitempty"`
	Counts       map[string]map[string]int `json:"counts"`
	TestCases    int                       `json:"test_cases"`
	Criteria     int                       `json:"criteria"`
	BacklogItems int                       `json:"backlog_items"`
}

func displayDashboard(cmd *cobra.Command, proc *cli.Processor, ctx context.Context) error {
	pulse, err := buildKernelPulse(ctx, proc)
	if err != nil {
		return err
	}
	format := cli.GetFormat(cmd)
	if format == cli.FormatJSON || format == cli.FormatYAML || format == cli.FormatJSONL {
		return cli.FormatOutput(cmd, pulse)
	}
	return cli.WriteOutput(cmd, []byte(renderKernelPulse(pulse)))
}

func buildKernelPulse(ctx context.Context, proc *cli.Processor) (kernelPulse, error) {
	sp := proc.Storage()
	sec := proc.SecurityContext()
	sctx := proc.StorageContext()

	kinds := []string{
		objects.KindPriorityPlan,
		objects.KindBacklogItem,
		objects.KindCriteria,
		objects.KindTestCase,
		objects.KindRequirement,
	}
	counts := map[string]map[string]int{}
	listed := map[string][]map[string]any{}
	for _, kind := range kinds {
		objs, err := listKindMaps(ctx, sp, sec, sctx, kind)
		if err != nil {
			return kernelPulse{}, err
		}
		listed[kind] = objs
		counts[kind] = countByStatus(objs)
	}

	return kernelPulse{
		LeadPlan:     pickLeadPlan(listed[objects.KindPriorityPlan]),
		Counts:       counts,
		TestCases:    len(listed[objects.KindTestCase]),
		Criteria:     len(listed[objects.KindCriteria]),
		BacklogItems: len(listed[objects.KindBacklogItem]),
	}, nil
}

func renderKernelPulse(p kernelPulse) string {
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	bold := color.New(color.Bold).SprintFunc()
	exe := brand.ExecutableName()

	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("\n%s\n", bold("KERNEL PULSE | first-run")))
	buf.WriteString(fmt.Sprintf("%s\n\n", strings.Repeat("=", 56)))

	if p.LeadPlan != nil {
		title, _ := p.LeadPlan[objects.FieldKeyTitle].(string)
		status, _ := p.LeadPlan[objects.FieldKeyStatus].(string)
		id, _ := p.LeadPlan[objects.FieldKeyID].(string)
		buf.WriteString(fmt.Sprintf("  %-14s %s\n", "Lead plan:", cyan(title)))
		buf.WriteString(fmt.Sprintf("  %-14s %s (%s)\n", "", id, status))
		refs := objects.KernelObjectRefIDs(p.LeadPlan, objects.FieldKeyBacklogItemRefs)
		if n := len(refs); n > 0 {
			buf.WriteString(fmt.Sprintf("  %-14s %s items on plan\n", "", green(fmt.Sprintf("%d", n))))
		}
	} else {
		buf.WriteString(fmt.Sprintf("  %s No live priority plan. Run `%s workflow whats-next`.\n", yellow("·"), exe))
	}
	buf.WriteString("\n")
	buf.WriteString(fmt.Sprintf("%s\n", bold("Counts")))
	for _, kind := range []string{objects.KindPriorityPlan, objects.KindBacklogItem, objects.KindRequirement, objects.KindCriteria, objects.KindTestCase} {
		buf.WriteString(fmt.Sprintf("  %-16s %s\n", kind, formatStatusCounts(p.Counts[kind])))
	}
	buf.WriteString("\n")
	buf.WriteString(fmt.Sprintf("%s\n", bold("Next")))
	buf.WriteString(fmt.Sprintf("  %s test dashboard\n", exe))
	buf.WriteString(fmt.Sprintf("  %s workflow whats-next --format json\n\n", exe))
	return buf.String()
}

func pickLeadPlan(plans []map[string]any) map[string]any {
	var candidates []map[string]any
	for _, obj := range plans {
		status, _ := obj[objects.FieldKeyStatus].(string)
		if status == objects.ObjectStatusArchived || status == objects.ObjectStatusComplete || status == objects.ObjectStatusCompleted {
			continue
		}
		candidates = append(candidates, obj)
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		statusI, _ := candidates[i][objects.FieldKeyStatus].(string)
		statusJ, _ := candidates[j][objects.FieldKeyStatus].(string)
		if statusI != statusJ {
			if statusI == objects.ObjectStatusActive {
				return true
			}
			if statusJ == objects.ObjectStatusActive {
				return false
			}
		}
		idI, _ := candidates[i][objects.FieldKeyID].(string)
		idJ, _ := candidates[j][objects.FieldKeyID].(string)
		return idI > idJ
	})
	return candidates[0]
}

func countByStatus(objs []map[string]any) map[string]int {
	out := map[string]int{}
	for _, obj := range objs {
		st, _ := obj[objects.FieldKeyStatus].(string)
		if st == "" {
			st = "unknown"
		}
		out[st]++
	}
	return out
}

func formatStatusCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return "0"
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, counts[k]))
	}
	return strings.Join(parts, " ")
}

func listKindMaps(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext, sctx *pkgctx.StorageContext, kind string) ([]map[string]any, error) {
	res, err := sp.List(ctx, sec, sctx, storage.ListFilter{Kind: kind, Limit: 0})
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	return res.Objects, nil
}
