package ui

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/studio"
)

// NewUICmd creates the 'zqk ui' command for interactive terminal mission control.
func NewUICmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewUiCommandBuilder()
	cmd.Aliases = []string{"dashboard", "console"}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
			projectRoot := proc.ProjectRoot()
			if projectRoot == "" {
				projectRoot = cli.ResolveProjectRoot(".")
			}

			web, _ := cmd.Flags().GetBool("web")
			if web {
				port, _ := cmd.Flags().GetInt("port")
				if port <= 0 {
					port = 8080
				}
				addr := fmt.Sprintf("127.0.0.1:%d", port)
				server := studio.NewServer(projectRoot)
				if err := server.Start(addr); err != nil {
					return err
				}
				fmt.Printf("⚡ ZQK Knowledge Kernel Visual Studio running at http://%s (Press Ctrl+C to stop)\n", server.Addr())
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
				<-sigCh
				return server.Shutdown(proc.OperationContext())
			}

			tab, _ := cmd.Flags().GetString("tab")
			sec := pkgctx.NewSystemSecurityContext()
			sp := proc.Storage()

			format := proc.Format()
			if format == cli.FormatJSON || format == cli.FormatJSONL || format == cli.FormatYAML {
				m := NewUIModel(projectRoot, tab)
				m.RefreshMutations()
				m.RefreshAuditEvents()
				m.RefreshObjects()
				m.RefreshQA(proc.OperationContext(), sp, sec)
				m.RefreshHealth()
				if sp != nil && sec != nil {
					m.RefreshSwarm(proc.OperationContext(), sp, sec)
					m.RefreshPM(proc.OperationContext(), sp, sec)
					m.RefreshMetrics(proc.OperationContext(), sp, sec)
					m.RefreshScheduler(proc.OperationContext(), sp, sec)
				}
				return cli.FormatOutput(cmd, m)
			}

			return RunTUI(proc.OperationContext(), projectRoot, tab, sp, sec)
		})(cmd, args)
	}

	return cmd
}
