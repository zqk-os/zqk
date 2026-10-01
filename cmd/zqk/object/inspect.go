package object

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/tui/tds"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
)

// SemanticAgentProjection is the high-signal, token-efficient projection
// designed for autonomous agent decision loops (SPEC-OBJECT-INSPECTOR-CONSOLE-001 §6.2).
type SemanticAgentProjection struct {
	Kind             string                    `json:"kind" yaml:"kind"`
	ID               string                    `json:"id" yaml:"id"`
	Status           string                    `json:"status,omitempty" yaml:"status,omitempty"`
	Priority         string                    `json:"priority,omitempty" yaml:"priority,omitempty"`
	Title            string                    `json:"title,omitempty" yaml:"title,omitempty"`
	ClaimedBy        string                    `json:"claimed_by,omitempty" yaml:"claimed_by,omitempty"`
	StorageProfile   *StorageProfileProjection `json:"storage_profile,omitempty" yaml:"storage_profile,omitempty"`
	Ontology         *OntologyProjection       `json:"ontology,omitempty" yaml:"ontology,omitempty"`
	Lineage          *LineageRadarProjection   `json:"lineage,omitempty" yaml:"lineage,omitempty"`
	CriteriaSummary  *CriteriaSummary          `json:"criteria_summary,omitempty" yaml:"criteria_summary,omitempty"`
	ActionsAvailable []string                  `json:"actions_available,omitempty" yaml:"actions_available,omitempty"`
	RawFields        map[string]any            `json:"raw_fields,omitempty" yaml:"raw_fields,omitempty"`
}

// StorageProfileProjection conveys physical and logical CAS storage parameters.
type StorageProfileProjection struct {
	CASHash      string `json:"cas_hash,omitempty" yaml:"cas_hash,omitempty"`
	StoragePlane string `json:"storage_plane,omitempty" yaml:"storage_plane,omitempty"` // "cas", "draft_plane", "stream_buffer"
	ByteSize     int64  `json:"byte_size" yaml:"byte_size"`
	Permissions  string `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	LastModified string `json:"last_modified,omitempty" yaml:"last_modified,omitempty"`
	FilePath     string `json:"file_path,omitempty" yaml:"file_path,omitempty"`
}

// OntologyProjection conveys schema traits, namespace isolation, and field registry data.
type OntologyProjection struct {
	Namespace             string   `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	VersionContext        string   `json:"version_context,omitempty" yaml:"version_context,omitempty"`
	StorageProfile        string   `json:"storage_profile,omitempty" yaml:"storage_profile,omitempty"`
	Traits                []string `json:"traits,omitempty" yaml:"traits,omitempty"`
	RegisteredFieldsCount int      `json:"registered_fields_count" yaml:"registered_fields_count"`
}

// LineageRadarProjection conveys the vertical traceability hierarchy of an object.
type LineageRadarProjection struct {
	Goal         string   `json:"goal,omitempty" yaml:"goal,omitempty"`
	Requirement  string   `json:"requirement,omitempty" yaml:"requirement,omitempty"`
	Milestone    string   `json:"milestone,omitempty" yaml:"milestone,omitempty"`
	PriorityPlan string   `json:"priority_plan,omitempty" yaml:"priority_plan,omitempty"`
	TestCases    []string `json:"test_cases,omitempty" yaml:"test_cases,omitempty"`
	IsIntact     bool     `json:"is_intact" yaml:"is_intact"`
}

// CriteriaSummary summarizes satisfaction of linked acceptance criteria.
type CriteriaSummary struct {
	Total     int `json:"total" yaml:"total"`
	Satisfied int `json:"satisfied" yaml:"satisfied"`
	Pending   int `json:"pending" yaml:"pending"`
}

// PolicyStudioProjection conveys policy verification status and field rules.
type PolicyStudioProjection struct {
	Kind        string           `json:"kind" yaml:"kind"`
	TotalRules  int              `json:"total_rules" yaml:"total_rules"`
	Fields      []string         `json:"fields" yaml:"fields"`
	Evaluations []RuleEvaluation `json:"evaluations,omitempty" yaml:"evaluations,omitempty"`
}

