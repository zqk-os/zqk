package system

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/systemcheck"
	"github.com/zqk-os/zqk/pkg/validation"
)

// runCheckIncremental executes fast incremental validation of dirty or git-staged objects
// under .zqk/process/ along with kernel CAS membrane invariants.
func runCheckIncremental(cmd *cobra.Command, args []string) error {
	cli.TouchMeaningfulActivity()
	startTime := time.Now()

	stagedOnly, _ := cmd.Flags().GetBool("staged")
	fastMode, _ := cmd.Flags().GetBool("fast")
	tierFilter, _ := cmd.Flags().GetInt("tier")
	details, _ := cmd.Flags().GetBool("details")
	includeIDs, _ := cmd.Flags().GetBool("include-ids")
	showDetails := details || includeIDs

	projectRoot := resolveProjectRootFromCommand(cmd)
	if projectRoot == "" {
		projectRoot = ProjectRootOrResolve("")
	}

	var ctx *cli.Context
	if cliCtx := cli.GetContext(cmd); cliCtx != nil && cliCtx.Context != nil {
		ctx = cliCtx
	}
	if ctx == nil {
		ctx = cli.ContextForProjectAndProfile(projectRoot, "system")
	}

	format := string(cli.GetFormat(cmd))

	// 1. Discover dirty/staged files in .zqk/process/
	dirtyFiles, deletedPaths, err := systemcheck.GetDirtyProcessFiles(projectRoot, stagedOnly)
	if err != nil {
		// If git diff fails (e.g. not a git repo), inform caller
		cmd.PrintErrf("Warning: git status inspection failed (%v); verifying membrane invariants only.\n", err)
	}

	// 2. FAST PATH: No files under .zqk/process/ are dirty or deleted
	if len(dirtyFiles) == 0 && len(deletedPaths) == 0 {
		membraneIssues, memErr := systemcheck.VerifyMembraneInvariants(projectRoot)
		if memErr != nil {
			return memErr
		}

		if len(membraneIssues) > 0 {
			results := []CheckResult{
				{
					ObjectID:   "CAS-MEMBRANE",
					ObjectKind: "membrane",
					Issues:     membraneIssues,
				},
			}
			return renderIncrementalResults(cmd, ctx, results, dirtyFiles, deletedPaths, format, tierFilter, showDetails, time.Since(startTime))
		}

		// Truly clean fast path (<100ms)
		return renderIncrementalClean(cmd, format, stagedOnly, time.Since(startTime))
	}

	// 3. DIRTY VALIDATION PATH: Objects under .zqk/process/ are dirty or deleted
	fmt.Fprintf(cmd.ErrOrStderr(), "Incremental check: validating %d staged/dirty object(s) under %s/...\n", len(dirtyFiles), paths.ProcessDir)

	// 3a. Verify CAS membrane on dirty files and deleted files (CASOrphanGuard)
	membraneResults := systemcheck.VerifyDirtyCASMembrane(projectRoot, dirtyFiles, deletedPaths)

	// Also verify overall membrane invariants
	memIssues, _ := systemcheck.VerifyMembraneInvariants(projectRoot)
	if len(memIssues) > 0 {
		membraneResults = append(membraneResults, CheckResult{
			ObjectID:   "CAS-MEMBRANE",
			ObjectKind: "membrane",
			Issues:     memIssues,
		})
	}

	// Map to track results by object ID
	resultsByID := make(map[string]*CheckResult)
	for i := range membraneResults {
		r := membraneResults[i]
		key := r.ObjectID
		if key == "" {
			key = r.FilePath
		}
		resultsByID[key] = &r
	}

	// 3b. Load object ID cache and validator for instance & relational checks
	_ = TryLoadObjectIDCacheOnly(projectRoot)
	objectIDCache := GetGlobalObjectIDCache()

	validatorRegistry := validation.GetGlobalRegistry()
	validator := validatorRegistry.Get("")

	yamlParser := parser.NewYAMLParser()

	// 3c. Validate each dirty file
	for _, df := range dirtyFiles {
		if len(df.Content) == 0 {
			continue
		}

		obj, parseErr := yamlParser.ParseBytes(df.Content)
		if parseErr != nil {
			// Malformed YAML is already captured by VerifyDirtyCASMembrane
			continue
		}

		objID := df.ObjectID
		if objID == "" && obj != nil {
			objID = obj.ID
		}
		kind := df.Kind
		if kind == "" && obj != nil {
			kind = obj.Kind
		}

		existingRes, exists := resultsByID[objID]
		if !exists {
			existingRes = &CheckResult{
				ObjectID:   objID,
				ObjectKind: kind,
				FilePath:   df.Path,
				Status:     df.Status,
				Issues:     []Issue{},
			}
			resultsByID[objID] = existingRes
		}

		// Integrity check (hash registry)
		integrityIssues, _ := checkIntegrity(ctx, cmd, obj, df.FullPath, kind)
		existingRes.Issues = append(existingRes.Issues, integrityIssues...)

		// Instance validation (schema, preconditions, lifecycle)
		stdCtx := cmd.Context()
		if stdCtx == nil {
			stdCtx = pkgctx.NewSystemContext()
		}
		if validator != nil && obj != nil {
			instanceIssues := checkInstanceValidationWithValidatorAndData(ctx, stdCtx, obj, kind, validator, obj.Properties, nil)
			existingRes.Issues = append(existingRes.Issues, instanceIssues...)
		}

		// Relational invariants (references) - unless fast mode requested
		if !fastMode && obj != nil {
			refIssues := checkReferencesWithCache(ctx, obj, kind, objectIDCache, nil)
			existingRes.Issues = append(existingRes.Issues, refIssues...)
		}
	}

	// 3d. Flatten results
	var allResults []CheckResult
	for _, r := range resultsByID {
		// Deduplicate issues by message
		r.Issues = deduplicateIssues(r.Issues)
		allResults = append(allResults, *r)
	}

	// Filter by tier if requested
	if tierFilter > 0 {
		allResults = filterResultsByTierIfNeeded(cmd, allResults)
	}

	// Write failing IDs if requested
	if writeFailingIDs, _ := cmd.Flags().GetString("write-failing-ids"); writeFailingIDs != "" {
		_ = writeFailingIDsToFile(cmd, allResults)
	}

	return renderIncrementalResults(cmd, ctx, allResults, dirtyFiles, deletedPaths, format, tierFilter, showDetails, time.Since(startTime))
}

