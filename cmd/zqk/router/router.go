package router

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	cliapp "github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/process"
	routerpkg "github.com/zqk-os/zqk/pkg/router"
)

// NewRouterCmd creates the 'zqk router' command group for evaluating and inspecting the L7 gradient router.
func NewRouterCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Universal tool and model gradient router daemon and evaluator",
		"Evaluates tool invocations, shell commands, and model requests through the L7 gradient router pipeline.",
		"",
		"Ensures token budgeting, in-repo interception, AST optimization, and graceful fallback.",
	).
		AddExample("Evaluate a command route", paths.CLIUsage("router", "eval", "grep -rn 'foo' pkg/")).
		AddExample("Evaluate route with token budget", paths.CLIUsage("router", "eval", "grep 'foo' pkg/", "--token-budget", "1500")).
		AddExample("View router runtime statistics", paths.CLIUsage("router", "stats"))

	cmd := &cobra.Command{
		Use:   "router",
		Short: "Universal tool and model gradient router",
		Long:  "Universal tool and model gradient router daemon and evaluator for L7 token budgeting and dispatch.",
	}

	helpBuilder.ApplyToCommand(cmd)

	cmd.AddCommand(newEvalCmd())
	cmd.AddCommand(newStatsCmd())

	cliapp.RequireSession(cmd, false)
	cliapp.RequireStorage(cmd, false)

	return cmd
}

func newEvalCmd() *cobra.Command {
	var tokenBudget int
	var format string
	var cwd string

	cmd := &cobra.Command{
		Use:   "eval [command]",
		Short: "Evaluate a command line or tool call through the gradient router pipeline",
		Long:  "Evaluates a command line against the L7 gradient pipeline stages (funnel, in-repo boundary, token budget guard, specialized tool selector, and fallback).",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			process.TouchMeaningfulActivity()

			cmdLine := strings.Join(args, " ")
			gr := routerpkg.NewGradientRouter(nil, nil)

			req := &routerpkg.RouteRequest{
				CommandLine: cmdLine,
				TokenBudget: tokenBudget,
				Cwd:         cwd,
			}

			decision, err := gr.Route(cmd.Context(), req)
			if err != nil {
				return errfmt.Errorf("router eval failed: %w", err)
			}

			if strings.ToLower(format) == "json" {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(decision)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Action:     %s\n", decision.Action)
			fmt.Fprintf(out, "Target:     %s\n", decision.TargetTool)
			fmt.Fprintf(out, "Intercept:  %t\n", decision.Intercepted)
			fmt.Fprintf(out, "Bypassed:   %t\n", decision.Bypassed)
			fmt.Fprintf(out, "Fallback:   %t\n", decision.Fallback)
			if decision.Reason != "" {
				fmt.Fprintf(out, "Reason:     %s\n", decision.Reason)
			}
			if decision.RewrittenCommand != "" {
				fmt.Fprintf(out, "Rewritten:  %s\n", decision.RewrittenCommand)
			}
			fmt.Fprintf(out, "Stage:      %s\n", decision.Stage)
			fmt.Fprintf(out, "Tokens:     %d\n", decision.TokenBudget)
			return nil
		},
	}

	cmd.Flags().IntVar(&tokenBudget, "token-budget", 2000, "Token budget ceiling for intercepted commands")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format (json, text)")
	cmd.Flags().StringVar(&cwd, "cwd", "", "Working directory context")

	return cmd
}

func newStatsCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Display ambient gradient router telemetry and statistics",
		Long:  "Outputs point-in-time metrics on model gradient dispatch, tool calls, routes chosen, and bypass reasons.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			process.TouchMeaningfulActivity()

			gr := routerpkg.NewGradientRouter(nil, nil)
			stats := gr.Stats()

			if strings.ToLower(format) == "text" {
				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "Model Requests:  %d\n", stats.ModelRequestsTotal)
				fmt.Fprintf(out, "Tool Calls:      %d\n", stats.ToolCallsTotal)
				fmt.Fprintf(out, "Intercepted:     %d\n", stats.ToolCallsIntercepted)
				fmt.Fprintf(out, "Passthrough:     %d\n", stats.ToolCallsPassthrough)
				fmt.Fprintf(out, "Fallback Events: %d\n", stats.FallbackEventsTotal)
				fmt.Fprintf(out, "Route Requests:  %d\n", stats.RouteRequestsTotal)
				return nil
			}

			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(stats)
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format (json, text)")

	return cmd
}