// RuleEvaluation conveys a single policy rule verification check.
type RuleEvaluation struct {
	RuleID         string   `json:"rule_id" yaml:"rule_id"`
	Description    string   `json:"description" yaml:"description"`
	Passed         bool     `json:"passed" yaml:"passed"`
	Violations     int      `json:"violations"`
	Expression     string   `json:"expression,omitempty" yaml:"expression,omitempty"`
	TotalEvaluated int      `json:"total_evaluated,omitempty" yaml:"total_evaluated,omitempty"`
	OffendingIDs   []string `json:"offending_ids,omitempty" yaml:"offending_ids,omitempty"`
	Summary        string   `json:"summary,omitempty" yaml:"summary,omitempty"`
}

// NewInspectCmd creates the inspect command handler.
func NewInspectCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectInspectCommandBuilder()
	cli.BindAsyncProgress(cmd, runInspect)
	cmd.Aliases = []string{"ins"}
	cmd.Flags().String("rule-expr", "", "Evaluate a custom DSL predicate expression across repository objects in real time")
	cmd.Flags().String("suggest-dsl", "", "Get autocomplete suggestions for a DSL input prefix")
	return cmd
}

func runInspect(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		ctx := proc.OperationContext()
		secCtx := proc.SecurityContext()
		if secCtx == nil {
			secCtx = pkgctx.NewSystemSecurityContext()
		}
		storageCtx := proc.StorageContext()
		sp := proc.Storage()

		// Read flags
		kindFlag, _ := cmd.Flags().GetString("kind")
		idFlag, _ := cmd.Flags().GetString("id")
		var rawFields []string
		if fStr, err := cmd.Flags().GetString("fields"); err == nil && fStr != "" {
			rawFields = append(rawFields, fStr)
		}
		if fArr, err := cmd.Flags().GetStringArray("fields"); err == nil && len(fArr) > 0 {
			rawFields = append(rawFields, fArr...)
		}
		if fSlice, err := cmd.Flags().GetStringSlice("fields"); err == nil && len(fSlice) > 0 {
			rawFields = append(rawFields, fSlice...)
		}
		var fieldsFlag []string
		for _, f := range rawFields {
			for _, sub := range strings.Split(f, ",") {
				if s := strings.TrimSpace(sub); s != "" {
					fieldsFlag = append(fieldsFlag, s)
				}
			}
		}

		var rawFilters []string
		if fStr, err := cmd.Flags().GetString("filter"); err == nil && fStr != "" {
			rawFilters = append(rawFilters, fStr)
		}
		if fArr, err := cmd.Flags().GetStringArray("filter"); err == nil && len(fArr) > 0 {
			rawFilters = append(rawFilters, fArr...)
		}
		if fSlice, err := cmd.Flags().GetStringSlice("filter"); err == nil && len(fSlice) > 0 {
			rawFilters = append(rawFilters, fSlice...)
		}
		var filterFlags []string
		for _, f := range rawFilters {
			for _, sub := range strings.Split(f, ",") {
				if s := strings.TrimSpace(sub); s != "" {
					filterFlags = append(filterFlags, s)
				}
			}
		}
		sortBy, _ := cmd.Flags().GetString("sort-by")
		if sortBy == "" {
			sortBy = "updated_at"
		}
		sortAsc, _ := cmd.Flags().GetBool("sort-asc")
		groupBy, _ := cmd.Flags().GetString("group-by")
		policyStudio, _ := cmd.Flags().GetBool("policy-studio")
		ruleExpr, _ := cmd.Flags().GetString("rule-expr")
		if strings.TrimSpace(ruleExpr) != "" || cmd.Flags().Changed("suggest-dsl") {
			policyStudio = true
		}

		targetKind := strings.TrimSpace(kindFlag)
		targetID := strings.TrimSpace(idFlag)

		// Positional argument resolution
		if len(args) >= 2 {
			targetKind = strings.TrimSpace(args[0])
			targetID = strings.TrimSpace(args[1])
		} else if len(args) == 1 {
			arg := strings.TrimSpace(args[0])
			inferredKind := validation.GetIDValidator().InferKindFromID(arg)
			if inferredKind != "" {
				targetID = arg
				if targetKind == "" {
					targetKind = inferredKind
				}
			} else {
				if targetKind == "" {
					targetKind = arg
				} else if targetID == "" {
					targetID = arg
				}
			}
		}

		if targetID != "" && targetKind == "" {
			targetKind = validation.GetIDValidator().InferKindFromID(targetID)
		}

		format := cli.GetFormat(cmd)
		isStructured := format == cli.FormatJSON || format == cli.FormatYAML || format == cli.FormatJSONRPC

		// Interactive TUI Mode:
		// When output is an interactive terminal (or --interactive is passed), not asking for structured JSON/YAML,
		// and not explicitly requesting non-interactive execution.
		interactiveFlag, _ := cmd.Flags().GetBool("interactive")
		isTerminal := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
		if (interactiveFlag || (isTerminal && targetID == "")) && !isStructured && !policyStudio {
			return RunInspectTUI(cmd, targetKind, fieldsFlag, filterFlags, sortBy, sortAsc, sp, ctx, secCtx, storageCtx)
		}

		// Branch 1: Policy Studio
		if policyStudio {
			return runPolicyStudio(cmd, targetKind, sp, ctx, secCtx, storageCtx, isStructured)
		}

		// Branch 2: Inspect Single Object
		if targetID != "" {
			return inspectSingleObject(cmd, targetKind, targetID, fieldsFlag, sp, ctx, secCtx, isStructured)
		}

		// Branch 3: Inspect Kind / Objects List
		if targetKind != "" {
			return inspectKindList(cmd, targetKind, fieldsFlag, filterFlags, sortBy, sortAsc, groupBy, sp, ctx, secCtx, storageCtx, isStructured)
		}

		// Branch 4: Discover Available Kinds
		return inspectAvailableKinds(cmd, sp, ctx, secCtx, storageCtx, isStructured)
	})(cmd, args)
}

