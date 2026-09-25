package swarm

import (
	"bufio"
	stdcontext "context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/zqk-os/zqk/cmd/zqk/system"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scenario"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/swarm/metabolism"
	"github.com/zqk-os/zqk/pkg/swarm/pack"
	"github.com/zqk-os/zqk/pkg/swarm/remote"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

// NewRunCmd creates a new swarm run command.
func NewRunCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSwarmRunCommandBuilder()
	cmd.Flags().Bool("stage-only", false, "Stage and ingest the swarm package into the kernel without launching execution")
	cmd.Flags().BoolP("yes", "y", false, "Launch the swarm automatically without interactive confirmation")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		entrypoint, _ := cmd.Flags().GetString("entrypoint")
		stageOnly, _ := cmd.Flags().GetBool("stage-only")
		autoLaunch, _ := cmd.Flags().GetBool("yes")
		return runSwarmPackage(cmd, args[0], dryRun, entrypoint, stageOnly, autoLaunch)
	}
	return cmd
}

// NewTopLevelRunCmd creates the root 'zqk run' command.
func NewTopLevelRunCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewRunCommandBuilder()
	cmd.Flags().Bool("stage-only", false, "Stage and ingest the swarm package into the kernel without launching execution")
	cmd.Flags().BoolP("yes", "y", false, "Launch the swarm automatically without interactive confirmation")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		entrypoint, _ := cmd.Flags().GetString("entrypoint")
		stageOnly, _ := cmd.Flags().GetBool("stage-only")
		autoLaunch, _ := cmd.Flags().GetBool("yes")
		return runSwarmPackage(cmd, args[0], dryRun, entrypoint, stageOnly, autoLaunch)
	}
	return cmd
}

