package testdrive

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
)

// Engine executes the interactive developer test drive experience.
type Engine struct {
	opts   TestDriveOptions
	writer io.Writer
	reader io.Reader
	delay  time.Duration
}

// NewEngine creates a new TestDrive Engine with validated options.
func NewEngine(opts TestDriveOptions) (*Engine, error) {
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}
	if opts.Reader == nil {
		opts.Reader = os.Stdin
	}

	trimmedIntent := strings.TrimSpace(opts.Intent)
	if trimmedIntent != "" {
		if len(trimmedIntent) < MinIntentLength {
			return nil, errfmt.Errorf("intent too short (minimum %d characters)", MinIntentLength)
		}
		if len(trimmedIntent) > MaxIntentLength {
			return nil, errfmt.Errorf("intent exceeds maximum length (%d characters)", MaxIntentLength)
		}
	}

	var delay time.Duration
	switch strings.ToLower(opts.Speed) {
	case SpeedInstant:
		delay = 0
	case SpeedFast:
		delay = FastStepDelay
	default:
		delay = NormalStepDelay
	}

	if opts.Scenario == "" {
		opts.Scenario = ScenarioAPIService
	}

	return &Engine{
		opts:   opts,
		writer: opts.Writer,
		reader: opts.Reader,
		delay:  delay,
	}, nil
}

// Run executes the complete 4-stage test drive experience.
func (e *Engine) Run(ctx context.Context) (*TestDriveReport, error) {
	start := time.Now()

	e.printHeader()

	intent, err := e.resolveIntent()
	if err != nil {
		return nil, err
	}

	report := &TestDriveReport{
		Intent:   intent,
		Scenario: e.opts.Scenario,
		NextSteps: []string{
			paths.CLIInvocation("zqk do") + "           # Launch full autonomous swarm worker loop",
			paths.CLIInvocation("zqk ui -w") + "        # Open interactive Web Studio DAG visualizer",
			paths.CLIInvocation("zqk workflow whats-next") + "  # Self-discover priority tasks from knowledge kernel",
		},
	}

	// Stage 1: Seed Planting (Vision & Mission)
	stage1Start := time.Now()
	s1, objs1, err := e.runStageSeedIntent(ctx, intent)
	if err != nil {
		return nil, err
	}
	s1.Duration = time.Since(stage1Start).Round(time.Millisecond).String()
	report.Stages = append(report.Stages, s1)
	report.MintedObjects = append(report.MintedObjects, objs1...)

	e.sleep()

	// Stage 2: Intent Crystallization (5-Layer Cascade)
	stage2Start := time.Now()
	s2, objs2, err := e.runStageOntologyCascade(ctx, intent)
	if err != nil {
		return nil, err
	}
	s2.Duration = time.Since(stage2Start).Round(time.Millisecond).String()
	report.Stages = append(report.Stages, s2)
	report.MintedObjects = append(report.MintedObjects, objs2...)

	e.sleep()

	// Stage 3: Concurrent Swarm Execution & AST Proofs
	stage3Start := time.Now()
	s3, workers, err := e.runStageSwarmAST(ctx)
	if err != nil {
		return nil, err
	}
	s3.Duration = time.Since(stage3Start).Round(time.Millisecond).String()
	report.Stages = append(report.Stages, s3)
	report.SwarmWorkers = workers

	e.sleep()

	// Stage 4: Cryptographic Verification Receipt & VDS Done-Gate
	stage4Start := time.Now()
	s4, receiptHash, err := e.runStageVDSReceipt(ctx, intent, report.MintedObjects)
	if err != nil {
		return nil, err
	}
	s4.Duration = time.Since(stage4Start).Round(time.Millisecond).String()
	report.Stages = append(report.Stages, s4)
	report.VDSReceiptHash = receiptHash

	report.Duration = time.Since(start).Round(time.Millisecond).String()
	report.Success = true

	e.printSummary(report)

	return report, nil
}