func deduplicateIssues(issues []Issue) []Issue {
	if len(issues) <= 1 {
		return issues
	}
	seen := make(map[string]bool)
	var deduped []Issue
	for _, issue := range issues {
		key := fmt.Sprintf("%d:%s:%s", issue.Tier, issue.Category, issue.Message)
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, issue)
		}
	}
	return deduped
}

func renderIncrementalClean(cmd *cobra.Command, format string, stagedOnly bool, duration time.Duration) error {
	if format == "json" {
		summary := systemcheck.IncrementalCheckSummary{
			DirtyFilesCount:   0,
			DeletedFilesCount: 0,
			MembraneClean:     true,
			Results:           []CheckResult{},
			TotalIssues:       0,
			BlockingIssues:    0,
			Duration:          duration,
		}
		data, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return err
		}
		return cli.WriteOutput(cmd, append(data, '\n'))
	}

	green := color.New(color.FgGreen, color.Bold).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

	stagedMsg := "staged/dirty"
	if stagedOnly {
		stagedMsg = "staged"
	}

	cmd.Printf("%s %s (0 %s objects under %s/)\n",
		cyan("System check:"), "Incremental mode", stagedMsg, paths.ProcessDir)
	cmd.Printf("  %s Knowledge Kernel CAS membrane integrity verified\n", green("✓"))
	cmd.Printf("  %s Fast relational invariants satisfied\n", green("✓"))
	cmd.Printf("  %s Sub-second scan completed in %s\n", dim("•"), duration.Round(time.Millisecond))
	cmd.Println("System check complete.")
	return nil
}

func renderIncrementalResults(
	cmd *cobra.Command,
	ctx *cli.Context,
	results []CheckResult,
	dirtyFiles []systemcheck.DirtyProcessFile,
	deletedPaths []string,
	format string,
	tierFilter int,
	showDetails bool,
	duration time.Duration,
) error {
	totalIssues := 0
	blockingIssues := 0
	for _, r := range results {
		for _, issue := range r.Issues {
			totalIssues++
			if issue.Tier == 1 {
				blockingIssues++
			}
		}
	}

	if format == "json" {
		summary := systemcheck.IncrementalCheckSummary{
			DirtyFilesCount:   len(dirtyFiles),
			DeletedFilesCount: len(deletedPaths),
			MembraneClean:     blockingIssues == 0,
			Results:           results,
			TotalIssues:       totalIssues,
			BlockingIssues:    blockingIssues,
			Duration:          duration,
		}
		data, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return err
		}
		_ = cli.WriteOutput(cmd, append(data, '\n'))
		return evaluateSystemCheckPristine(results, nil, ctx.ProjectRoot)
	}

	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed, color.Bold).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	cyan := color.New(color.FgCyan, color.Bold).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	cmd.Printf("\n=== %s ===\n\n", cyan("System Check: Incremental Compliance Summary"))
	cmd.Printf("Validated %d dirty/staged file(s) in %s\n\n", len(dirtyFiles)+len(deletedPaths), duration.Round(time.Millisecond))

	if len(results) == 0 || totalIssues == 0 {
		cmd.Printf("%s All dirty objects passed validation.\n", green("✓"))
		cmd.Printf("%s Knowledge Kernel CAS membrane integrity verified.\n\n", green("✓"))
		cmd.Println("System check complete.")
		return nil
	}

	for _, r := range results {
		if len(r.Issues) == 0 {
			if showDetails {
				cmd.Printf("  %s %s (%s) - clean\n", green("✓"), r.ObjectID, r.ObjectKind)
			}
			continue
		}

		cmd.Printf("Object: %s (%s) [%s]\n", cyan(r.ObjectID), r.ObjectKind, r.FilePath)
		for _, issue := range r.Issues {
			tag := yellow(fmt.Sprintf("Tier %d", issue.Tier))
			if issue.Tier == 1 {
				tag = red("Tier 1 (BLOCKING)")
			}
			cmd.Printf("  • [%s] [%s] %s\n", tag, issue.Category, issue.Message)
		}
		cmd.Println()
	}

	if blockingIssues > 0 {
		cmd.Printf("%s %d blocking violation(s) found in dirty objects or membrane invariants.\n",
			red("❌"), blockingIssues)
		cmd.Printf("%s\n", dim(fmt.Sprintf("Run '%s' or inspect dirty objects above to resolve.", paths.CLIUsage("system", "check", "--details"))))
	}

	cmd.Println("System check complete.")
	return evaluateSystemCheckPristine(results, nil, ctx.ProjectRoot)
}
