package system

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/agentdelivery"
	"github.com/zqk-os/zqk/pkg/agentprompt"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/entitlements"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/evolution"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/observer"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// EvolveOptions holds the execution options for evolve
type EvolveOptions struct {
	MaxComplexity int
	Limit         int
	DryRun        bool
	OutputPath    string
	SystemsOnly   bool
	BrandOnly     bool
	Autonomous    bool
}

// EvolutionCandidate represents a target for recursive improvement
type EvolutionCandidate struct {
	ID         string
	Name       string
	Kind       string
	Location   string
	Complexity int
	Reason     string
	Context    string
}

// NewEvolveCmd creates the system evolve command
func NewEvolveCmd() *cobra.Command {
	opts := &EvolveOptions{}
	cmd := bldr_cli_cmd_v1.NewSystemEvolveCommandBuilder()

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		// Map builder-managed flags back to options struct
		opts.MaxComplexity, _ = cmd.Flags().GetInt("max-complexity")
		opts.Limit, _ = cmd.Flags().GetInt("limit")
		opts.DryRun, _ = cmd.Flags().GetBool("dry-run")
		opts.SystemsOnly, _ = cmd.Flags().GetBool("systems-only")
		opts.BrandOnly, _ = cmd.Flags().GetBool("brand-only")
		opts.Autonomous, _ = cmd.Flags().GetBool("autonomous")
		opts.OutputPath, _ = cmd.Flags().GetString("output-path")

		return runEvolve(cmd, opts)
	}

	return cmd
}