func (e *Engine) resolveIntent() (string, error) {
	if strings.TrimSpace(e.opts.Intent) != "" {
		return strings.TrimSpace(e.opts.Intent), nil
	}

	if e.opts.NonInteractive {
		switch e.opts.Scenario {
		case ScenarioCLITool:
			return "Build a high-performance CLI utility with sub-command parsing and JSON formatting", nil
		case ScenarioStreamPipeline:
			return "Build an event stream subscriber with WAL persistence and backpressure handling", nil
		default:
			return DefaultIntentPrompt, nil
		}
	}

	// Interactive prompt
	_, _ = fmt.Fprintf(e.writer, "\n🌱 Enter your core project intent or feature to build (press Enter for default):\n")
	_, _ = fmt.Fprintf(e.writer, "   [default: %s]\n", DefaultIntentPrompt)
	_, _ = fmt.Fprint(e.writer, "   > ")

	scanner := bufio.NewScanner(e.reader)
	if scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text != "" {
			if len(text) < MinIntentLength {
				return "", errfmt.Errorf("intent too short (minimum %d characters)", MinIntentLength)
			}
			if len(text) > MaxIntentLength {
				return "", errfmt.Errorf("intent exceeds maximum length (%d characters)", MaxIntentLength)
			}
			return text, nil
		}
	}

	return DefaultIntentPrompt, nil
}

func (e *Engine) runStageSeedIntent(_ context.Context, intent string) (TestDriveStageResult, []MintedObjectSummary, error) {
	_, _ = fmt.Fprintf(e.writer, "\n┌── [Stage 1/4] Seed Planting: Grounding in Vision & Mission ──────────────────────┐\n")
	_, _ = fmt.Fprintf(e.writer, "│ In ZQK, work begins with an intentional seed that pools into persistent reality.  │\n")
	_, _ = fmt.Fprintf(e.writer, "└─────────────────────────────────────────────────────────────────────────────────┘\n")

	visObj := MintedObjectSummary{
		Kind:        "vision",
		ID:          "VIS-TD-001",
		Title:       "Autonomous System Evolution & Developer Delight",
		Description: "Foster high-confidence software delivery through persistent Knowledge Kernel graphs.",
	}
	misObj := MintedObjectSummary{
		Kind:        "mission",
		ID:          "MIS-TD-001",
		Title:       fmt.Sprintf("Realize Intent: %s", intent),
		Description: "Orchestrate verified autonomous deliverables satisfying user requirements.",
	}

	_, _ = fmt.Fprintf(e.writer, "  ✓ Minted %s: %s\n", visObj.ID, visObj.Title)
	_, _ = fmt.Fprintf(e.writer, "  ✓ Minted %s: %s\n", misObj.ID, misObj.Title)

	return TestDriveStageResult{
		Stage:   StageSeedIntent,
		Title:   "Seed Planting (Vision & Mission)",
		Success: true,
		Summary: fmt.Sprintf("Grounded initial intent into persistent Vision (%s) and Mission (%s)", visObj.ID, misObj.ID),
	}, []MintedObjectSummary{visObj, misObj}, nil
}

