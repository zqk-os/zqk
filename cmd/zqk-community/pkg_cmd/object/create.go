package object

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/cliexamples"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/interactive"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/objects"
)

// NewCreateCmd creates a new create command
// Generated from command spec - DO NOT EDIT MANUALLY (use spec file as source of truth)
func NewCreateCmd() *cobra.Command {
	// Use generated builder from spec
	cmd := bldr_cli_cmd_v1.NewObjectCreateCommandBuilder()
	cli.BindAsyncProgress(cmd, runCreate)
	cmd.Aliases = []string{"add", "new"}
	cmd.ValidArgsFunction = kindCompletion
	ensureCmdAnnotations(cmd)
	cmd.Annotations[AnnotationKindValidate] = KindValidatePositional0

	// Augment help with spec-driven examples when the user supplies a kind:
	//   zqk object create backlog_item --help
	// This is preserved from the original implementation
	defaultHelp := cmd.HelpFunc()
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		defaultHelp(cmd, args)
		var kind string
		if k, ok := kindCanonicalFromPRERun(cmd); ok {
			kind = k
		} else {
			if len(args) < 1 || args[0] == emptyValue {
				return
			}
			var err error
			kind, err = objects.ResolveAndValidateKindForProject(cli.ResolveProjectRoot("."), args[0])
			if err != nil {
				return
			}
		}

		gen, err := cliexamples.New()
		if err != nil {
			return
		}
		examples, err := gen.GenerateCLICommandExamples(kind)
		if err != nil || len(examples) == 0 {
			return
		}

		// Build additional help text using strings.Builder (POLICY-CODE-007 compliance)
		var helpBuilder strings.Builder
		helpBuilder.WriteString("\n")
		helpBuilder.WriteString("Spec-driven examples:\n")
		for _, line := range examples {
			helpBuilder.WriteString(line)
			helpBuilder.WriteString("\n")
		}

		// Route through cli.WriteOutput (POLICY-CODE-007 / MCP test harness)
		_ = cli.WriteOutput(cmd, []byte(helpBuilder.String()))
	})

	return cmd
}

func runCreate(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err

		kind, ok := kindCanonicalFromPRERun(cmd)
		if !ok {
			var err error
			kind, err = objects.ResolveAndValidateKindForProject(proc.ProjectRoot(), args[0])
			if err != nil {
				return cli.Guard(cmd).Err(err).Return()
			}
		}

		// Do not clear spec loader cache on every create (PRE_CHANGE_CHECKLIST §2–3, CLI_PERFORMANCE_AND_CONSISTENCY).
		// Full cache clear on each create/update causes reload of all specs and pushes response time to tens of seconds.
		// Spec freshness: use incremental invalidation or explicit refresh when config/spec files change; for white-labeling,
		// run a dedicated refresh (e.g. scheduler pre-warm or explicit command) when patterns change, not on hot path.

		logging.FluentEvent(proc.Logger()).Debug("Creating object").
			Kind(kind).
			Log()

		// Load object data
		objData, filePath, err := loadObjectData(cmd, proc, kind)
		if err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}

		// Handle interactive wizard flow
		interactiveFlag, _ := cmd.Flags().GetBool("interactive")
		if interactiveFlag {
			if err := runInteractiveWizard(cmd, kind, objData, proc); err != nil {
				return cli.Guard(cmd).Err(err).Return()
			}
		}

		// Normalize object values
		normalizeObjectData(objData, kind, proc)

		// Ensure kind matches
		if err := ensureKindMatches(objData, kind, proc); err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}

		// Handle dry-run
		handled, err := handleDryRun(cmd, objData, kind, proc)
		if err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}
		if handled {
			return nil
		}

		// Get relaxed flag
		relaxed, err := cmd.Flags().GetBool("relaxed")
		if err != nil {
			relaxed = false
		}

		// Set cache checker if relaxed mode (enables batch creation with non-blocking reference validation)
		if relaxed {
			setCacheCheckerForBatchCreation(proc)
		}

		// Get object ID and kind for cache context
		objID, _ := objData[objects.FieldKeyID].(string)
		objKind, _ := objData[objects.FieldKeyKind].(string)

		// Get force flag
		force, err := cmd.Flags().GetBool("force")
		if err != nil {
			force = false
		}

		// Add cache context for update operation
		opCtx := pkgctx.WithCacheUpdate(proc.OperationContext(), objID, objKind, "")

		// Emit loud contextual broadcast if we are doing this across a boundary
		clipkg.EmitContextHUD(proc.OperationContext(), proc.SecurityContext(), objData, "creating", objID)

		// Create object
		if err := proc.Storage().Create(opCtx, proc.SecurityContext(), objData); err != nil {
			// If object already exists and --force is set, update it instead
			if (err == storage.ErrObjectExists || strings.Contains(err.Error(), "already exists")) && force {
				if objID == emptyValue {
					return cli.Guard(cmd).Require(false, "cannot use --force without object ID").Return()
				}
				// Update existing object
				updateCtx := pkgctx.WithCacheUpdate(proc.OperationContext(), objID, objKind, "")
				if updateErr := proc.Storage().Update(updateCtx, proc.SecurityContext(), objID, objData); updateErr != nil {
					logging.FluentEvent(proc.Logger()).Error("Failed to update existing object with --force", updateErr).
						ObjectID(objID).
						Log()
					return cli.Guard(cmd).Err(updateErr).Wrapf("failed to update existing object with --force: %w").Return()
				}
				logging.FluentEvent(proc.Logger()).Info("Updated existing object with --force").
					ObjectID(objID).
					Log()
			} else {
				logging.FluentEvent(proc.Logger()).Error("Failed to create object", err).
					Kind(kind).
					Log()
				return cli.Guard(cmd).Err(err).Wrapf("failed to create object: %w").Return()
			}
		}

		// Update object ID and kind from data (in case they were generated/normalized by storage)
		if id, ok := objData[objects.FieldKeyID].(string); ok && id != emptyValue {
			objID = id
		}
		if k, ok := objData[objects.FieldKeyKind].(string); ok && k != emptyValue {
		}

		// Write-behind: Create returns after WAL+buffer enqueue; flush so the next zqk process (e.g. object get) sees CAS.
		flushCtx, cancelFlush := storage.DurabilityFlushContext()
		defer cancelFlush()
		t0 := time.Now()
		if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), []string{kind}); err != nil {
			logging.FluentEvent(proc.Logger()).Warn("Persist flush after object create timed out, but object is durable").
				WithError(err).
				Kind(kind).
				ObjectID(objID).
				Log()
			if objID != emptyValue {
				// Emit warning but don't fail, the object is safely on disk.
				fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString(fmt.Sprintf("Warning: Object '%s' created successfully, but index refresh is delayed.", objID)))
			} else {
				fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString("Warning: Object created successfully, but index refresh is delayed."))
			}
		}
		logSlowCLIObjectMutationFlush(proc.Logger(), "create", objID, []string{kind}, time.Since(t0), 0)

		// Trigger cache freshness check (async, non-blocking)
		proc.TriggerCacheFreshnessCheck("create", []string{kind})

		if filePath != emptyValue {
			_ = cli.ClearLastDraftPointerIfPath(proc.ProjectRoot(), filePath)
		}

		// Cleanup source file if needed
		cleanupSourceFile(cmd, filePath, proc)

		format := cli.GetFormat(cmd)
		if format == cli.FormatJSON || format == cli.FormatYAML || format == cli.FormatJSONRPC || format == cli.FormatStream {
			return cli.FormatOutput(cmd, objData)
		}

		// Output success message
		msg := formatCreateSuccessMessage(objData, kind, proc)
		return cli.WriteOutput(cmd, []byte(msg))
	})(cmd, args)
}

