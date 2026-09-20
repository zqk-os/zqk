package vds

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ObjectLookup loads a kernel object by id (optional; nil skips object predicates).
type ObjectLookup func(ctx context.Context, id string) (map[string]any, error)

// EvalOptions controls depth of predicate evaluation.
type EvalOptions struct {
	ProjectRoot   string
	Customization *Customization
	Lookup        ObjectLookup
	TitleLookup   TitleLookup
	// RunCommands executes lint/scan commands from customization (expensive).
	RunCommands bool
	// CommandTimeout bounds each external command when RunCommands is set.
	CommandTimeout time.Duration
}

// PredicateResult is one DSL check outcome.
type PredicateResult struct {
	Predicate string `json:"predicate"`
	OK        bool   `json:"ok"`
	Detail    string `json:"detail,omitempty"`
	Skipped   bool   `json:"skipped,omitempty"`
}

func (o EvalOptions) timeout() time.Duration {
	if o.CommandTimeout > 0 {
		return o.CommandTimeout
	}
	return 60 * time.Second
}

// EvalPredicate runs a single DSL predicate.
func EvalPredicate(ctx context.Context, pred string, chunk Chunk, opt EvalOptions) PredicateResult {
	pred = strings.TrimSpace(pred)
	if pred == "" {
		return PredicateResult{Predicate: pred, OK: false, Detail: "empty predicate"}
	}
	name, arg, hasArg := strings.Cut(pred, ":")
	name = strings.TrimSpace(name)
	arg = strings.TrimSpace(arg)

	switch name {
	case "object_exists":
		return predObjectExists(ctx, pred, arg, opt)
	case "field_nonempty":
		return predFieldNonempty(ctx, pred, arg, opt)
	case "criteria_linked_or_acceptance_present":
		id := arg
		if id == "" {
			id = chunk.WorkObjectRef
		}
		return predCriteriaOrAcceptance(ctx, pred, id, opt)
	case "git_diff_nonempty_or_waiver":
		return predGitDiffOrWaiver(pred, chunk, opt)
	case "lint_ok_per_customization":
		return predLint(ctx, pred, opt)
	case "tests_ok_per_customization":
		return predTests(pred, chunk, opt)
	case "smoke_or_integration_evidence_present":
		return predEvidenceKind(pred, chunk, []string{"smoke", "integration", "e2e", paths.LogsDir, "SCH-", "job"}, "smoke/integration")
	case "ci_required_checks_green_or_na":
		return predCIOrNA(pred, chunk, opt)
	case "security_gate_ok_or_na":
		return predOptionalStageGate(pred, "security", chunk, opt)
	case "performance_gate_ok_or_na":
		return predOptionalStageGate(pred, "performance", chunk, opt)
	case "publish_ack_present_if_public":
		return predPublishAck(pred, chunk, opt)
	case "path_exists":
		return predPathExists(pred, arg, opt)
	default:
		_ = hasArg
		return PredicateResult{
			Predicate: pred,
			OK:        false,
			Detail:    "unknown predicate; see VERIFIABLE_DECOMPOSITION_SPINE.md DSL catalog",
		}
	}
}

func predPathExists(pred, rel string, opt EvalOptions) PredicateResult {
	if rel == "" {
		return PredicateResult{Predicate: pred, OK: false, Detail: "missing path"}
	}
	if opt.ProjectRoot == "" {
		return PredicateResult{Predicate: pred, OK: false, Skipped: true, Detail: "no project root; cannot verify path_exists"}
	}
	if pathExists(opt.ProjectRoot, rel) {
		return PredicateResult{Predicate: pred, OK: true, Detail: "path found"}
	}
	return PredicateResult{Predicate: pred, OK: false, Detail: "path missing: " + rel}
}