func inspectSingleObject(cmd *cobra.Command, kind, id string, fields []string, sp storage.ObjectStorageProvider, ctx context.Context, secCtx *pkgctx.SecurityContext, isStructured bool) error {
	rawObj, err := sp.Read(ctx, secCtx, id)

	if err != nil || rawObj == nil {
		return errfmt.Errorf("object '%s' not found", id)
	}

	actualKind, _ := rawObj[objects.FieldKeyKind].(string)
	if actualKind == "" {
		actualKind = kind
	}

	proj := buildSemanticProjection(ctx, sp, secCtx, rawObj, actualKind, fields)

	if isStructured {
		return cli.FormatOutput(cmd, proj)
	}

	// Human / ANSI TUI rendering
	termWidth := getTerminalWidth()
	var lines []string

	statItems := []tds.StatItem{
		{Label: "Kind", Value: proj.Kind},
		{Label: "Status", Value: proj.Status, Extra: tds.Badge(proj.Status)},
		{Label: "Priority", Value: proj.Priority},
	}
	if proj.ClaimedBy != "" {
		statItems = append(statItems, tds.StatItem{Label: "Claimed By", Value: proj.ClaimedBy})
	}
	lines = append(lines, tds.StatRow(statItems, termWidth-4))
	lines = append(lines, "")

	if proj.Title != "" {
		lines = append(lines, fmt.Sprintf("Title: %s", proj.Title))
		lines = append(lines, "")
	}

	if proj.StorageProfile != nil {
		lines = append(lines, "── CAS Storage & Data-Cell Profile ──")
		hashDisplay := proj.StorageProfile.CASHash
		if len(hashDisplay) > 28 {
			hashDisplay = hashDisplay[:12] + "…" + hashDisplay[len(hashDisplay)-10:]
		}
		lines = append(lines, fmt.Sprintf("  Storage Plane: %s  │ CAS Hash: %s",
			tds.Badge(proj.StorageProfile.StoragePlane), hashDisplay))
		lines = append(lines, fmt.Sprintf("  Content Size:  %d bytes  │ Mode: %s  │ Modified: %s",
			proj.StorageProfile.ByteSize, proj.StorageProfile.Permissions, proj.StorageProfile.LastModified))
		if proj.StorageProfile.FilePath != "" {
			lines = append(lines, fmt.Sprintf("  File Path:     %s", proj.StorageProfile.FilePath))
		}
		lines = append(lines, "")
	}

	if proj.Ontology != nil {
		lines = append(lines, "── Ontology & Schema Profile ──")
		lines = append(lines, fmt.Sprintf("  Namespace:     %s  │ Version: %s  │ Storage: %s",
			proj.Ontology.Namespace, proj.Ontology.VersionContext, proj.Ontology.StorageProfile))
		lines = append(lines, fmt.Sprintf("  Field Count:   %d registered schema fields", proj.Ontology.RegisteredFieldsCount))
		if len(proj.Ontology.Traits) > 0 {
			lines = append(lines, fmt.Sprintf("  Traits:        [%s]", strings.Join(proj.Ontology.Traits, ", ")))
		}
		lines = append(lines, "")
	}

	if proj.Lineage != nil {
		lines = append(lines, "── Lineage & Traceability Radar ──")
		lines = append(lines, fmt.Sprintf("  Goal:          %s", defaultStr(proj.Lineage.Goal, "(none)")))
		lines = append(lines, fmt.Sprintf("  Milestone:     %s", defaultStr(proj.Lineage.Milestone, "(none)")))
		lines = append(lines, fmt.Sprintf("  Requirement:   %s", defaultStr(proj.Lineage.Requirement, "(none)")))
		lines = append(lines, fmt.Sprintf("  Priority Plan: %s", defaultStr(proj.Lineage.PriorityPlan, "(none)")))
		if len(proj.Lineage.TestCases) > 0 {
			lines = append(lines, fmt.Sprintf("  Test Cases:    %s", strings.Join(proj.Lineage.TestCases, ", ")))
		}
		intactBadge := tds.Badge("PASS")
		if !proj.Lineage.IsIntact {
			intactBadge = tds.Badge("WARN")
		}
		lines = append(lines, fmt.Sprintf("  Lineage Intact: %s", intactBadge))
		lines = append(lines, "")
	}

	if proj.CriteriaSummary != nil && proj.CriteriaSummary.Total > 0 {
		lines = append(lines, fmt.Sprintf("── Acceptance Criteria (%d total, %d satisfied, %d pending) ──",
			proj.CriteriaSummary.Total, proj.CriteriaSummary.Satisfied, proj.CriteriaSummary.Pending))
		lines = append(lines, "")
	}

	if len(proj.ActionsAvailable) > 0 {
		lines = append(lines, fmt.Sprintf("Available Actions: [%s]", strings.Join(proj.ActionsAvailable, ", ")))
	}

	if len(proj.RawFields) > 0 {
		lines = append(lines, "")
		lines = append(lines, "── Selected Fields ──")
		for k, v := range proj.RawFields {
			lines = append(lines, fmt.Sprintf("  %s: %v", k, v))
		}
	}

	panelOutput := tds.Panel("OBJECT INSPECTOR: "+proj.ID, lines, termWidth, tds.BorderRounded)
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), panelOutput)
	return nil
}

