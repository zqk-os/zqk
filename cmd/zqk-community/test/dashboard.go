package test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type dashboardRow struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	Status   string         `json:"status"`
	Criteria []criterionRow `json:"criteria"`
}

type criterionRow struct {
	ID     string `json:"id"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status"`
	Bound  bool   `json:"bound"`
}

type dashboardPayload struct {
	View            string         `json:"view"`
	TestCases       []dashboardRow `json:"test_cases"`
	UnboundCriteria []criterionRow `json:"unbound_criteria"`
	MissingRefs     []string       `json:"missing_refs,omitempty"`
	Counts          map[string]int `json:"counts"`
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
				if err := displayTestDashboard(cmd, proc, bgCtx); err != nil {
					return err
				}
				if err := cli.WriteOutput(cmd, []byte("\033[J")); err != nil {
					return err
				}
				time.Sleep(watchInterval)
			}
		}
		return displayTestDashboard(cmd, proc, bgCtx)
	})(cmd, args)
}

func displayTestDashboard(cmd *cobra.Command, proc *cli.Processor, ctx context.Context) error {
	view, _ := cmd.Flags().GetString("view")
	if view == "" {
		view = "active"
	}
	includeAll, _ := cmd.Flags().GetBool("all")
	statusFilter, _ := cmd.Flags().GetString("status")
	testCaseID, _ := cmd.Flags().GetString("test-case")
	checkDoD, _ := cmd.Flags().GetBool("check-dod")

	payload, err := buildTestDashboard(ctx, proc, view, includeAll, statusFilter, testCaseID)
	if err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}

	if checkDoD && len(payload.MissingRefs) > 0 {
		return cli.Guard(cmd).Err(fmt.Errorf("definition of done failed: missing criteria %s", strings.Join(payload.MissingRefs, ", "))).Return()
	}

	format := cli.GetFormat(cmd)
	if format == cli.FormatJSON || format == cli.FormatYAML || format == cli.FormatJSONL {
		return cli.FormatOutput(cmd, payload)
	}
	return cli.WriteOutput(cmd, []byte(renderTestDashboard(payload)))
}

func buildTestDashboard(ctx context.Context, proc *cli.Processor, view string, includeAll bool, statusFilter, testCaseID string) (dashboardPayload, error) {
	sp := proc.Storage()
	sec := proc.SecurityContext()
	sctx := proc.StorageContext()

	cases, err := listKind(ctx, sp, sec, sctx, objects.KindTestCase)
	if err != nil {
		return dashboardPayload{}, err
	}
	crits, err := listKind(ctx, sp, sec, sctx, objects.KindCriteria)
	if err != nil {
		return dashboardPayload{}, err
	}

	critByID := map[string]map[string]any{}
	for _, c := range crits {
		id := objectID(c)
		if id != "" {
			critByID[id] = c
		}
	}

	bound := map[string]struct{}{}
	var rows []dashboardRow
	var missing []string
	seenMissing := map[string]struct{}{}

	for _, tc := range cases {
		id := objectID(tc)
		status := objectStatus(tc)
		if testCaseID != "" && id != testCaseID {
			continue
		}
		if !includeTestCase(status, view, statusFilter, includeAll) {
			continue
		}
		row := dashboardRow{
			ID:     id,
			Title:  objectTitle(tc),
			Status: status,
		}
		for _, cid := range objects.KernelObjectRefIDs(tc, objects.FieldKeyCriteriaRefs) {
			bound[cid] = struct{}{}
			cr := criterionRow{ID: cid, Bound: true}
			if obj, ok := critByID[cid]; ok {
				cr.Title = objectTitle(obj)
				cr.Status = objectStatus(obj)
			} else {
				cr.Status = "missing"
				if _, seen := seenMissing[cid]; !seen {
					seenMissing[cid] = struct{}{}
					missing = append(missing, cid)
				}
			}
			row.Criteria = append(row.Criteria, cr)
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })

	var unbound []criterionRow
	for _, c := range crits {
		id := objectID(c)
		if id == "" {
			continue
		}
		if _, ok := bound[id]; ok {
			continue
		}
		st := objectStatus(c)
		if !includeTestCase(st, view, "", includeAll) {
			continue
		}
		unbound = append(unbound, criterionRow{
			ID:     id,
			Title:  objectTitle(c),
			Status: st,
			Bound:  false,
		})
	}
	sort.Slice(unbound, func(i, j int) bool { return unbound[i].ID < unbound[j].ID })

	counts := map[string]int{
		"test_cases":       len(rows),
		"unbound_criteria": len(unbound),
		"missing_refs":     len(missing),
	}
	return dashboardPayload{
		View:            view,
		TestCases:       rows,
		UnboundCriteria: unbound,
		MissingRefs:     missing,
		Counts:          counts,
	}, nil
}