func predObjectExists(ctx context.Context, pred, id string, opt EvalOptions) PredicateResult {
	if id == "" {
		return PredicateResult{Predicate: pred, OK: false, Detail: "missing object id"}
	}
	if opt.Lookup == nil {
		return PredicateResult{Predicate: pred, OK: false, Skipped: true, Detail: "no object lookup wired; cannot verify object_exists"}
	}
	_, err := opt.Lookup(ctx, id)
	if err != nil {
		return PredicateResult{Predicate: pred, OK: false, Detail: err.Error()}
	}
	return PredicateResult{Predicate: pred, OK: true, Detail: "object found"}
}

func predFieldNonempty(ctx context.Context, pred, arg string, opt EvalOptions) PredicateResult {
	// arg: id:field
	id, field, ok := strings.Cut(arg, ":")
	id = strings.TrimSpace(id)
	field = strings.TrimSpace(field)
	if !ok || id == "" || field == "" {
		return PredicateResult{Predicate: pred, OK: false, Detail: "want field_nonempty:{id}:{field}"}
	}
	if opt.Lookup == nil {
		return PredicateResult{Predicate: pred, OK: false, Skipped: true, Detail: "no object lookup wired"}
	}
	obj, err := opt.Lookup(ctx, id)
	if err != nil {
		return PredicateResult{Predicate: pred, OK: false, Detail: err.Error()}
	}
	v, exists := obj[field]
	if !exists || v == nil {
		return PredicateResult{Predicate: pred, OK: false, Detail: "field missing: " + field}
	}
	switch t := v.(type) {
	case string:
		if strings.TrimSpace(t) == "" {
			return PredicateResult{Predicate: pred, OK: false, Detail: "field empty: " + field}
		}
	case []any:
		if len(t) == 0 {
			return PredicateResult{Predicate: pred, OK: false, Detail: "field empty list: " + field}
		}
	}
	return PredicateResult{Predicate: pred, OK: true, Detail: "field present"}
}

func predCriteriaOrAcceptance(ctx context.Context, pred, id string, opt EvalOptions) PredicateResult {
	if id == "" {
		return PredicateResult{Predicate: pred, OK: false, Detail: "missing object id"}
	}
	if opt.Lookup == nil {
		return PredicateResult{Predicate: pred, OK: false, Skipped: true, Detail: "no object lookup wired"}
	}
	obj, err := opt.Lookup(ctx, id)
	if err != nil {
		return PredicateResult{Predicate: pred, OK: false, Detail: err.Error()}
	}
	if nonemptyListOrString(obj[objects.FieldKeyCriteriaRefs]) {
		return PredicateResult{Predicate: pred, OK: true, Detail: "criteria_refs present"}
	}
	return PredicateResult{Predicate: pred, OK: false, Detail: "no criteria_refs"}
}

func nonemptyListOrString(v any) bool {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) != ""
	case []any:
		return len(t) > 0
	case []string:
		return len(t) > 0
	default:
		return v != nil
	}
}

func predGitDiffOrWaiver(pred string, chunk Chunk, opt EvalOptions) PredicateResult {
	if strings.TrimSpace(chunk.WaiverRef) != "" {
		return PredicateResult{Predicate: pred, OK: true, Detail: "waiver_ref set"}
	}
	if opt.ProjectRoot == "" {
		return PredicateResult{Predicate: pred, OK: false, Detail: "project root unknown"}
	}
	cmd := execwrap.Command("git", "-C", opt.ProjectRoot, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return PredicateResult{Predicate: pred, OK: false, Detail: "git status failed: " + err.Error()}
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		return PredicateResult{Predicate: pred, OK: false, Detail: "clean tree and no waiver; no implement footprint"}
	}
	return PredicateResult{Predicate: pred, OK: true, Detail: "working tree has changes"}
}