func inspectKindList(cmd *cobra.Command, kind string, fields, filters []string, sortBy string, sortAsc bool, groupBy string, sp storage.ObjectStorageProvider, ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *storage.StorageContext, isStructured bool) error {
	filterMap := make(map[string]any)
	for _, f := range filters {
		parts := strings.SplitN(f, "=", 2)
		if len(parts) == 2 {
			filterMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	res, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind:    kind,
		Filters: filterMap,
	})
	if err != nil {
		return errfmt.Errorf("failed to query objects of kind '%s': %w", kind, err)
	}

	filtered := make([]map[string]any, 0, len(res.Objects))
	for _, obj := range res.Objects {
		if matchesFilterExpr(obj, filters) {
			filtered = append(filtered, obj)
		}
	}

	// Sort objects
	sort.Slice(filtered, func(i, j int) bool {
		vi, _ := filtered[i][sortBy].(string)
		vj, _ := filtered[j][sortBy].(string)
		if sortAsc {
			return vi < vj
		}
		return vi > vj
	})

	projections := make([]SemanticAgentProjection, 0, len(filtered))
	for _, obj := range filtered {
		projections = append(projections, buildSemanticProjection(ctx, sp, secCtx, obj, kind, fields))
	}

	if isStructured {
		return cli.FormatOutput(cmd, projections)
	}

	// Human / TDS Table
	termWidth := getTerminalWidth()
	table := tds.NewTable(termWidth)
	table.AddColumn("ID", tds.AlignLeft, 16, 1.2)
	table.AddColumn("STATUS", tds.AlignLeft, 14, 1.0)
	table.AddColumn("PRI", tds.AlignCenter, 6, 0.5)
	table.AddColumn("TITLE", tds.AlignLeft, 28, 2.5)
	table.AddColumn("CLAIMED BY", tds.AlignLeft, 14, 1.0)

	for _, p := range projections {
		table.AddRow(
			p.ID,
			p.Status,
			defaultStr(p.Priority, "-"),
			truncateString(p.Title, 40),
			defaultStr(p.ClaimedBy, "-"),
		)
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), table.Render())
	return nil
}

