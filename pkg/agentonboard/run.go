package agentonboard

import (
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specialization"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// SeatFunc seeds default agent seating; injected so CLI can call system.SeedDefaultAgentSeatingPack
// without an import cycle (system → agentonboard).
type SeatFunc func(projectRoot string, logger logging.Logger) (created int, err error)

// Options configures a Vector A/B onboard run.
type Options struct {
	ProjectRoot  string
	Logger       logging.Logger
	Seat         SeatFunc
	DryRun       bool
	DetectOnly   bool
	SkipSeat     bool
	SkipPrime    bool
	AllVendors   bool
	Headless     bool                  // force Vector B: prime AGENTS.md only (unless AllVendors)
	Force        bool                  // overwrite existing vendor directive files
	VendorFilter map[VendorID]struct{} // nil = no filter
	SessionOK    bool                  // soft auth signal from caller
}

// StageResult is one fail-closed stage outcome.
type StageResult struct {
	Status string         `json:"status"` // ok | warn | skipped | failed
	Detail map[string]any `json:"detail,omitempty"`
	Error  string         `json:"error,omitempty"`
}

// Result is the structured onboard outcome (FormatOutput payload).
type Result struct {
	Schema             string                 `json:"schema"`
	Status             string                 `json:"status"` // success | blocked | failed
	Vector             string                 `json:"vector"`
	Stages             map[string]StageResult `json:"stages"`
	NextSteps          []string               `json:"next_steps"`
	SyncReport         string                 `json:"sync_report,omitempty"`
	EdgeSignals        []EdgeSignal           `json:"edge_signals,omitempty"`
	MarketProbeOpen    []MarketProbeQuestion  `json:"market_probe_open,omitempty"`
	SpecializationTier string                 `json:"specialization_tier,omitempty"`
	PackPaths          []string               `json:"pack_paths,omitempty"`
}

// Run executes detect → (auth warn) → seat → prime → kernel sync → smoke.
func Run(opts Options) (*Result, error) {
	root := opts.ProjectRoot
	if root == "" {
		return nil, errfmt.Errorf("project root is required")
	}
	if _, err := fileutil.Stat(root); err != nil {
		return nil, errfmt.Newf("project root").Wrap(err)
	}

	res := &Result{
		Schema: ResultSchema,
		Stages: map[string]StageResult{},
	}

	detected := DetectVendors(root)
	edge := CollectEdgeSignals(root)
	res.EdgeSignals = edge
	res.SpecializationTier = string(specialization.GetCurrentTier())
	res.Vector = InferVector(detected)
	if opts.Headless {
		res.Vector = "B"
	}
	res.Stages[StageDetect] = StageResult{
		Status: StageOK,
		Detail: map[string]any{
			"detected":            detected,
			"count":               len(detected),
			"edge_signals":        edge,
			"headless_requested":  opts.Headless,
			"strong_edge_signals": AnyStrongEdgeSignal(edge),
		},
	}
	if res.Vector == "B" {
		res.MarketProbeOpen = MarketProbeChecklist()
	}

	authStatus := StageOK
	authDetail := map[string]any{"session_ok": opts.SessionOK}
	if !opts.SessionOK {
		if zqkenv.IsCommunityEdition {
			authStatus = StageOK
			authDetail[objects.FieldKeyNote] = "Community edition local session active"
		} else {
			authStatus = StageWarn
			authDetail[objects.FieldKeyNote] = "No active session observed; seating/object mutate may fail until auth login"
		}
	}
	res.Stages[StageAuth] = StageResult{Status: authStatus, Detail: authDetail}

	if opts.DetectOnly {
		res.Status = ResultSuccess
		res.Stages[StageSeat] = StageResult{Status: StageSkipped}
		res.Stages[StagePrimeWorkspace] = StageResult{Status: StageSkipped}
		res.Stages[StagePrimeKernel] = StageResult{Status: StageSkipped}
		res.Stages[StageSmoke] = StageResult{Status: StageSkipped}
		res.NextSteps = []string{
			"Re-run without --detect-only to seat, prime workspace directives, and write sync report",
			"docs/onboarding/COMMUNITY_FIRST_RUN.md",
			"docs/onboarding/EDGE_HEADLESS_FIRST_RUN.md",
		}
		return res, nil
	}

	seatingCreated := 0
	if opts.SkipSeat {
		res.Stages[StageSeat] = StageResult{Status: StageSkipped}
	} else if opts.Seat == nil {
		res.Stages[StageSeat] = StageResult{
			Status: StageFailed,
			Error:  "seating function not configured",
		}
		res.Status = ResultBlocked
		res.NextSteps = []string{"Internal error: seat func missing; report to maintainers"}
		return res, nil
	} else if opts.DryRun {
		res.Stages[StageSeat] = StageResult{
			Status: StageOK,
			Detail: map[string]any{"dry_run": true, objects.FieldKeyNote: "would seed default seating if missing"},
		}
	} else {
		created, err := opts.Seat(root, opts.Logger)
		if err != nil {
			res.Stages[StageSeat] = StageResult{Status: StageFailed, Error: err.Error()}
			res.Status = ResultBlocked
			exe := brand.ExecutableName()
			res.NextSteps = []string{
				"Fix seating errors, then re-run: " + exe + " system agent-onboard",
				"Or repair: " + exe + " system seed-default-agent-seating",
			}
			return res, nil
		}
		seatingCreated = created
		res.Stages[StageSeat] = StageResult{
			Status: StageOK,
			Detail: map[string]any{"created": created},
		}
	}

	var primed, skipped, packPaths []string
	if opts.SkipPrime {
		res.Stages[StagePrimeWorkspace] = StageResult{Status: StageSkipped}
	} else {
		vendors := ensureAgentsMD(FilterVendors(VendorsToPrime(detected, opts.AllVendors), opts.VendorFilter))
		if (opts.Headless || res.Vector == "B") && !opts.AllVendors {
			vendors = headlessVendorsOnly(vendors)
		}
		wr, err := WriteVendorConfigs(root, vendors, opts.DryRun, opts.Force)
		if err != nil {
			res.Stages[StagePrimeWorkspace] = StageResult{Status: StageFailed, Error: err.Error()}
			res.Status = ResultBlocked
			res.NextSteps = []string{"Fix workspace write errors, then re-run agent-onboard"}
			return res, nil
		}
		primed, skipped = wr.Written, wr.Skipped
		packPaths, err = WriteVendorPacks(root, vendors, opts.DryRun, opts.Force)
		if err != nil {
			res.Stages[StagePrimeWorkspace] = StageResult{Status: StageFailed, Error: err.Error()}
			res.Status = ResultBlocked
			res.NextSteps = []string{"Fix .zqk/agent_packs write errors, then re-run agent-onboard"}
			return res, nil
		}
		res.PackPaths = packPaths
		res.Stages[StagePrimeWorkspace] = StageResult{
			Status: StageOK,
			Detail: map[string]any{
				"files":                 primed,
				objects.FieldKeySkipped: skipped,
				"pack_paths":            packPaths,
				"dry_run":               opts.DryRun,
				"force":                 opts.Force,
				"vendors":               vendorIDs(vendors),
			},
		}
	}

	smokeOK, smokeDetail := smoke(root, opts.DryRun, opts.SkipPrime, primed, skipped)
	fp := WorkspaceFingerprint(root, res.Vector, detected)
	report := SyncReport{
		Schema:         SyncReportSchema,
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
		Vector:         res.Vector,
		Detected:       detected,
		PrimedFiles:    primed,
		SkippedFiles:   skipped,
		PackPaths:      packPaths,
		Fingerprint:    fp,
		EdgeSignals:    edge,
		SeatingCreated: seatingCreated,
		SmokeOK:        smokeOK,
		// TRACK: BLI-AGENT-ONBOARD-HYGIENE-001 — promote lite sync (+ fingerprint) to CAS object when multi-workspace needs graph identity
		Notes: []string{"workspace→kernel lite registration; packs under .zqk/agent_packs/; seating objects live in CAS"},
	}

	rel, err := WriteSyncReport(root, report, opts.DryRun)
	if err != nil {
		res.Stages[StagePrimeKernel] = StageResult{Status: StageFailed, Error: err.Error()}
		res.Status = ResultBlocked
		res.NextSteps = []string{"Fix .zqk/agent-runtime write permissions, then re-run"}
		return res, nil
	}
	res.SyncReport = rel
	res.Stages[StagePrimeKernel] = StageResult{
		Status: StageOK,
		Detail: map[string]any{objects.FieldKeyPath: rel, "dry_run": opts.DryRun, "fingerprint": fp},
	}

	if smokeOK {
		res.Stages[StageSmoke] = StageResult{Status: StageOK, Detail: smokeDetail}
		res.Status = ResultSuccess
	} else {
		res.Stages[StageSmoke] = StageResult{Status: StageFailed, Detail: smokeDetail}
		res.Status = ResultFailed
	}

	if opts.Logger != nil {
		logging.Fluent(opts.Logger).Info("agent-onboard finished").
			String("status", res.Status).
			String("vector", res.Vector).
			Log()
	}

	res.NextSteps = nextSteps(res)
	return res, nil
}

func ensureAgentsMD(vendors []Vendor) []Vendor {
	for _, v := range vendors {
		if v.ID == VendorAgentsMD {
			return vendors
		}
	}
	for _, v := range KnownVendors() {
		if v.ID == VendorAgentsMD {
			return append([]Vendor{v}, vendors...)
		}
	}
	return vendors
}

func vendorIDs(vendors []Vendor) []string {
	ids := make([]string, 0, len(vendors))
	for _, v := range vendors {
		ids = append(ids, string(v.ID))
	}
	return ids
}

func headlessVendorsOnly(vendors []Vendor) []Vendor {
	var out []Vendor
	for _, v := range vendors {
		if v.ID == VendorAgentsMD {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		for _, v := range KnownVendors() {
			if v.ID == VendorAgentsMD {
				return []Vendor{v}
			}
		}
	}
	return out
}

func smoke(projectRoot string, dryRun, skipPrime bool, primed, skipped []string) (ok bool, detail map[string]any) {
	detail = map[string]any{}
	checks := map[string]bool{}

	if dryRun {
		checks["sync_report_would_exist"] = true
		if !skipPrime {
			checks["directive_would_exist"] = len(primed)+len(skipped) > 0
		}
		detail["checks"] = checks
		detail["dry_run"] = true
		return true, detail
	}

	if !skipPrime {
		agentsMD := filepath.Join(projectRoot, ".agents", "AGENTS.md")
		okDirective := pathExists(agentsMD) || len(primed) > 0 || len(skipped) > 0
		checks["directive"] = okDirective
		detail["checks"] = checks
		detail[objects.FieldKeyNote] = "Smoke verifies regenerable directives; sync report write is gated after smoke"
		return okDirective, detail
	}
	detail["checks"] = checks
	detail[objects.FieldKeyNote] = "prime skipped; seating verified at seat stage"
	return true, detail
}

func nextSteps(res *Result) []string {
	exe := brand.ExecutableName()
	steps := []string{
		exe + " workflow whats-next --format json",
	}
	if res.Vector == "B" {
		steps = append(steps,
			"docs/onboarding/EDGE_HEADLESS_FIRST_RUN.md",
			"docs/strategy/open-core/SKU_ONBOARDING_SURFACES.md",
		)
	} else {
		steps = append(steps,
			"docs/onboarding/COMMUNITY_FIRST_RUN.md",
			"Connect MCP: "+exe+" mcp serve (see start-here)",
		)
	}
	if res.Status != ResultSuccess {
		steps = append([]string{"Re-run: " + exe + " system agent-onboard"}, steps...)
	}
	return steps
}
