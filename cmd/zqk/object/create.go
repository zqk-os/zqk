package object

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/interactive"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objectget"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewCreateCmd creates a new create command
// Generated from command spec - DO NOT EDIT MANUALLY (use spec file as source of truth)
func NewCreateCmd() *cobra.Command {
	// Use generated builder from spec
	cmd := bldr_cli_cmd_v1.NewObjectCreateCommandBuilder()
	cli.BindAsyncProgress(cmd, runCreate)
	cmd.Aliases = []string{"add", "new"}
	configureKindPositionalValidation(cmd, true)

	return cmd
}

func runCreate(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		if err := RequireElevatedInternal(cmd); err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}

		kind, err := resolvePositional0Kind(cmd, proc, args, fmt.Sprintf("object kind argument required (e.g. %s)", paths.CLIInvocation("object create <kind>")))
		if err != nil {
			return cli.Guard(cmd).Err(err).Return()
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

		if err := guardManualStatusOnCreate(cmd, proc, kind, objData); err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}

		// Elevated create defaults source_type like legacy internal create.
		if ElevatedInternalRequested(cmd) && objData[objects.FieldKeySourceType] == nil {
			objData[objects.FieldKeySourceType] = "internal"
		}

		// Handle interactive wizard flow
		interactiveFlag, _ := cmd.Flags().GetBool("interactive")
		if interactiveFlag {
			if err := runInteractiveWizard(cmd, kind, objData, proc); err != nil {
				return cli.Guard(cmd).Err(err).Return()
			}
		}

		// Normalize object values (spec field types). Kind policy is not here:
		// CoerceMutationFields + kernelcas overlays + CAS membrane.
		normalizeObjectData(objData, kind, proc)

		// Drop get-time hydration keys so CAS stays sealed (hash = truth).
		_ = objectget.StripReferenceResolverOverlayFields(objData)

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

		configureRelaxedMode(cmd, proc)

		// Get object ID and kind for cache context
		objID, _ := objData[objects.FieldKeyID].(string)
		objKind, _ := objData[objects.FieldKeyKind].(string)

		// Get force flag
		force, err := cmd.Flags().GetBool("force")
		if err != nil {
			force = false
		}

		promote, err := cmd.Flags().GetBool("promote")
		if err != nil {
			promote = false
		}

		// Add cache context for update operation.
		// Skip write-behind on interactive CLI create so CAS+index flush happen in-process
		// (avoids EnsureCLIObjectMutationVisible waiting on WAL checkpoint stabilization).
		// Revisit if bulk/API create needs shared write-behind.
		opCtx := pkgctx.WithCacheUpdate(proc.OperationContext(), objID, objKind, "")
		opCtx = storage.WithCLIOperation(storage.WithSkipWriteBehind(opCtx))
		if promote {
			opCtx = pkgctx.WithPromoteOnCreate(opCtx)
		}

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

		// Prove membrane visibility (or land repair draft).
		return finalizeCLIObjectCreate(cmd, proc, objData, kind, objID, filePath)
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

	tmpFile, err := fileutil.CreateTemp("", "zqk-interactive-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer fileutil.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(templateContent); err != nil {
		return fmt.Errorf("failed to write to temp file: %w", err)
	}
	_ = tmpFile.Close()

	editor := zqkenv.OSEditor().Get()
	if editor == "" {
		editor = "vi"
	}

	cmdEdit := execwrap.Command("sh", "-c", editor+" '"+tmpFile.Name()+"'")
	cmdEdit.Stdin = cmd.InOrStdin()
	cmdEdit.Stdout = cmd.OutOrStdout()
	cmdEdit.Stderr = cmd.ErrOrStderr()

	if err := cmdEdit.Run(); err != nil {
		return fmt.Errorf("editor exited with error: %w", err)
	}

	content, err := fileutil.ReadFile(tmpFile.Name())
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