func inspectAvailableKinds(cmd *cobra.Command, sp storage.ObjectStorageProvider, ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *storage.StorageContext, isStructured bool) error {
	commonKinds := []string{
		objects.KindBacklogItem,
		objects.KindRequirement,
		objects.KindCriteria,
		objects.KindTestCase,
		objects.KindMilestone,
		objects.KindPriorityPlan,
		objects.KindStrategicPlan,
		objects.KindPolicy,
		objects.KindWorkstream,
	}

	type kindInfo struct {
		Kind  string `json:"kind" yaml:"kind"`
		Count int    `json:"count" yaml:"count"`
	}

	results := make([]kindInfo, 0, len(commonKinds))
	for _, k := range commonKinds {
		res, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: k})
		cnt := 0
		if err == nil {
			cnt = len(res.Objects)
		}
		results = append(results, kindInfo{Kind: k, Count: cnt})
	}

	if isStructured {
		return cli.FormatOutput(cmd, results)
	}

	termWidth := getTerminalWidth()
	table := tds.NewTable(termWidth)
	table.AddColumn("OBJECT KIND", tds.AlignLeft, 24, 2.0)
	table.AddColumn("ACTIVE COUNT", tds.AlignRight, 12, 1.0)

	for _, r := range results {
		table.AddRow(r.Kind, fmt.Sprintf("%d", r.Count))
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), table.Render())
	return nil
}