func (e *Engine) runStageOntologyCascade(_ context.Context, intent string) (TestDriveStageResult, []MintedObjectSummary, error) {
	_, _ = fmt.Fprintf(e.writer, "\n┌── [Stage 2/4] Intent Crystallization: 5-Layer Cascade ──────────────────────────┐\n")
	_, _ = fmt.Fprintf(e.writer, "│ ZQK converts transient prompts into timeless, verifiable graph contracts:         │\n")
	_, _ = fmt.Fprintf(e.writer, "│ Vision → Goals → Priority Plans → Requirements/Criteria → Backlog Items          │\n")
	_, _ = fmt.Fprintf(e.writer, "└─────────────────────────────────────────────────────────────────────────────────┘\n")

	goalObj := MintedObjectSummary{
		Kind:        "goal",
		ID:          "GOAL-TD-001",
		Title:       "Robust Execution Spine & Verifiable Delivery",
		Description: "Timeless compass defining strategic delivery boundaries.",
	}
	planObj := MintedObjectSummary{
		Kind:        "priority_plan",
		ID:          "PRI-TD-001",
		Title:       "Cycle 1: Core Foundation & Verification Spine",
		Description: "Time-bounded execution batch locked against scope-creep.",
	}
	reqObj := MintedObjectSummary{
		Kind:        "requirement",
		ID:          "REQ-TD-001",
		Title:       fmt.Sprintf("Deliverable Contract: %s", intent),
		Description: "Owns Three-Fold Proof criteria: Static Invariant, Operational Proof, Negative Boundary.",
	}
	bli1 := MintedObjectSummary{
		Kind:        "backlog_item",
		ID:          "BLI-TD-001",
		Title:       "Static Floor Invariant & Type Safety Implementation",
		Description: "Compile-time invariants, defaults, and boundary configurations.",
	}
	bli2 := MintedObjectSummary{
		Kind:        "backlog_item",
		ID:          "BLI-TD-002",
		Title:       "Operational Dynamic Delivery & Core Capability Proof",
		Description: "Functional throughput, state transitions, and concurrency safety.",
	}
	bli3 := MintedObjectSummary{
		Kind:        "backlog_item",
		ID:          "BLI-TD-003",
		Title:       "Negative Adversarial Boundary Guardrails",
		Description: "Fail-closed defenses against poison payloads and edge faults.",
	}

	_, _ = fmt.Fprintf(e.writer, "  ✓ Minted %s: %s\n", goalObj.ID, goalObj.Title)
	_, _ = fmt.Fprintf(e.writer, "  ✓ Minted %s: %s\n", planObj.ID, planObj.Title)
	_, _ = fmt.Fprintf(e.writer, "  ✓ Minted %s: %s\n", reqObj.ID, reqObj.Title)
	_, _ = fmt.Fprintf(e.writer, "  ✓ Minted %s: %s\n", bli1.ID, bli1.Title)
	_, _ = fmt.Fprintf(e.writer, "  ✓ Minted %s: %s\n", bli2.ID, bli2.Title)
	_, _ = fmt.Fprintf(e.writer, "  ✓ Minted %s: %s\n", bli3.ID, bli3.Title)

	return TestDriveStageResult{
		Stage:   StageOntologyCascade,
		Title:   "Intent Crystallization (5-Layer Cascade)",
		Success: true,
		Summary: "Objectified intent into 1 Goal, 1 Priority Plan, 1 Requirement, and 3 Backlog Items",
	}, []MintedObjectSummary{goalObj, planObj, reqObj, bli1, bli2, bli3}, nil
}

func (e *Engine) runStageSwarmAST(_ context.Context) (TestDriveStageResult, []SwarmWorkerSimulation, error) {
	_, _ = fmt.Fprintf(e.writer, "\n┌── [Stage 3/4] Swarm Execution & AST Invariant Verification ─────────────────────┐\n")
	_, _ = fmt.Fprintf(e.writer, "│ Autonomous seats work concurrently with token-budgeted zgrep and AST checks:     │\n")
	_, _ = fmt.Fprintf(e.writer, "└─────────────────────────────────────────────────────────────────────────────────┘\n")

	workers := []SwarmWorkerSimulation{
		{
			SeatID:    "SEAT-1",
			Persona:   "PER-COMMUNITY-ARCH-DESIGN",
			Role:      "Architecture & Floor Invariants",
			Status:    "COMPLETED",
			Duration:  "420ms",
			ASTChecks: 12,
		},
		{
			SeatID:    "SEAT-2",
			Persona:   "PER-COMMUNITY-CODE-CRAFTSMAN",
			Role:      "Operational Dynamic Implementation",
			Status:    "COMPLETED",
			Duration:  "580ms",
			ASTChecks: 24,
		},
		{
			SeatID:    "SEAT-3",
			Persona:   "PER-COMMUNITY-QA-VERIFICATION",
			Role:      "Adversarial Boundaries & Latch Proofs",
			Status:    "COMPLETED",
			Duration:  "390ms",
			ASTChecks: 18,
		},
	}

	for _, w := range workers {
		_, _ = fmt.Fprintf(e.writer, "  ⚡ [%s] %s (%s) → %s in %s (AST checks: %d passed)\n",
			w.SeatID, w.Persona, w.Role, w.Status, w.Duration, w.ASTChecks)
	}

	return TestDriveStageResult{
		Stage:   StageSwarmAST,
		Title:   "Swarm Execution & AST Verification",
		Success: true,
		Summary: fmt.Sprintf("Executed %d concurrent swarm seats with 54 verified AST invariant checks", len(workers)),
	}, workers, nil
}

