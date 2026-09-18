package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/cmd/zqk/agent"
	"github.com/zqk-os/zqk/cmd/zqk/ambient"
	"github.com/zqk-os/zqk/cmd/zqk/automation"
	"github.com/zqk-os/zqk/cmd/zqk/callback"
	cicmd "github.com/zqk-os/zqk/cmd/zqk/ci"
	"github.com/zqk-os/zqk/cmd/zqk/convergence"
	"github.com/zqk-os/zqk/cmd/zqk/docman"
	"github.com/zqk-os/zqk/cmd/zqk/domain"
	"github.com/zqk-os/zqk/cmd/zqk/feed"
	"github.com/zqk-os/zqk/cmd/zqk/graph"
	"github.com/zqk-os/zqk/cmd/zqk/grep"
	"github.com/zqk-os/zqk/cmd/zqk/healthchk"
	"github.com/zqk-os/zqk/cmd/zqk/inbox"
	"github.com/zqk-os/zqk/cmd/zqk/intake"
	"github.com/zqk-os/zqk/cmd/zqk/keystore"
	"github.com/zqk-os/zqk/cmd/zqk/learn"
	matrixcmd "github.com/zqk-os/zqk/cmd/zqk/matrix"
	"github.com/zqk-os/zqk/cmd/zqk/mcp"
	"github.com/zqk-os/zqk/cmd/zqk/mesh"
	newcmd "github.com/zqk-os/zqk/cmd/zqk/new"
	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/cmd/zqk/observer"
	"github.com/zqk-os/zqk/cmd/zqk/ontology"
	"github.com/zqk-os/zqk/cmd/zqk/ops"
	"github.com/zqk-os/zqk/cmd/zqk/organizational"
	"github.com/zqk-os/zqk/cmd/zqk/precommit"
	"github.com/zqk-os/zqk/cmd/zqk/reports"
	rollbackcmd "github.com/zqk-os/zqk/cmd/zqk/rollback"
	"github.com/zqk-os/zqk/cmd/zqk/scheduler"
	"github.com/zqk-os/zqk/cmd/zqk/semantic"
	"github.com/zqk-os/zqk/cmd/zqk/spec"
	"github.com/zqk-os/zqk/cmd/zqk/swarm"
	"github.com/zqk-os/zqk/cmd/zqk/system"
	testcmd "github.com/zqk-os/zqk/cmd/zqk/test"
	"github.com/zqk-os/zqk/cmd/zqk/tray"
	"github.com/zqk-os/zqk/cmd/zqk/utility"
	"github.com/zqk-os/zqk/cmd/zqk/validate"
	"github.com/zqk-os/zqk/cmd/zqk/workflow"
	"github.com/zqk-os/zqk/pkg/objects"
	internal "github.com/zqk-os/zqk/pkg/zqkcli"
	"github.com/spf13/cobra"
)

