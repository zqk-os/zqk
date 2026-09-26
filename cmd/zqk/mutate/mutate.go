package mutate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/mutation"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewMutateCmd creates the top-level 'zqk mutate' command for declarative ZQL transactions.
func NewMutateCmd() *cobra.Command {
	var (
		filePath string
		dryRun   bool
		format   string
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Execute declarative ACID mutations across the knowledge kernel",
		"Declarative ZQL mutation engine with Kahn topological variable resolution, in-memory preflight validation, and atomic transaction rollback.",
		"",
		"Enables agents and swarms to execute multi-object creations and relationship wiring in a single declarative block with complete rollback guarantees.",
	).
		AddExample("Create plan and backlog item atomically", `%s mutate "BEGIN; LET $p = UPSERT priority_plan { title: 'New Plan', priority_tier: 'P1', status: 'in_progress' }; UPSERT backlog_item { title: 'First Task', priority_plan_ref: $p.id, priority_tier: 'P1', status: 'planned' }; COMMIT;"`).
		AddExample("Validate mutations with dry-run preflight", `%s mutate -f script.zql --dry-run`).
		AddExample("Output diagnostic receipt as JSON", `%s mutate -f script.zql --format json`)

	cmd := &cobra.Command{
		Use:     "mutate [zql-script]",
		Short:   "Execute declarative ZQL mutations and transactions",
		Aliases: []string{"zql"},
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
				for i := range program.Statements {
					if program.Statements[i].NodeType == mutation.StmtBeginTransaction {
						program.Statements[i].IsolationLevel = mutation.IsolationDryRun
					}
				}
			}

			engine := mutation.NewTransactionEngine()
			executor := mutation.NewZQLExecutor(engine)

			receipt, err := executor.Execute(cmd.Context(), program)
			if err != nil {
				return fmt.Errorf("transaction execution failed: %w", err)
			}

			// If connected to storage and not dry-run, persist committed mutations
			if !dryRun && receipt.Committed {
				if proc, procErr := cli.NewProcessor(cmd); procErr == nil && proc.Storage() != nil {
					_ = persistCommittedMutations(cmd.Context(), proc, engine, receipt)
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
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to file containing ZQL mutation script")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate preflight schemas and topological order without persisting")
	cmd.Flags().StringVar(&format, "format", "table", "Output format (table, json)")

	helpBuilder.ApplyToCommand(cmd)
	cli.RequireSession(cmd, false)
	cli.RequireStorage(cmd, false)

	return cmd
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
			if k, ok := data["kind"].(string); ok {
				kind = k
			}
		}

		switch r.Action {
		case mutation.ActionCreateNode, mutation.ActionUpdateNode:
			existing, readErr := proc.Storage().Read(ctx, secCtx, r.TargetID)
			if readErr == nil && existing != nil {
				for k, v := range data {
					existing[k] = v
				}
				_ = proc.Storage().Update(ctx, secCtx, r.TargetID, existing)
			} else {
				if err := proc.Storage().Create(ctx, secCtx, data); err != nil {
					_ = proc.Storage().Update(ctx, secCtx, r.TargetID, data)
				}
			}
		case mutation.ActionRemoveEdge:
			_ = proc.Storage().Delete(ctx, secCtx, r.TargetID, false)
		}
	}
	return nil
}
