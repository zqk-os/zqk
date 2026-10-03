package mutate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/mutation"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewMutateCmd creates the top-level 'zqk mutate' command for declarative ZQL transactions.
func NewMutateCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewMutateCommandBuilder(), &cobra.Command{
		RunE: runMutate,
	})
	cmd.Flags().Bool("break-glass", false, "Elevated break-glass execution for emergency overrides")
	cmd.Flags().String("break-glass-reason", "", "Mandatory justification reason when --break-glass is armed (min 10 characters)")
	cli.BindAsyncProgress(cmd, runMutate)
	return cmd
}

func runMutate(cmd *cobra.Command, args []string) error {
	filePath, _ := cmd.Flags().GetString("file")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	format, _ := cmd.Flags().GetString("format")
	breakGlass, _ := cmd.Flags().GetBool("break-glass")
	breakGlassReason, _ := cmd.Flags().GetString("break-glass-reason")

	ctx := cmd.Context()
	if breakGlass {
		trimmedReason := strings.TrimSpace(breakGlassReason)
		if trimmedReason == "" {
			return fmt.Errorf("--break-glass requires explicit non-empty justification via --break-glass-reason")
		}
		if len(trimmedReason) < 10 {
			return fmt.Errorf("--break-glass justification reason too short (%d chars, min 10 required)", len(trimmedReason))
		}
		ctx = pkgctx.WithLifecycleBreakGlass(ctx, trimmedReason)
	}

	var scriptStr string

	if filePath != "" {
		if filePath == "-" {
			b, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("failed to read mutation script from stdin: %w", err)
			}
			scriptStr = string(b)
		} else {
			data, err := fileutil.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read mutation script file: %w", err)
			}
			scriptStr = string(data)
		}
	} else if len(args) > 0 {
		if args[0] == "-" {
			b, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("failed to read mutation script from stdin: %w", err)
			}
			scriptStr = string(b)
		} else {
			scriptStr = args[0]
		}
	} else {
		return fmt.Errorf("mutation script required as argument or via -f/--file")
	}

	program, err := mutation.ParseZQL(scriptStr)
	if err != nil {
		return fmt.Errorf("ZQL syntax error: %w", err)
	}

	// If dry-run requested, force dry_run isolation
	if dryRun {
		program.ApplyDryRunIsolation()
	}

	executor := mutation.NewZQLExecutor(nil)
	engine := executor.Engine()

	var proc *cli.Processor
	if p, procErr := cli.NewProcessor(cmd); procErr == nil && p.Storage() != nil {
		proc = p
		executor.WithObjectResolver(func(ctx context.Context, id string) (map[string]any, bool, error) {
			if data, ok := engine.Get(ctx, id); ok {
				return data, true, nil
			}
			obj, err := proc.Storage().Read(ctx, proc.SecurityContext(), id)
			if err == nil && obj != nil {
				return obj, true, nil
			}
			return nil, false, nil
		})
	}

	receipt, err := executor.Execute(ctx, program)
	if err != nil {
		return fmt.Errorf("transaction execution failed: %w", err)
	}

	// If connected to storage and not dry-run, persist committed mutations
	if !dryRun && receipt.Committed && proc != nil && proc.Storage() != nil {
		if err := persistCommittedMutations(ctx, proc, engine, receipt); err != nil {
			return fmt.Errorf("mutation persistence failed: %w", err)
		}
	}

	out := cmd.OutOrStdout()
	if strings.EqualFold(format, "json") {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(receipt)
	}

	// Table / human output
	statusStr := "COMMITTED"
	if dryRun {
		statusStr = "DRY-RUN VALIDATED (NOT COMMITTED)"
	} else if !receipt.Committed {
		statusStr = "ROLLED BACK"
	}

	fmt.Fprintf(out, "Transaction %s [%s]\n", receipt.TransactionID, statusStr)
	fmt.Fprintf(out, "Isolation: %s | Total Mutations: %d\n", receipt.IsolationLevel, len(receipt.Receipts))
	fmt.Fprintln(out, strings.Repeat("-", 60))

	for _, r := range receipt.Receipts {
		errPart := ""
		if r.Error != "" {
			errPart = fmt.Sprintf(" (error: %s)", r.Error)
		}
		fmt.Fprintf(out, "[%d] %s %s [%s] -> %s%s\n",
			r.Index, r.Action, r.TargetKind, r.TargetID, r.Status, errPart)
	}

	return nil
}

func persistCommittedMutations(ctx context.Context, proc *cli.Processor, engine *mutation.TransactionEngine, receipt *mutation.ZQLExecutionReceipt) error {
	secCtx := proc.SecurityContext()

	for _, r := range receipt.Receipts {
		if r.Status != mutation.ReceiptStatusCommitted {
			continue
		}
		data, ok := engine.Get(ctx, r.TargetID)
		if !ok {
			continue
		}
		kind := r.TargetKind
		if kind == "" {
			if k, ok := data[objects.FieldKeyKind].(string); ok {
				kind = k
			}
		}
		if kind != "" {
			data[objects.FieldKeyKind] = kind
		}
		if r.TargetID != "" {
			data[objects.FieldKeyID] = r.TargetID
		}

		switch r.Action {
		case mutation.ActionCreateNode, mutation.ActionUpdateNode:
			existing, readErr := proc.Storage().Read(ctx, secCtx, r.TargetID)

			if readErr == nil && existing != nil {
				// For updates to existing objects, updates map should only contain the updated domain fields.
				// System provenance fields (created_at, created_by, updated_at, updated_by, cas_address, hash)
				// are system-computed by Storage().Update() and must NOT be injected manually unless
				// break-glass is explicitly armed with custom fields.
				if !pkgctx.IsLifecycleBreakGlass(ctx) {
					delete(data, "created_at")
					delete(data, "created_by")
					delete(data, "updated_at")
					delete(data, "updated_by")
					delete(data, "cas_address")
					delete(data, "hash")
				}

				if err := proc.Storage().Update(ctx, secCtx, r.TargetID, data); err != nil {
					return fmt.Errorf("failed to update node %s: %w", r.TargetID, err)
				}
			} else {
				if err := proc.Storage().Create(ctx, secCtx, data); err != nil {
					if updateErr := proc.Storage().Update(ctx, secCtx, r.TargetID, data); updateErr != nil {
						return fmt.Errorf("failed to persist node %s: %w (create error: %v)", r.TargetID, updateErr, err)
					}
				}
			}
		case mutation.ActionRemoveEdge:
			if err := proc.Storage().Delete(ctx, secCtx, r.TargetID, false); err != nil {
				return fmt.Errorf("failed to delete node %s: %w", r.TargetID, err)
			}
		}
	}
	return nil
}