func runPolicyStudio(cmd *cobra.Command, kind string, sp storage.ObjectStorageProvider, ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *storage.StorageContext, isStructured bool) error {
	if kind == "" {
		kind = objects.KindBacklogItem
	}

	// 1. Suggest DSL Autocompletion
	if cmd.Flags().Changed("suggest-dsl") {
		suggestPrefix, _ := cmd.Flags().GetString("suggest-dsl")
		suggestions := SuggestDSLTokens(kind, suggestPrefix)
		if isStructured {
			return cli.FormatOutput(cmd, suggestions)
		}
		termWidth := getTerminalWidth()
		table := tds.NewTable(termWidth)
		table.AddColumn("TOKEN", tds.AlignLeft, 24, 2.0)
		table.AddColumn("TYPE", tds.AlignLeft, 14, 1.0)
		table.AddColumn("DESCRIPTION", tds.AlignLeft, 42, 3.0)
		for _, s := range suggestions {
			table.AddRow(s.Token, string(s.Type), s.Description)
		}
		title := fmt.Sprintf("DSL AUTOCOMPLETE SUGGESTIONS: %s (prefix: %q)", strings.ToUpper(kind), suggestPrefix)
		panelOutput := tds.Panel(title, []string{table.Render()}, termWidth, tds.BorderRounded)
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), panelOutput)
		return nil
	}

	reg := objects.GetGlobalFieldRegistry()
	var registeredFields []string
	if reg != nil {
		if kf, err := reg.GetFieldsForKind(kind); err == nil && kf != nil {
			for _, f := range kf.AllFields {
				registeredFields = append(registeredFields, f.Name)
			}
		}
	}
	sort.Strings(registeredFields)

	rules := DefaultPolicyRulesForKind(kind)
	ruleExpr, _ := cmd.Flags().GetString("rule-expr")
	if strings.TrimSpace(ruleExpr) != "" {
		customRule := PolicyRule{
			ID:          "POL-CUSTOM-PREDICATE",
			Name:        "Custom Expression Rule",
			Description: fmt.Sprintf("Dynamic predicate evaluation: %s", ruleExpr),
			TargetKind:  kind,
			Expression:  strings.TrimSpace(ruleExpr),
			Severity:    "warning",
		}
		rules = append([]PolicyRule{customRule}, rules...)
	}

	var evals []RuleEvaluation
	if sp != nil {
		results, err := RunPolicyStudioDryRun(ctx, sp, secCtx, storageCtx, kind, rules)
		if err == nil {
			for _, r := range results {
				evals = append(evals, RuleEvaluation{
					RuleID:         r.RuleID,
					Description:    r.Name,
					Passed:         r.Passed,
					Violations:     r.ViolationsCount,
					Expression:     r.Expression,
					TotalEvaluated: r.TotalEvaluated,
					OffendingIDs:   r.OffendingIDs,
					Summary:        r.Summary,
				})
			}
		}
	}

	if len(evals) == 0 {
		for _, r := range rules {
			evals = append(evals, RuleEvaluation{
				RuleID:      r.ID,
				Description: r.Name,
				Passed:      true,
				Violations:  0,
				Expression:  r.Expression,
			})
		}
	}

	proj := PolicyStudioProjection{
		Kind:        kind,
		TotalRules:  len(evals),
		Fields:      registeredFields,
		Evaluations: evals,
	}

	if isStructured {
		return cli.FormatOutput(cmd, proj)
	}

	termWidth := getTerminalWidth()
	var lines []string
	lines = append(lines, fmt.Sprintf("Registered Schema Fields: %d fields available for DSL predicates", len(registeredFields)))
	lines = append(lines, "")
	lines = append(lines, "── Active Evaluation Rules ──")
	for _, e := range evals {
		statusBadge := tds.Badge("PASS")
		if !e.Passed {
			statusBadge = tds.Badge("FAIL")
		}
		violStr := ""
		if e.Violations > 0 {
			violStr = fmt.Sprintf(" (%d violations)", e.Violations)
		}
		lines = append(lines, fmt.Sprintf("  • [%s] %s  %s%s", e.RuleID, e.Description, statusBadge, violStr))
		if e.Expression != "" {
			lines = append(lines, fmt.Sprintf("      DSL: %s", e.Expression))
		}
		if len(e.OffendingIDs) > 0 {
			lines = append(lines, fmt.Sprintf("      Offenders: %s", strings.Join(e.OffendingIDs, ", ")))
		}
	}
	lines = append(lines, "")
	lines = append(lines, "Actions: [c] Edit Condition  [t] Test Expression  [s] Save Rule  [Esc] Close")

	panelOutput := tds.Panel("LIVE POLICY RULE STUDIO: "+strings.ToUpper(kind), lines, termWidth, tds.BorderRounded)
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), panelOutput)
	return nil
}