func predLint(ctx context.Context, pred string, opt EvalOptions) PredicateResult {
	cmds := []string{}
	if opt.Customization != nil {
		cmds = opt.Customization.CodeStyle.LintCommands
	}
	if len(cmds) == 0 {
		return PredicateResult{Predicate: pred, OK: true, Detail: "no lint_commands configured (na)", Skipped: true}
	}
	if !opt.RunCommands {
		return PredicateResult{
			Predicate: pred,
			OK:        false,
			Skipped:   true,
			Detail:    "lint commands configured; re-run with --run-commands to execute, or attach prior lint evidence",
		}
	}
	for _, c := range cmds {
		if err := runShell(ctx, opt.ProjectRoot, c, opt.timeout()); err != nil {
			return PredicateResult{Predicate: pred, OK: false, Detail: c + ": " + err.Error()}
		}
	}
	return PredicateResult{Predicate: pred, OK: true, Detail: "lint commands exited 0"}
}

func predTests(pred string, chunk Chunk, opt EvalOptions) PredicateResult {
	mode := "hybrid"
	example := ""
	if opt.Customization != nil && strings.TrimSpace(opt.Customization.TestExecution.Mode) != "" {
		mode = strings.ToLower(strings.TrimSpace(opt.Customization.TestExecution.Mode))
		example = opt.Customization.TestExecution.SchedulerSubmitExample
	}
	refs := chunk.EvidenceRefs
	hasJob := false
	hasLog := false
	hasGoTest := false
	hasShellTest := false
	for _, r := range refs {
		rl := strings.ToLower(r)
		if strings.Contains(rl, "sch-") || strings.Contains(rl, "job") {
			hasJob = true
		}
		if strings.Contains(rl, paths.LogsDir) || strings.HasSuffix(rl, ".log") || strings.Contains(rl, "health.jsonl") {
			hasLog = true
			if pathExists(opt.ProjectRoot, r) {
				hasLog = true
			}
		}
		if strings.Contains(rl, "go test") || strings.Contains(rl, "ci://") {
			hasGoTest = true
		}
		fields := strings.Fields(rl)
		if len(fields) >= 2 &&
			(fields[0] == "sh" || fields[0] == "bash") &&
			strings.Contains(filepath.Base(fields[1]), "test") {
			hasShellTest = true
		}
	}
	switch mode {
	case "scheduler":
		if hasJob && hasLog {
			return PredicateResult{Predicate: pred, OK: true, Detail: "scheduler job id + log evidence present"}
		}
		hint := "need evidence_refs with job id (SCH-*) and log path"
		if example != "" {
			hint += "; submit e.g. " + example
		}
		return PredicateResult{Predicate: pred, OK: false, Detail: hint}
	case "foreground":
		if hasGoTest || hasShellTest || hasLog {
			return PredicateResult{Predicate: pred, OK: true, Detail: "foreground/probe evidence present"}
		}
		return PredicateResult{Predicate: pred, OK: false, Detail: "need go test, shell test, or log evidence for foreground mode"}
	case "ci":
		if hasGoTest || strings.Contains(strings.Join(refs, " "), "ci://") {
			return PredicateResult{Predicate: pred, OK: true, Detail: "ci evidence present"}
		}
		return PredicateResult{Predicate: pred, OK: false, Detail: "need ci:// or CI check evidence_refs"}
	default: // hybrid
		if (hasJob && hasLog) || hasGoTest || hasShellTest {
			return PredicateResult{Predicate: pred, OK: true, Detail: "hybrid test evidence present"}
		}
		return PredicateResult{Predicate: pred, OK: false, Detail: "need scheduler job+log, go test, shell test, or ci evidence"}
	}
}

func predEvidenceKind(pred string, chunk Chunk, needles []string, label string) PredicateResult {
	joined := strings.ToLower(strings.Join(chunk.EvidenceRefs, " "))
	if joined == "" {
		return PredicateResult{Predicate: pred, OK: false, Detail: "no evidence_refs for " + label}
	}
	for _, n := range needles {
		if strings.Contains(joined, strings.ToLower(n)) {
			return PredicateResult{Predicate: pred, OK: true, Detail: "matched " + n}
		}
	}
	return PredicateResult{Predicate: pred, OK: false, Detail: "evidence_refs lack " + label + " footprint"}
}