// registerCommands registers all implemented commands
// Commands are organized into logical groups with common and specialized subcommands
func registerCommands() {
	// getting_started must be registered before any subcommand uses GroupID
	// "getting_started" (e.g. quickstart) — cobra panics on Execute otherwise.
	// TRACK: BLI-1785839468275201000-ddb513d4 — quickstart GroupID registration.
	rootCmd.AddGroup(&cobra.Group{ID: "getting_started", Title: "Getting Started:"})
	rootCmd.AddGroup(&cobra.Group{ID: "everyday", Title: "Everyday Commands:"})
	rootCmd.AddGroup(&cobra.Group{ID: "integrations", Title: "Integrations & Automation:"})
	rootCmd.AddGroup(&cobra.Group{ID: "advanced", Title: "Advanced Knowledge Kernel:"})
	rootCmd.AddGroup(&cobra.Group{ID: "admin", Title: "Administration:"})

	// Ensure field registry is loaded before adding object command so dynamic kind commands
	// are available (e.g. when ZQK_TEST_ROOT was set by test before NewRootCommand()).
	if reg := objects.GetGlobalFieldRegistry(); reg != nil {
		// _ = reg.LoadFields() // PERF: Deferred to RegisterDynamicKindCommands
	}
	// Object operations group (CRUD, query, and management)
	objCmd := object.NewObjectCmd()
	object.RegisterDynamicKindCommands(objCmd)
	objCmd.GroupID = "everyday"
	rootCmd.AddCommand(objCmd)

	// System operations group (health, validation, and maintenance)
	systemCmdInst := system.NewSystemCmd()
	systemCmdInst.GroupID = "everyday"
	rootCmd.AddCommand(systemCmdInst)

	// In-process code search engine (grep / zgrep)
	grepCmdInst := grep.NewGrepCmd()
	grepCmdInst.GroupID = "everyday"
	rootCmd.AddCommand(grepCmdInst)

	// Health check registry (list, enable/disable, run monitors)
	healthchkCmdInst := healthchk.NewHealthchkCmd()
	healthchkCmdInst.GroupID = "admin"
	rootCmd.AddCommand(healthchkCmdInst)

	// Documentation management group
	docmanCmdInst := docman.NewDocmanCmd()
	docmanCmdInst.GroupID = "advanced"
	rootCmd.AddCommand(docmanCmdInst)

	// Learn curriculum — onboarding adjacent, not core everyday CRUD
	learnCmdInst := learn.NewLearnCmd()
	learnCmdInst.GroupID = "integrations"
	rootCmd.AddCommand(learnCmdInst)

	// Graph operations group (reasoning and raw queries)
	graphCmdInst := graph.NewGraphCmd()
	graphCmdInst.GroupID = "advanced"
	rootCmd.AddCommand(graphCmdInst)

	// Ambient operations group
	ambientCmdInst := ambient.NewAmbientCmd()
	ambientCmdInst.GroupID = "advanced"
	rootCmd.AddCommand(ambientCmdInst)

	// Internal operations group (admin-only, for managing built-in and internal objects)
	internalCmdInst := internal.NewInternalCmd()
	internalCmdInst.GroupID = "admin"
	rootCmd.AddCommand(internalCmdInst)

	// Utility operations group (version, migration, and helpers)
	utilityCmdInst := utility.NewUtilityCmd()
	utilityCmdInst.GroupID = "admin"
	rootCmd.AddCommand(utilityCmdInst)
	// Top-level `version` — spec `.zqk/cli/specs/root/version_command.yaml` (same Run as utility version).
	versionCmdInst := utility.NewRootVersionCmd()
	versionCmdInst.GroupID = "everyday" // Give it a more appropriate group
	rootCmd.AddCommand(versionCmdInst)

	// Automation and integration group (hooks, CI/CD, and scripts)
	automationCmdInst := automation.NewAutomationCmd()
	automationCmdInst.GroupID = "integrations"
	rootCmd.AddCommand(automationCmdInst)

	// Pre-commit background results (hook reads single file; jobs write category files)
	precommitCmdInst := precommit.NewPreCommitCmd()
	precommitCmdInst.GroupID = "integrations"
	rootCmd.AddCommand(precommitCmdInst)

	// Quick create — shortcut; canonical draft path is object template (community CLI friction)
	quickstartCmdInst := system.NewQuickstartCmd()
	quickstartCmdInst.GroupID = "getting_started"
	rootCmd.AddCommand(quickstartCmdInst)

	// Tray: named shortcuts to zqk argv (.zqk/tray.yaml over embedded defaults)
	trayCmdInst := tray.NewTrayCmd()
	trayCmdInst.GroupID = "integrations"
	rootCmd.AddCommand(trayCmdInst)

	// Draft templates (new) — shortcut; prefer object template for community orientation
	newcmdCmdInst := newcmd.NewNewCmd()
	newcmdCmdInst.GroupID = "integrations"
	rootCmd.AddCommand(newcmdCmdInst)

	// Reports group (AI metrics: PCS, EDD, D&B)
	reportsCmdInst := reports.NewReportsCmd()
	reportsCmdInst.GroupID = "integrations"
	rootCmd.AddCommand(reportsCmdInst)

	// Traceability matrices (vetting CSVs, registry; see docs/architecture/MATRIX_CLI_STRATEGY.md)
	matrixcmdCmdInst := matrixcmd.NewMatrixCmd()
	matrixcmdCmdInst.GroupID = "advanced"
	rootCmd.AddCommand(matrixcmdCmdInst)

	// MCP server group
	mcpCmdInst := mcp.NewMCPCmd()
	mcpCmdInst.GroupID = "integrations"
	rootCmd.AddCommand(mcpCmdInst)

	// Observer agent (AST extraction, knowledge kernel)
	observerCmdInst := observer.NewObserverCmd()
	observerCmdInst.GroupID = "advanced"
	rootCmd.AddCommand(observerCmdInst)

	// Keystore operations group
	keystoreCmdInst := keystore.NewKeystoreCmd()
	keystoreCmdInst.GroupID = "integrations"
	rootCmd.AddCommand(keystoreCmdInst)

	// Scheduler — operator tooling; keep near everyday but not orientation clutter
	schedulerCmdInst := scheduler.NewSchedulerCmd()
	schedulerCmdInst.GroupID = "integrations"
	rootCmd.AddCommand(schedulerCmdInst)

	// Local CI — commit on studio, checkout SHA elsewhere, scan-tests (PRI-1785699924616992000-8000284f)
	ciCmdInst := cicmd.NewCICmd()
	ciCmdInst.GroupID = "integrations"
	rootCmd.AddCommand(ciCmdInst)

	// Callback operations group
	callbackCmdInst := callback.NewCallbackCmd()
	callbackCmdInst.GroupID = "integrations"
	rootCmd.AddCommand(callbackCmdInst)

	// Rollback (lifecycle/maintenance snapshots: list, apply, reconstruct)
	rollbackcmdCmdInst := rollbackcmd.NewRollbackCmd()
	rollbackcmdCmdInst.GroupID = "advanced"
	rootCmd.AddCommand(rollbackcmdCmdInst)

	// Semantic operations group
	semanticCmdInst := semantic.NewSemanticCmd()
	semanticCmdInst.GroupID = "advanced"
	rootCmd.AddCommand(semanticCmdInst)

	// Spec operations group (programmatic spec management, CRIT-9036)
	specCmdInst := spec.NewSpecCmd()
	specCmdInst.GroupID = "advanced"
	rootCmd.AddCommand(specCmdInst)

	// Organizational operations group
	organizationalCmdInst := organizational.NewOrganizationalCmd()
	organizationalCmdInst.GroupID = "advanced"
	rootCmd.AddCommand(organizationalCmdInst)

	// Domain ontology discovery and registration
	domainCmdInst := domain.NewDomainCmd()
	domainCmdInst.GroupID = "advanced"
	rootCmd.AddCommand(domainCmdInst)

	// Ontology import and translation
	ontologyCmdInst := ontology.NewOntologyCmd()
	ontologyCmdInst.GroupID = "advanced"
	rootCmd.AddCommand(ontologyCmdInst)

	// Workflow guidance commands
	workflowCmdInst := workflow.NewWorkflowCmd()
	validateCmdInst := validate.NewValidateCmd()
	workflowCmdInst.GroupID = "everyday"
	validateCmdInst.GroupID = "everyday"
	rootCmd.AddCommand(workflowCmdInst)
	rootCmd.AddCommand(validateCmdInst)

	// Auth (login, logout) for session lifecycle
	authCmdInst := NewAuthCmd()
	authCmdInst.GroupID = "everyday"
	rootCmd.AddCommand(authCmdInst)

	// Use: set persistent project root for this workspace (see PROJECT_ROOT_USE_AND_SCHEDULER_ALIGNMENT.md)
	useCmdInst := NewUseCmd()
	useCmdInst.GroupID = "everyday"
	rootCmd.AddCommand(useCmdInst)

	// Join: connect to a peer kernel in the Sovereign Mesh
	joinCmdInst := NewJoinCmd()
	joinCmdInst.GroupID = "advanced"
	rootCmd.AddCommand(joinCmdInst)

	// Mesh: manage the federated economy
	meshCmdInst := mesh.NewMeshCmd()
	meshCmdInst.GroupID = "advanced"
	rootCmd.AddCommand(meshCmdInst)

	// Autonomy Inbox: human-in-the-loop proposal review
	inboxCmdInst := inbox.NewInboxCmd()
	inboxCmdInst.GroupID = "integrations"
	rootCmd.AddCommand(inboxCmdInst)

	// Agent correspondence feed (steer / emit-status) — Go-only ship path
	feedCmdInst := feed.NewFeedCmd()
	feedCmdInst.GroupID = "integrations"
	rootCmd.AddCommand(feedCmdInst)

	// Swarm throughput dashboard (MMORCH)
	swarmCmdInst := swarm.NewSwarmCmd()
	swarmCmdInst.GroupID = "advanced"
	rootCmd.AddCommand(swarmCmdInst)

	// Semantic Intake Pipeline
	intakeCmdInst := intake.NewIntakeCmd()
	intakeCmdInst.GroupID = "integrations"
	rootCmd.AddCommand(intakeCmdInst)

	// Ops commands
	opsCmdInst := ops.NewOpsCmd()
	opsCmdInst.GroupID = "advanced"
	rootCmd.AddCommand(opsCmdInst)

	// Policy management

	// TRACK: doctor command (2026-09-02)

	// Shell completion (bash, zsh, fish)
	completionCmd := NewCompletionCmd(rootCmd)
	completionCmd.GroupID = "integrations"
	rootCmd.AddCommand(completionCmd)

	// Priority plan shortcuts (pplan add/remove/current/next/prev)
	pplanCmdInst := object.NewPPlanCmd()
	pplanCmdInst.GroupID = "everyday"
	rootCmd.AddCommand(pplanCmdInst)

	// Executable test case runner and criteria verification
	testCmdInst := testcmd.NewTestCmd()
	testCmdInst.GroupID = "everyday"
	rootCmd.AddCommand(testCmdInst)

	// Multi-agent orchestration
	agentCmdInst := agent.NewAgentCmd()
	agentCmdInst.GroupID = "advanced"
	rootCmd.AddCommand(agentCmdInst)

	// Top-level convergence measurement & nest management
	convergenceCmdInst := convergence.NewConvergenceCmd()
	convergenceCmdInst.GroupID = "advanced"
	rootCmd.AddCommand(convergenceCmdInst)

	// Only available in zqk-admin binary
	isAdminBinary := false
	if len(os.Args) > 0 && (strings.HasSuffix(os.Args[0], "zqk-admin") || strings.HasSuffix(os.Args[0], "zqk-admin.exe")) {
		isAdminBinary = true
	}

	if isAdminBinary {
		rootCmd.AddCommand(ambientCmdInst)
		rootCmd.AddCommand(internalCmdInst)
		rootCmd.AddCommand(ontologyCmdInst)
		rootCmd.AddCommand(matrixcmdCmdInst)
		rootCmd.AddCommand(organizationalCmdInst)
		rootCmd.AddCommand(callbackCmdInst)
		rootCmd.AddCommand(intakeCmdInst)
		rootCmd.AddCommand(keystoreCmdInst)
		rootCmd.AddCommand(domainCmdInst)
		rootCmd.AddCommand(observerCmdInst)
		rootCmd.AddCommand(semanticCmdInst)
		rootCmd.AddCommand(specCmdInst)
		rootCmd.AddCommand(meshCmdInst)
	}

	// Domain-specific commands (future)
	// rootCmd.AddCommand(domain.NewBacklogCmd())
	// rootCmd.AddCommand(domain.NewGoalCmd())
	// rootCmd.AddCommand(domain.NewMilestoneCmd())
	// rootCmd.AddCommand(domain.NewWorkstreamCmd())
	// rootCmd.AddCommand(domain.NewPriorityPlanCmd())

	// Filter out admin commands if we are not running as the zqk-admin binary (or a test binary)
	baseArg := filepath.Base(os.Args[0])
	if !strings.HasPrefix(baseArg, "zqk-admin") && !strings.HasSuffix(baseArg, ".test") {
		var toRemove []*cobra.Command
		for _, cmd := range rootCmd.Commands() {
			if cmd.GroupID == "admin" {
				toRemove = append(toRemove, cmd)
			}
		}
		rootCmd.RemoveCommand(toRemove...)
	}
}