func buildSemanticProjection(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, obj map[string]any, kind string, requestedFields []string) SemanticAgentProjection {
	id, _ := obj[objects.FieldKeyID].(string)
	title, _ := obj[objects.FieldKeyTitle].(string)
	status, _ := obj[objects.FieldKeyStatus].(string)
	priority, _ := obj[objects.FieldKeyPriority].(string)
	if priority == "" {
		priority, _ = obj["priority_tier"].(string)
	}

	claimedBy, _ := obj["claimed_by"].(string)
	if claimedBy == "" {
		claimedBy, _ = obj["claimed_agent"].(string)
	}
	if claimedBy == "" {
		claimedBy, _ = obj["assignee"].(string)
	}

	lineage := extractLineageRadar(obj)
	critSummary := extractCriteriaSummary(ctx, sp, secCtx, obj)
	actions := determineAvailableActions(kind, status, claimedBy)
	storageProf := extractStorageProfile(sp, id, kind, obj)
	ontologyProf := extractOntologyProjection(obj, kind)

	var rawSelected map[string]any
	if len(requestedFields) > 0 {
		rawSelected = make(map[string]any)
		for _, f := range requestedFields {
			fTrim := strings.TrimSpace(f)
			if val, ok := obj[fTrim]; ok {
				rawSelected[fTrim] = val
			}
		}
	}

	return SemanticAgentProjection{
		Kind:             kind,
		ID:               id,
		Status:           status,
		Priority:         priority,
		Title:            title,
		ClaimedBy:        claimedBy,
		StorageProfile:   storageProf,
		Ontology:         ontologyProf,
		Lineage:          lineage,
		CriteriaSummary:  critSummary,
		ActionsAvailable: actions,
		RawFields:        rawSelected,
	}
}

func extractStorageProfile(sp storage.ObjectStorageProvider, id, kind string, obj map[string]any) *StorageProfileProjection {
	var filePath string
	if fos, ok := sp.(interface {
		GetFilePathForObject(id, kind string) (string, error)
	}); ok {
		if p, err := fos.GetFilePathForObject(id, kind); err == nil {
			filePath = p
		}
	}

	storagePlane := "cas"
	var byteSize int64
	var permissions string
	var lastModified string
	var casHash string

	if filePath != "" {
		if fi, err := os.Stat(filePath); err == nil {
			byteSize = fi.Size()
			permissions = fi.Mode().String()
			lastModified = fi.ModTime().UTC().Format(time.RFC3339)
			base := filepath.Base(filePath)
			if strings.HasSuffix(base, ".yaml") {
				stem := strings.TrimSuffix(base, ".yaml")
				if len(stem) == 64 {
					casHash = stem
				}
			}
		}
		if strings.Contains(filePath, "object_drafts") {
			storagePlane = "draft_plane"
		} else if strings.Contains(filePath, "stream") {
			storagePlane = "stream_buffer"
		}
	}

	if lastModified == "" {
		if upd, ok := obj["updated_at"].(string); ok && upd != "" {
			lastModified = upd
		} else if cr, ok := obj["created_at"].(string); ok && cr != "" {
			lastModified = cr
		}
	}

	if byteSize == 0 && obj != nil {
		if d, err := yaml.Marshal(obj); err == nil {
			byteSize = int64(len(d))
			if casHash == "" {
				h := sha256.Sum256(d)
				casHash = hex.EncodeToString(h[:])
			}
		}
	}

	if permissions == "" {
		permissions = "-rw-r--r--"
	}

	return &StorageProfileProjection{
		CASHash:      casHash,
		StoragePlane: storagePlane,
		ByteSize:     byteSize,
		Permissions:  permissions,
		LastModified: lastModified,
		FilePath:     filePath,
	}
}

func extractOntologyProjection(obj map[string]any, kind string) *OntologyProjection {
	var namespace string
	if ns, ok := obj["namespace_id"].(string); ok && ns != "" {
		namespace = ns
	}
	versionCtx := "default"
	if vc, ok := obj["version_context"].(string); ok && vc != "" {
		versionCtx = vc
	}

	var traits []string
	storageProfile := "cas_entity"
	sl := objects.NewSpecLoader("")
	if spec, err := sl.LoadSpec(kind); err == nil && spec != nil {
		if namespace == "" && spec.Namespace != "" {
			namespace = spec.Namespace
		}
		if spec.StorageProfile != "" {
			storageProfile = spec.StorageProfile
		}
		if len(spec.ResolvedTraits) > 0 {
			traits = spec.ResolvedTraits
		} else if len(spec.Traits) > 0 {
			traits = spec.Traits
		}
	}
	if len(traits) == 0 {
		traits = []string{"HasMetadata", "HasLifecycle", "HasAudit"}
	}
	if namespace == "" {
		namespace = "zqk:kernel"
	}

	fieldCount := 0
	reg := objects.GetGlobalFieldRegistry()
	if reg != nil {
		if kf, err := reg.GetFieldsForKind(kind); err == nil && kf != nil {
			fieldCount = len(kf.AllFields)
		}
	}

	return &OntologyProjection{
		Namespace:             namespace,
		VersionContext:        versionCtx,
		StorageProfile:        storageProfile,
		Traits:                traits,
		RegisteredFieldsCount: fieldCount,
	}
}