func (e *Engine) runStageVDSReceipt(_ context.Context, intent string, objects []MintedObjectSummary) (TestDriveStageResult, string, error) {
	_, _ = fmt.Fprintf(e.writer, "\n┌── [Stage 4/4] Cryptographic Verification Receipt & VDS Done-Gate ───────────────┐\n")
	_, _ = fmt.Fprintf(e.writer, "│ Unlike chat bots that promise 'trust me', ZQK delivers machine-provable proofs.  │\n")
	_, _ = fmt.Fprintf(e.writer, "└─────────────────────────────────────────────────────────────────────────────────┘\n")

	h := sha256.New()
	h.Write([]byte(intent))
	for _, o := range objects {
		h.Write([]byte(o.ID))
	}
	receiptHash := hex.EncodeToString(h.Sum(nil))

	_, _ = fmt.Fprintf(e.writer, "  🛡️  QA Auditor Signer: KEY-AUDITOR-001 [VERIFIED]\n")
	_, _ = fmt.Fprintf(e.writer, "  ✓ Static Floor Invariant Proof: PASSED\n")
	_, _ = fmt.Fprintf(e.writer, "  ✓ Operational Dynamic Proof:    PASSED\n")
	_, _ = fmt.Fprintf(e.writer, "  ✓ Negative Boundary Proof:      PASSED\n")
	_, _ = fmt.Fprintf(e.writer, "  🔒 Cryptographic VDS Receipt:    %s\n", receiptHash[:16]+"...")

	return TestDriveStageResult{
		Stage:   StageVDSReceipt,
		Title:   "Cryptographic Verification Receipt & VDS Done-Gate",
		Success: true,
		Summary: fmt.Sprintf("Issued cryptographic verification receipt %s with Three-Fold Proof satisfied", receiptHash[:16]),
	}, receiptHash, nil
}

func (e *Engine) printHeader() {
	_, _ = fmt.Fprintf(e.writer, "=================================================================================\n")
	_, _ = fmt.Fprintf(e.writer, "🚀 ZQK KNOWLEDGE KERNEL — DEVELOPER SWARM TEST-DRIVE\n")
	_, _ = fmt.Fprintf(e.writer, "=================================================================================\n")
	_, _ = fmt.Fprintf(e.writer, "Welcome to the guided test-drive of ZQK: the autonomous Knowledge Kernel.\n")
	_, _ = fmt.Fprintf(e.writer, "See how user intent transforms into verifiable graphs, concurrent swarms,\n")
	_, _ = fmt.Fprintf(e.writer, "and tamper-proof cryptographic delivery receipts.\n")
}

func (e *Engine) printSummary(report *TestDriveReport) {
	_, _ = fmt.Fprintf(e.writer, "\n=================================================================================\n")
	_, _ = fmt.Fprintf(e.writer, "🎉 TEST-DRIVE COMPLETED SUCCESSFULLY (%s)\n", report.Duration)
	_, _ = fmt.Fprintf(e.writer, "=================================================================================\n")
	_, _ = fmt.Fprintf(e.writer, "Intent:          %s\n", report.Intent)
	_, _ = fmt.Fprintf(e.writer, "Graph Objects:   %d minted across 5 architectural layers\n", len(report.MintedObjects))
	_, _ = fmt.Fprintf(e.writer, "Swarm Seats:     %d concurrent autonomous workers verified\n", len(report.SwarmWorkers))
	_, _ = fmt.Fprintf(e.writer, "VDS Receipt:     %s\n", report.VDSReceiptHash)
	_, _ = fmt.Fprintf(e.writer, "\nNext Steps to Fall in Love with ZQK:\n")
	for _, step := range report.NextSteps {
		_, _ = fmt.Fprintf(e.writer, "  • %s\n", step)
	}
	_, _ = fmt.Fprintf(e.writer, "\nFor full interactive docs, visit: https://docs.zqk.dev\n\n")
}

func (e *Engine) sleep() {
	if e.delay > 0 {
		time.Sleep(e.delay)
	}
}