func runSwarmPackage(cmd *cobra.Command, targetPath string, dryRun bool, entrypointOverride string, stageOnly bool, autoLaunch bool) error {
	logger := logging.GetLoggerFromContext(cmd.Context())

	manifestPath := targetPath
	if remote.IsRemoteTarget(targetPath) {
		logging.FluentEvent(logger).Info(fmt.Sprintf("Resolving remote swarm package from %s", targetPath)).Log()
		resolved, err := remote.Resolve(cmd.Context(), targetPath, remote.ResolveOptions{})
		if err != nil {
			return errfmt.Newf("failed to resolve remote swarm %s", targetPath).Wrap(err)
		}
		manifestPath = resolved
	} else if st, err := fileutil.Stat(targetPath); err == nil && st.IsDir() {
		manifestPath = filepath.Join(targetPath, "swarm.yaml")
	}

	if !fileutil.Exists(manifestPath) {
		return errfmt.Errorf("swarm package manifest not found: %s", manifestPath)
	}

	pkg, err := pack.LoadManifestFile(manifestPath)
	if err != nil {
		return errfmt.Newf("failed to load swarm package from %s", manifestPath).Wrap(err)
	}

	selectedEntrypoint := pkg.Entrypoint
	if entrypointOverride != "" {
		selectedEntrypoint = entrypointOverride
	}

	// Structured summary
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Swarm Package: %s v%s\n", pkg.Name, pkg.Version)
	fmt.Fprintf(out, "Description:   %s\n", pkg.Description)
	if pkg.License != "" {
		fmt.Fprintf(out, "License:       %s\n", pkg.License)
	}
	if selectedEntrypoint != "" {
		fmt.Fprintf(out, "Entrypoint:    %s\n", selectedEntrypoint)
	}

	fmt.Fprintf(out, "\nConfigured Agents (%d):\n", len(pkg.Agents))
	for _, a := range pkg.Agents {
		fmt.Fprintf(out, "  • %s [role: %s]", a.Name, a.Role)
		if len(a.Skills) > 0 {
			fmt.Fprintf(out, " (skills: %s)", strings.Join(a.Skills, ", "))
		}
		fmt.Fprintln(out)
	}

	if len(pkg.Membranes) > 0 {
		fmt.Fprintf(out, "\nMembrane Boundaries (%d):\n", len(pkg.Membranes))
		for _, m := range pkg.Membranes {
			fmt.Fprintf(out, "  • %s -> %s\n", m.Path, m.Mode)
		}
	}

	if len(pkg.Tasks) > 0 {
		fmt.Fprintf(out, "\nExecution Pipeline (%d tasks):\n", len(pkg.Tasks))
		for _, t := range pkg.Tasks {
			deps := ""
			if len(t.DependsOn) > 0 {
				deps = fmt.Sprintf(" [after: %s]", strings.Join(t.DependsOn, ", "))
			}
			fmt.Fprintf(out, "  • %s: %s%s\n", t.ID, t.Title, deps)
		}
	}

	if dryRun {
		fmt.Fprintf(out, "\n✓ Swarm validation successful (dry-run mode).\n")
		return nil
	}

	logging.FluentEvent(logger).Info(fmt.Sprintf("Launching swarm package %s (entrypoint: %s)", pkg.Name, selectedEntrypoint)).Log()

	// Resolve project root and storage provider
	opCtx := cmd.Context()
	if opCtx == nil {
		opCtx = stdcontext.Background()
	}

	var sp storage.ObjectStorageProvider
	var secCtx *pkgctx.SecurityContext
	projectRoot := ""

	if proc, err := cli.NewProcessor(cmd); err == nil && proc.Storage() != nil {
		sp = proc.Storage()
		projectRoot = proc.ProjectRoot()
		secCtx = proc.SecurityContext()
		if proc.OperationContext() != nil {
			opCtx = proc.OperationContext()
		}
	} else {
		projectRoot = paths.ResolveProjectRoot(".")
		if projectRoot == "" {
			projectRoot = "."
		}
		var err error
		sp, err = storage.GetGlobalStorageProviderCache().GetOrCreate(opCtx, projectRoot)
		if err != nil {
			return errfmt.Newf("failed to initialize storage provider for project %s", projectRoot).Wrap(err)
		}
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	opCtx = pkgctx.WithPromoteOnCreate(opCtx)

	packDir := filepath.Dir(manifestPath)
	params := make(map[string]interface{})
	for k, def := range pkg.Parameters {
		if def.Default != nil {
			params[k] = def.Default
		}
	}

	outputDir := filepath.Join(projectRoot, paths.ProjectDataDir, "runs", fmt.Sprintf("%s-latest", pkg.Name))
	if outVal, ok := params["output_dir"].(string); ok && outVal != "" {
		if filepath.IsAbs(outVal) {
			outputDir = outVal
		} else {
			outputDir = filepath.Join(projectRoot, outVal)
		}
	}
	_ = fileutil.MkdirAll(outputDir, paths.DirPerm755)

	reg := metabolism.NewReceptorRegistry()
	engine := metabolism.NewMetabolismEngine(reg)
	verifySeal := (pkg.Integrity != nil)
	// Ensure default agent seating (personas and skills) is seeded
	sysLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_, _ = system.SeedDefaultAgentSeatingPack(projectRoot, sysLogger)

	// Ensure valid active account for workstream owner_ref
	accountID := secCtx.AccountID
	validAccount := false
	if accountID != "" {
		if obj, err := sp.Read(opCtx, secCtx, accountID); err == nil && obj != nil {
			if st, _ := obj[objects.FieldKeyStatus].(string); st == "" || st == objects.ObjectStatusActive {
				validAccount = true
			}
		}
	}
	if !validAccount {
		accList, err := sp.List(opCtx, secCtx, pkgctx.NewStorageContext(), storage.ListFilter{
			Kind: objects.KindAccount,
			Filters: map[string]any{
				objects.FieldKeyStatus: objects.ObjectStatusActive,
			},
			Limit:  1,
			Fields: []string{objects.FieldKeyID, objects.FieldKeyStatus},
		})
		if err == nil && len(accList.Objects) > 0 {
			if firstID, ok := accList.Objects[0][objects.FieldKeyID].(string); ok && firstID != "" {
				accountID = firstID
				validAccount = true
			}
		}
	}
	if !validAccount {
		if accountID == "" {
			accountID = pkgctx.SystemAccountID
		}
		now := time.Now().UTC().Format(time.RFC3339)
		_ = sp.Create(opCtx, secCtx, map[string]any{
			objects.FieldKeyID:        accountID,
			objects.FieldKeyKind:      objects.KindAccount,
			objects.FieldKeyTitle:     "System Account",
			"username":                "system",
			"roles":                   []string{"admin"},
			objects.FieldKeyStatus:    objects.ObjectStatusActive,
			objects.FieldKeyCreatedAt: now,
			objects.FieldKeyCreatedBy: accountID,
		})
	}

	digest, err := engine.Ingest(metabolism.IngestionOptions{
		PackDir:    packDir,
		OutputDir:  outputDir,
		VerifySeal: verifySeal,
		Parameters: params,
		AccountID:  accountID,
	})
	if err != nil {
		return errfmt.Newf("metabolism ingestion failed for swarm package %s", pkg.Name).Wrap(err)
	}

	// Persist prompt templates into Storage
	templatesStored := 0
	for _, tpl := range digest.PromptTemplates {
		if tpl.ID == "" {
			continue
		}
		tplObj := map[string]any{
			objects.FieldKeyID:          tpl.ID,
			objects.FieldKeyKind:        "prompt_template",
			objects.FieldKeyTitle:       tpl.Title,
			objects.FieldKeyDescription: tpl.Description,
			"category":                  tpl.Category,
			"prompt_archetype":          "structured",
			"prompt_body":               tpl.Template,
			"variables":                 tpl.Variables,
			"metadata":                  tpl.Metadata,
			objects.FieldKeyStatus:      objects.ObjectStatusActive,
		}
		if _, err := sp.Read(opCtx, secCtx, tpl.ID); err == nil {
			continue
		}
		if err := sp.Create(opCtx, secCtx, tplObj); err != nil {
			logging.FluentEvent(logger).Warn(fmt.Sprintf("failed to persist prompt template %s", tpl.ID)).WithError(err).Log()
		} else {
			templatesStored++
		}
	}

	// Persist Synthesized Kernel Objects in Topological Order derived from spec index
	distinctKinds := make([]string, 0)
	kindSeen := make(map[string]bool)
	for _, obj := range digest.KernelObjects {
		if k, _ := obj[objects.FieldKeyKind].(string); k != "" && !kindSeen[k] {
			kindSeen[k] = true
			distinctKinds = append(distinctKinds, k)
		}
	}

	orderedKinds, _, _ := scenario.CreateOrderFromSpecIndex(projectRoot, distinctKinds)
	kindRank := make(map[string]int, len(orderedKinds))
	for rank, k := range orderedKinds {
		kindRank[k] = rank
	}

	sortedObjects := make([]map[string]any, len(digest.KernelObjects))
	copy(sortedObjects, digest.KernelObjects)
	sort.SliceStable(sortedObjects, func(i, j int) bool {
		ki, _ := sortedObjects[i][objects.FieldKeyKind].(string)
		kj, _ := sortedObjects[j][objects.FieldKeyKind].(string)
		ri, oki := kindRank[ki]
		rj, okj := kindRank[kj]
		if oki && okj {
			return ri < rj
		}
		if oki {
			return true
		}
		if okj {
			return false
		}
		return ki < kj
	})

	objectsPersisted := 0
	for _, obj := range sortedObjects {
		id, _ := obj[objects.FieldKeyID].(string)
		kind, _ := obj[objects.FieldKeyKind].(string)
		if id == "" {
			continue
		}
		if _, err := sp.Read(opCtx, secCtx, id); err == nil {
			continue
		}
		if err := sp.Create(opCtx, secCtx, obj); err != nil {
			return errfmt.Newf("failed to persist synthesized %s object %s", kind, id).Wrap(err)
		}
		objectsPersisted++
	}

	// Promote Priority Plan to Active
	cleanName := strings.ToUpper(strings.ReplaceAll(pkg.Name, "-", "_"))
	planID := fmt.Sprintf("PRI-%s", cleanName)

	// Ensure reverse reference index has all child backlog items linked to planID
	revIndex := storage.GetGlobalReverseReferenceIndex()
	for _, obj := range sortedObjects {
		if k, _ := obj[objects.FieldKeyKind].(string); k == objects.KindBacklogItem {
			if bid, _ := obj[objects.FieldKeyID].(string); bid != "" {
				revIndex.AddReference(bid, planID)
			}
		}
	}
	if projectRoot != "" {
		_ = revIndex.SaveCache(projectRoot)
	}

	// Promote Priority Plan to Active if not already active or in_progress
	planObj, planErr := sp.Read(opCtx, secCtx, planID)
	curStatus := ""
	if planErr == nil && planObj != nil {
		curStatus, _ = planObj[objects.FieldKeyStatus].(string)
	}
	if curStatus != objects.ObjectStatusActive && curStatus != objects.ObjectStatusInProgress {
		if err := sp.Update(opCtx, secCtx, planID, map[string]any{
			objects.FieldKeyStatus:      objects.ObjectStatusActive,
			objects.FieldKeyActiveOrder: 1,
		}); err != nil {
			return errfmt.Newf("failed to activate priority plan %s", planID).Wrap(err)
		}
	}

	// Invalidate and refresh materialized view cache so whats-next and scheduler see the plan and tasks immediately
	if projectRoot != "" && sp != nil {
		mv := whatsnext.NewWhatsNextMaterializedView(projectRoot)
		if scanErr := mv.ScanFromStorageWithSecurity(opCtx, sp, secCtx); scanErr == nil {
			_ = mv.SaveToLiteFile()
		}
	}

	// Emit Swarm Launch Steering to Agent Feed
	feedMsg := fmt.Sprintf("SWARM DISPATCH: Initialized swarm package %s v%s. Active Plan: %s with %d tasks. Entrypoint: %s.",
		pkg.Name, pkg.Version, planID, len(pkg.Tasks), selectedEntrypoint)
	_, _ = agentfeed.AppendEvent(agentfeed.AppendEventInput{
		ProjectRoot: projectRoot,
		Message:     feedMsg,
		AgentID:     "system",
		PersonaRef:  "PER-DEFAULT-OPERATOR",
		Sender:      agentfeed.FeedSenderMeshStatus,
		EventType:   agentfeed.FeedEventTypeMeshStatus,
		SelfACK:     true,
	})

	fmt.Fprintf(out, "\n✓ Swarm initialized and dispatch ready for %d agents.\n", len(pkg.Agents))
	fmt.Fprintf(out, "  • Graph Ingested:    %d kernel objects persisted, %d prompt templates stored\n", objectsPersisted, templatesStored)
	fmt.Fprintf(out, "  • Active Plan:       %s\n", planID)
	fmt.Fprintf(out, "  • Output Directory:  %s\n", outputDir)

	if stageOnly {
		fmt.Fprintf(out, "\nSwarm staged (stage-only mode). Launch anytime with:\n  %s\n", paths.CLIInvocation("agent orchestrate "+planID))
		return nil
	}

	launch := autoLaunch
	if !launch {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			fmt.Fprintf(out, "\nLaunch the swarm now? [Y/n]: ")
			reader := bufio.NewReader(os.Stdin)
			ans, err := reader.ReadString('\n')
			if err == nil {
				ans = strings.TrimSpace(strings.ToLower(ans))
				if ans == "" || ans == "y" || ans == "yes" {
					launch = true
				}
			}
		} else {
			// In non-interactive environments without --stage-only, execute by default
			launch = true
		}
	}

	if !launch {
		fmt.Fprintf(out, "\nSwarm staged. Launch anytime with:\n  %s\n", paths.CLIInvocation("agent orchestrate "+planID))
		return nil
	}

	// Launch swarm orchestration in the background
	exe, err := os.Executable()
	if err != nil {
		exe = paths.BrandExecutableName()
	}
	logRelPath := filepath.Join(paths.ProjectDataDir, paths.LogsDir, fmt.Sprintf("swarm-orchestrate-%s.log", strings.ToLower(cleanName)))
	logPath := filepath.Join(projectRoot, logRelPath)
	_ = fileutil.EnsureDir(filepath.Dir(logPath))
	logFile, err := fileutil.OpenAppend(logPath)
	if err != nil {
		return errfmt.Newf("failed to open swarm orchestrate log %s", logPath).Wrap(err)
	}

	orchCmd := execwrap.Command(exe, "agent", "orchestrate", planID)
	orchCmd.Dir = projectRoot
	orchCmd.Stdout = logFile
	orchCmd.Stderr = logFile
	orchCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := orchCmd.Start(); err != nil {
		_ = logFile.Close()
		return errfmt.Newf("failed to launch swarm orchestrator").Wrap(err)
	}
	_ = logFile.Close()

	fmt.Fprintf(out, "\n🚀 Swarm launched in background (PID: %d)\n", orchCmd.Process.Pid)
	fmt.Fprintf(out, "  • Plan:     %s\n", planID)
	fmt.Fprintf(out, "  • Logs:     %s\n", logRelPath)
	fmt.Fprintf(out, "  • Monitor:  %s  OR  %s\n", paths.CLIInvocation("agent status"), paths.CLIInvocation("ui"))
	return nil
}