func extractLineageRadar(obj map[string]any) *LineageRadarProjection {
	goal := extractRef(obj, "goal_ref", "goal_refs")
	req := extractRef(obj, "requirement_ref", "requirement_refs")
	milestone := extractRef(obj, "milestone_ref", "milestone_refs")
	pplan := extractRef(obj, "priority_plan_ref", "priority_plan_refs")
	testCases := extractSlice(obj, "test_case_refs")

	isIntact := (req != "" || milestone != "" || goal != "")

	return &LineageRadarProjection{
		Goal:         goal,
		Requirement:  req,
		Milestone:    milestone,
		PriorityPlan: pplan,
		TestCases:    testCases,
		IsIntact:     isIntact,
	}
}

func extractCriteriaSummary(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, obj map[string]any) *CriteriaSummary {
	critRefs := extractSlice(obj, "criteria_refs")
	if len(critRefs) == 0 {
		return &CriteriaSummary{Total: 0, Satisfied: 0, Pending: 0}
	}

	satisfied := 0
	for _, ref := range critRefs {
		cObj, err := sp.Read(ctx, secCtx, ref)
		if err == nil && cObj != nil {
			st, _ := cObj[objects.FieldKeyStatus].(string)
			if st == "satisfied" || st == "complete" || st == "validated" {
				satisfied++
			}
		}
	}

	return &CriteriaSummary{
		Total:     len(critRefs),
		Satisfied: satisfied,
		Pending:   len(critRefs) - satisfied,
	}
}

func determineAvailableActions(kind, status, claimedBy string) []string {
	var actions []string
	switch kind {
	case objects.KindBacklogItem:
		if status != objects.ObjectStatusComplete {
			actions = append(actions, "transition_status")
		}
		if claimedBy != "" {
			actions = append(actions, "unclaim")
		} else {
			actions = append(actions, "claim")
		}
		actions = append(actions, "edit_properties")
	case objects.KindRequirement:
		actions = []string{"link_criteria", "generate_pipeline", "edit_properties"}
	case objects.KindPolicy:
		actions = []string{"dry_run", "evaluate", "edit_rules"}
	default:
		actions = []string{"transition_status", "edit_properties", "delete"}
	}
	return actions
}

func extractRef(obj map[string]any, singularKey, pluralKey string) string {
	if val, ok := obj[singularKey].(string); ok && val != "" {
		return val
	}
	if slice, ok := obj[pluralKey].([]any); ok && len(slice) > 0 {
		if first, ok := slice[0].(string); ok {
			return first
		}
	}
	if slice, ok := obj[pluralKey].([]string); ok && len(slice) > 0 {
		return slice[0]
	}
	return ""
}

func extractSlice(obj map[string]any, key string) []string {
	var result []string
	if slice, ok := obj[key].([]any); ok {
		for _, item := range slice {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
	} else if slice, ok := obj[key].([]string); ok {
		result = append(result, slice...)
	}
	return result
}

func defaultStr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func getTerminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w < 40 {
		return 80
	}
	if w > 120 {
		return 120
	}
	return w
}

func matchesFilterExpr(obj map[string]any, filters []string) bool {
	for _, f := range filters {
		fTrim := strings.TrimSpace(f)
		if fTrim == "" {
			continue
		}
		if strings.Contains(fTrim, "!=") {
			parts := strings.SplitN(fTrim, "!=", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
				objVal := fmt.Sprintf("%v", obj[k])
				if objVal == v {
					return false
				}
			}
		} else if strings.Contains(fTrim, "=") {
			parts := strings.SplitN(fTrim, "=", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
				objVal := fmt.Sprintf("%v", obj[k])
				if objVal != v {
					return false
				}
			}
		}
	}
	return true
}