func predCIOrNA(pred string, chunk Chunk, opt EvalOptions) PredicateResult {
	var required []string
	if opt.Customization != nil {
		required = opt.Customization.CICD.RequiredChecks
	}
	if len(required) == 0 {
		return PredicateResult{Predicate: pred, OK: true, Detail: "no required_checks configured (na)", Skipped: true}
	}
	joined := strings.ToLower(strings.Join(chunk.EvidenceRefs, " "))
	for _, c := range required {
		if !strings.Contains(joined, strings.ToLower(c)) {
			return PredicateResult{Predicate: pred, OK: false, Detail: "missing CI evidence for " + c}
		}
	}
	return PredicateResult{Predicate: pred, OK: true, Detail: "required CI checks referenced in evidence"}
}

func predOptionalStageGate(pred, kind string, chunk Chunk, opt EvalOptions) PredicateResult {
	var cmds []string
	var stages []string
	if opt.Customization != nil {
		if kind == "security" {
			cmds = opt.Customization.Security.ScanCommands
			stages = opt.Customization.Security.RequiredForStages
		} else {
			cmds = opt.Customization.Performance.ScanCommands
			stages = opt.Customization.Performance.RequiredForStages
		}
	}
	requiredHere := false
	for _, s := range stages {
		if s == chunk.Stage {
			requiredHere = true
			break
		}
	}
	if !requiredHere || len(cmds) == 0 {
		return PredicateResult{Predicate: pred, OK: true, Detail: kind + " gate na for this stage/config", Skipped: true}
	}
	if !opt.RunCommands {
		return predEvidenceKind(pred, chunk, append(cmds, kind, "scan"), kind)
	}
	ctx := context.Background()
	for _, c := range cmds {
		if err := runShell(ctx, opt.ProjectRoot, c, opt.timeout()); err != nil {
			return PredicateResult{Predicate: pred, OK: false, Detail: c + ": " + err.Error()}
		}
	}
	return PredicateResult{Predicate: pred, OK: true, Detail: kind + " commands exited 0"}
}

func predPublishAck(pred string, chunk Chunk, opt EvalOptions) PredicateResult {
	requires := true
	if opt.Customization != nil {
		requires = opt.Customization.CICD.PublishRequiresAck
	}
	if !requires {
		return PredicateResult{Predicate: pred, OK: true, Detail: "publish ack not required", Skipped: true}
	}
	joined := strings.ToLower(strings.Join(chunk.EvidenceRefs, " "))
	if strings.Contains(joined, "public_push_ack") || strings.Contains(joined, "publish_ack") {
		ackPath := filepath.Join(paths.ProjectDataDir, paths.StateDir, "public_push_ack.json")
		if pathExists(opt.ProjectRoot, ackPath) || strings.Contains(joined, ackPath) {
			return PredicateResult{Predicate: pred, OK: true, Detail: "publish ack referenced"}
		}
		return PredicateResult{Predicate: pred, OK: true, Detail: "publish ack token in evidence_refs"}
	}
	// Only fail when stage is operate_release
	if chunk.Stage != "operate_release" {
		return PredicateResult{Predicate: pred, OK: true, Detail: "publish ack not required outside operate_release", Skipped: true}
	}
	return PredicateResult{Predicate: pred, OK: false, Detail: "operate_release with publish_requires_ack needs public_push_ack evidence"}
}

func runShell(ctx context.Context, projectRoot, command string, timeout time.Duration) error {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := execwrap.CommandContext(cctx, "sh", "-c", command)
	cmd.Dir = projectRoot
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func pathExists(projectRoot, ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false
	}
	p := ref
	if !filepath.IsAbs(p) {
		p = filepath.Join(projectRoot, ref)
	}
	_, err := fileutil.Stat(p)
	return err == nil
}