func runInteractiveWizard(cmd *cobra.Command, kind string, objData map[string]any, proc *cli.Processor) error {
	fmt.Fprintf(cmd.OutOrStdout(), "\n--- Interactive Object Instantiation (%s) ---\n", kind)

	reader := bufio.NewReader(cmd.InOrStdin())
	fmt.Fprintf(cmd.OutOrStdout(), "Scenario bundle to use for defaults (optional, press Enter to skip): ")
	scenarioID, _ := reader.ReadString('\n')
	scenarioID = strings.TrimSpace(scenarioID)

	if scenarioID != "" {
		ctx := pkgctx.NewSystemContext()
		secCtx := pkgctx.NewSystemSecurityContext()
		scenarioObj, err := proc.Storage().Read(ctx, secCtx, scenarioID)
		if err != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "Warning: failed to load scenario %s: %v\n", scenarioID, err)
		} else {
			// Extract defaults from scenario bundle
			if objectsMap, ok := scenarioObj["objects"].(map[string]any); ok {
				pluralKind := kind + "s"
				if kind == "scenario" {
					pluralKind = "scenarios"
				} else if strings.HasSuffix(kind, "y") {
					pluralKind = strings.TrimSuffix(kind, "y") + "ies"
				}

				if items, exists := objectsMap[pluralKind].([]any); exists && len(items) > 0 {
					if firstItem, ok := items[0].(map[string]any); ok {
						for k, v := range firstItem {
							if _, alreadySet := objData[k]; !alreadySet {
								objData[k] = v
							}
						}
						fmt.Fprintf(cmd.OutOrStdout(), "Loaded defaults from scenario %s.\n", scenarioID)
					}
				}
			}
		}
	}

	generator := interactive.NewTemplateGenerator(objects.GetGlobalFieldRegistry())
	templateWithTokens, err := generator.GenerateTemplateWithTokens(kind)
	if err != nil {
		return fmt.Errorf("failed to generate template: %w", err)
	}

	templateContent, err := interactive.ReplaceTokensInTemplate(templateWithTokens.Template, objData)
	if err != nil {
		return fmt.Errorf("failed to replace template tokens: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "zqk-interactive-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(templateContent); err != nil {
		return fmt.Errorf("failed to write to temp file: %w", err)
	}
	tmpFile.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	cmdEdit := exec.Command("sh", "-c", editor+" '"+tmpFile.Name()+"'")
	cmdEdit.Stdin = cmd.InOrStdin()
	cmdEdit.Stdout = cmd.OutOrStdout()
	cmdEdit.Stderr = cmd.ErrOrStderr()

	if err := cmdEdit.Run(); err != nil {
		return fmt.Errorf("editor exited with error: %w", err)
	}

	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return fmt.Errorf("failed to read from temp file: %w", err)
	}

	// Make sure they actually changed or wrote valid YAML
	var parsed map[string]any
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		return fmt.Errorf("invalid YAML in editor: %w", err)
	}

	for k, v := range parsed {
		// Ignore string values that are just the token placeholder
		if strV, ok := v.(string); ok && strings.HasPrefix(strV, "{") && strings.HasSuffix(strV, "}") {
			if strV == "{"+k+"}" {
				continue // Field left unfilled as a token placeholder
			}
		}
		objData[k] = v
	}

	fmt.Fprintf(cmd.OutOrStdout(), "-------------------------------------------\n\n")
	return nil
}