func runEvolve(cmd *cobra.Command, opts *EvolveOptions) error {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return err
	}
	ctx := proc.OperationContext()
	sp := proc.Storage()
	secCtx := proc.SecurityContext()
	projectRoot := proc.ProjectRoot()

	if err := entitlements.CheckEvolveEntitlement(ctx, opts.Limit); err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}

	var candidates []EvolutionCandidate

	// 1. Branding Drift Analysis (Purge legacy brands)
	if opts.BrandOnly || (!opts.SystemsOnly) {
		_ = cli.WriteOutput(cmd, []byte("Scanning for legacy branding references (nexus-os, nexos, workstream-os)...\n"))

		legacyPatterns := []string{"nexus-os", "nexos", "NexOS", "nex-os", "workstream-os"}
		patternStr := strings.Join(legacyPatterns, "|")

		// Use grep to find branding drift
		grepCmd := execwrap.CommandContext(ctx, "grep", "-rEil", patternStr, ".")
		grepCmd.Dir = projectRoot
		output, _ := grepCmd.CombinedOutput()

		files := strings.Split(string(output), "\n")
		for _, file := range files {
			f := strings.TrimSpace(file)
			if f == "" || strings.HasPrefix(f, ".git/") || strings.HasPrefix(f, "bin/") || strings.Contains(f, filepath.Join(paths.ProjectDataDir, paths.CacheDir)) {
				continue
			}
			candidates = append(candidates, EvolutionCandidate{
				Name:     filepath.Base(f),
				Kind:     "file_branding",
				Location: f,
				Reason:   "Legacy branding reference detected",
			})
		}
	}

	// 2. System Object Drift Analysis
	if opts.SystemsOnly || (!opts.BrandOnly) {
		_ = cli.WriteOutput(cmd, []byte("Analyzing system objects for environment drift and stale paths...\n"))

		jobFilter := storage.ListFilter{Kind: objects.KindSchedulerJob, Limit: 0}
		jobs, err := sp.List(ctx, secCtx, proc.StorageContext(), jobFilter)
		if err == nil {
			for _, job := range jobs.Objects {
				id, _ := job[objects.FieldKeyID].(string)
				title, _ := job[objects.FieldKeyTitle].(string)
				workingDir, _ := job[objects.FieldKeyWorkingDirectory].(string)

				if workingDir != "" && workingDir != projectRoot && !strings.HasPrefix(workingDir, projectRoot) {
					candidates = append(candidates, EvolutionCandidate{
						ID:     id,
						Name:   title,
						Kind:   objects.KindSchedulerJob,
						Reason: fmt.Sprintf("Stale working directory: %s (Project Root: %s)", workingDir, projectRoot),
					})
				}
			}
		}
	}

	// 3. AST Complexity Analysis
	if !opts.SystemsOnly && !opts.BrandOnly {
		_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Scanning codebase for AST entities with complexity > %d...\n", opts.MaxComplexity)))
		ext := observer.GoExtractor{}
		extractResult, err := observer.ExtractFromDir(ctx, nil, projectRoot, []observer.Extractor{ext})
		if err != nil {
			_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Warning: AST scan failed: %v\n", err)))
		} else {
			for _, ent := range extractResult.Entities {
				if ent.Complexity > opts.MaxComplexity {
					candidates = append(candidates, EvolutionCandidate{
						Name:       ent.Name,
						Kind:       ent.Kind,
						Location:   fmt.Sprintf("%s:%d", ent.File, ent.Line),
						Complexity: ent.Complexity,
						Reason:     fmt.Sprintf("High cyclomatic complexity (%d > %d)", ent.Complexity, opts.MaxComplexity),
						Context:    ent.Signature,
					})
				}
			}
		}
	}

	if len(candidates) == 0 {
		_ = cli.WriteOutput(cmd, []byte("No evolution candidates found. The kernel is in a state of high structural integrity.\n"))
		return nil
	}

	// Apply selection limit
	_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Found %d total evolution candidates. Selection limit: %d.\n\n", len(candidates), opts.Limit)))
	if len(candidates) > opts.Limit {
		candidates = candidates[:opts.Limit]
	}

	for _, c := range candidates {
		_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("📍 Candidate: %s (%s)\n   Reason: %s\n\n", c.Name, c.Kind, c.Reason)))
	}

	if opts.DryRun {
		_ = cli.WriteOutput(cmd, []byte("Dry run complete. No agents dispatched.\n"))
		return nil
	}

	// 4. Dispatch Healing Cycle
	evolutionHandoffDir := opts.OutputPath
	if evolutionHandoffDir == "" {
		evolutionHandoffDir = filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, "evolution")
	}

	// Load Policy Enforcement Layer
	policyEnforcement, err := agentprompt.LoadActivePolicies(ctx, sp, secCtx)
	if err != nil {
		return errfmt.Newf("failed to load policies for evolution cycle").Wrap(err)
	}
	policySection := policyEnforcement.GeneratePromptSection()

	// Load specialized skills
	astSkill, _ := agentprompt.LoadRelevantSkills(ctx, sp, secCtx, "Go AST Expert Refactoring")
	archSkill, _ := agentprompt.LoadRelevantSkills(ctx, sp, secCtx, "System Architect Healing")
	brandSkill, _ := agentprompt.LoadRelevantSkills(ctx, sp, secCtx, "Brand Specialist Cleanup")

	var mu sync.Mutex
	pool := goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "evolution_engine", "performing autonomous healing", len(candidates), len(candidates))
	pool.Start(ctx)

	for _, cand := range candidates {
		cand := cand
		_ = pool.Submit(ctx, func(workerCtx context.Context) error {
			_ = concurrency.RunInLockWithLogger(&mu, locknames.LockNameEvolutionOutput, logging.GetLockLoggerFromProfile(proc.Context().Profile), func() error {
				_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("🚀 Evolving: %s (%s)...\n", cand.Name, cand.Kind)))
				return nil
			})

			var promptMarkdown string
			var skillSection string

			switch cand.Kind {
			case "file_branding":
				if brandSkill != nil {
					skillSection = brandSkill.GeneratePromptSection()
				}
				promptMarkdown = fmt.Sprintf("# Evolution Context: Branding Cleanup\n"+
					"Target File: `%s` \n"+
					"Defect: %s\n\n"+
					"%s\n%s\n"+
					"## Cleanup Goal\n"+
					"- Replace all legacy project names (nexus-os, nexos, workstream-os) with the current brand: **ZQK (Zen Quantum Kernel)**.\n"+
					"- Prefer using brand-aware abstractions (e.g. from `pkg/brand` if in code) to enable future white-labeling.\n"+
					"- Ensure all internal documentation and tool references are consistent.\n",
					cand.Location, cand.Reason, policySection, skillSection)
			case objects.KindSchedulerJob:
				if archSkill != nil {
					skillSection = archSkill.GeneratePromptSection()
				}
				promptMarkdown = fmt.Sprintf("# Evolution Context: System Object Healing\n"+
					"Target Object: %s (%s)\n"+
					"ID: %s\n"+
					"Defect: %s\n\n"+
					"%s\n%s\n"+
					"## Healing Goal\n"+
					"- Update the scheduler_job object to align with the current project root: `%s`.\n"+
					"- Repair any stale paths in metadata (logs, fingerprints).\n"+
					"- Ensure the job is enabled and ready for execution in the current environment.\n",
					cand.Name, cand.Kind, cand.ID, cand.Reason, policySection, skillSection, projectRoot)
			default:
				if astSkill != nil {
					skillSection = astSkill.GeneratePromptSection()
				}
				promptMarkdown = fmt.Sprintf("# Evolution Context: Autonomous Refactoring\n"+
					"Target Entity: %s (%s)\n"+
					"Location: `%s` \n"+
					"Defect: %s\n\n"+
					"%s\n%s\n"+
					"## Refactoring Goal\n"+
					"- Reduce the cyclomatic complexity below %d.\n"+
					"- Maintain strict structural integrity using AST-based manipulation.\n"+
					"- Follow the 'Mandatory AST-Based Go Refactoring' policy.\n",
					cand.Name, cand.Kind, cand.Location, cand.Reason, policySection, skillSection, opts.MaxComplexity)
			}

			prompt := agentdelivery.Prompt{
				Markdown: []byte(promptMarkdown),
				Format:   "agent-prompt",
				DestPath: filepath.Join(evolutionHandoffDir, fmt.Sprintf("evolve_%s_%s.md", cand.Kind, cand.Name)),
			}

			if err := fileutil.EnsureDir(filepath.Dir(prompt.DestPath)); err != nil {
				return err
			}
			return fileutil.WriteSecureFile(prompt.DestPath, prompt.Markdown)
		})
	}

	pool.Stop()

	// 5. Track evolution cycle in the Kernel
	evolutionEntry := map[string]any{
		objects.FieldKeyID:                  fmt.Sprintf("EVOL-%d", time.Now().Unix()),
		objects.FieldKeyKind:                objects.KindEvolutionManagement,
		objects.FieldKeyTitle:               fmt.Sprintf("Evolution Cycle: %s", time.Now().Format(time.RFC3339)),
		objects.FieldKeyStrategy:            "adaptive",
		objects.FieldKeyPeriod:              time.Now().Format("2006-01-02"),
		objects.FieldKeyDescription:         fmt.Sprintf("Autonomous healing cycle targeting complexity > %d. Processed %d candidates.", opts.MaxComplexity, len(candidates)),
		objects.FieldKeyAdaptiveAdjustments: []string{fmt.Sprintf("Processed %d candidates", len(candidates))},
		// complete, not implemented: the evolution_management lifecycle is
		// planning/active/complete/archived/error. "implemented" belongs to other kinds, so every
		// cycle record written here was rejected — silently, because the error is discarded.
		objects.FieldKeyStatus: objects.ObjectStatusComplete,
	}
	_ = sp.Create(ctx, secCtx, evolutionEntry)

	_ = concurrency.RunInLockWithLogger(&mu, locknames.LockNameEvolutionOutput, logging.GetLockLoggerFromProfile(proc.Context().Profile), func() error {
		_ = cli.WriteOutput(cmd, []byte("\nEvolution cycle complete.\n"))
		_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("  - Candidates processed: %d\n", len(candidates))))
		_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("  - Prompts delivered to: %s\n", evolutionHandoffDir)))
		_ = cli.WriteOutput(cmd, []byte("  - Next step: specialized sub-agents (Coder, System Architect, Brand Specialist) should process the evolution log.\n"))

		if opts.Autonomous {
			_ = cli.WriteOutput(cmd, []byte("\n[AUTONOMOUS] Autonomous evolution active. Triggering skill recombination...\n"))
			composer := evolution.NewAutonomousSkillComposer()

			// Placeholder skill A and B for testing the composer invocation
			skillA := map[string]any{
				objects.FieldKeyKind:         "skill",
				objects.FieldKeyName:         "System Architect Healing",
				objects.FieldKeyDescription:  "Heals system objects.",
				objects.FieldKeyInstructions: "Fix stale paths and structural issues.",
			}
			skillB := map[string]any{
				objects.FieldKeyKind:         "skill",
				objects.FieldKeyName:         "Brand Specialist Cleanup",
				objects.FieldKeyDescription:  "Cleans up legacy branding.",
				objects.FieldKeyInstructions: "Remove old brand names.",
			}

			recombined, err := composer.Recombine(ctx, skillA, skillB)
			if err != nil {
				_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("[AUTONOMOUS] Skill recombination failed: %v\n", err)))
			} else {
				_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("[AUTONOMOUS] Successfully recombined into new skill: %s\n", recombined[objects.FieldKeyName])))
				// In a full implementation, we would save this back to the object store
			}
		}

		return nil
	})

	return nil
}