func renderTestDashboard(p dashboardPayload) string {
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	bold := color.New(color.Bold).SprintFunc()

	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("\n%s\n", bold("TEST DASHBOARD | criteria lineage")))
	buf.WriteString(fmt.Sprintf("%s\n", strings.Repeat("=", 56)))
	buf.WriteString(fmt.Sprintf("  view %-10s  cases %d  unbound %d  missing %d\n\n",
		p.View, p.Counts["test_cases"], p.Counts["unbound_criteria"], p.Counts["missing_refs"]))

	if len(p.TestCases) == 0 {
		buf.WriteString(fmt.Sprintf("  %s No test cases in this view. Create one with `new object test_case`.\n", yellow("·")))
	}
	for _, row := range p.TestCases {
		buf.WriteString(fmt.Sprintf("  %s %s  %s\n", statusIcon(row.Status, green, yellow, red), cyan(row.ID), row.Title))
		if len(row.Criteria) == 0 {
			buf.WriteString(fmt.Sprintf("      %s no criteria_refs\n", yellow("·")))
			continue
		}
		for _, cr := range row.Criteria {
			label := cr.Title
			if label == "" {
				label = cr.ID
			}
			buf.WriteString(fmt.Sprintf("      %s %s  %s\n", statusIcon(cr.Status, green, yellow, red), cr.ID, label))
		}
	}
	if len(p.UnboundCriteria) > 0 {
		buf.WriteString(fmt.Sprintf("\n%s\n", bold("Unbound criteria")))
		for _, cr := range p.UnboundCriteria {
			buf.WriteString(fmt.Sprintf("  %s %s  %s\n", statusIcon(cr.Status, green, yellow, red), cr.ID, cr.Title))
		}
	}
	buf.WriteString("\n")
	return buf.String()
}

func includeTestCase(status, view, statusFilter string, includeAll bool) bool {
	if statusFilter != "" && status != statusFilter {
		return false
	}
	if includeAll && view == "active" {
		view = "all"
	}
	if status == objects.ObjectStatusArchived && !includeAll {
		return false
	}
	switch view {
	case "regression":
		return status == objects.ObjectStatusComplete || status == objects.ObjectStatusCompleted
	case "all":
		return true
	default:
		return status != objects.ObjectStatusComplete &&
			status != objects.ObjectStatusCompleted &&
			status != objects.ObjectStatusArchived
	}
}

func listKind(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext, sctx *pkgctx.StorageContext, kind string) ([]map[string]any, error) {
	res, err := sp.List(ctx, sec, sctx, storage.ListFilter{Kind: kind, Limit: 0})
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	return res.Objects, nil
}

func objectID(obj map[string]any) string {
	id, _ := obj[objects.FieldKeyID].(string)
	return id
}

func objectStatus(obj map[string]any) string {
	st, _ := obj[objects.FieldKeyStatus].(string)
	return st
}

func objectTitle(obj map[string]any) string {
	if title, ok := obj[objects.FieldKeyTitle].(string); ok && title != "" {
		return title
	}
	return objectID(obj)
}

func statusIcon(status string, green, yellow, red func(a ...any) string) string {
	switch status {
	case objects.ObjectStatusComplete, objects.ObjectStatusCompleted:
		return green("✓")
	case objects.ObjectStatusArchived:
		return "·"
	case objects.ObjectStatusActive, "in_progress", "awaiting_verification":
		return yellow("●")
	case "missing":
		return red("✘")
	default:
		return yellow("○")
	}
}
