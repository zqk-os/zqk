package ui

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/studio"
	"github.com/zqk-os/zqk/pkg/tui"
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

			web, err := cmd.Flags().GetBool("web")
			if err != nil {
				return err
			}
			if web {
				port, err := cmd.Flags().GetInt("port")
				if err != nil || port <= 0 {
					port = 8080
				}
				server := studio.NewServer(projectRoot)

				if cmd.Flags().Changed("port") {
					// User explicitly requested this port: fail with helpful guidance if bound
					addr := fmt.Sprintf("127.0.0.1:%d", port)
					err = server.Start(addr)
					if err != nil {
						return fmt.Errorf("port %d is already in use. Specify a different port with: %s ui -w --port <port> (or -p <port>): %w", port, brand.ExecutableName(), err)
					}
				} else {
					// Default port: try 8080, and if in use, auto-fallback to next available ports (8081-8095)
					for p := port; p <= port+15; p++ {
						addr := fmt.Sprintf("127.0.0.1:%d", p)
						err = server.Start(addr)
						if err == nil {
							if p != port {
								_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("ℹ️  Port %d was in use; automatically selected port %d (use --port / -p to override)\n", port, p)))
							}
							break
						}
					}
					if err != nil {
						return fmt.Errorf("ports %d-%d are already in use. Specify an open port with: %s ui -w --port <port> (or -p <port>): %w", port, port+15, brand.ExecutableName(), err)
					}
				}

				_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("⚡ ZQK Knowledge Kernel Visual Studio running at http://%s (Press Ctrl+C to stop)\n", server.Addr())))
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
				<-sigCh
				return server.Shutdown(proc.OperationContext())
			}

			tab, err := cmd.Flags().GetString("tab")
			if err != nil {
				return err
			}
			sec := pkgctx.NewSystemSecurityContext()
			sp := proc.Storage()

			format := proc.Format()
			if format == cli.FormatJSON || format == cli.FormatJSONL || format == cli.FormatYAML {
				m := tui.NewUIModel(projectRoot, tab)
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

			return tui.RunTUI(proc.OperationContext(), projectRoot, tab, sp, sec)
		})(cmd, args)
	}

	return cmd
}
