package feed

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/agentfeed/httpapi"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

// NewServeCmd creates zqk feed serve (private HTTP API).
func NewServeCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewServeCommandBuilder()
	cmd.RunE = runFeedServe
	return cmd
}

func runFeedServe(cmd *cobra.Command, _ []string) error {
	return withFeedRoot(func(cmd *cobra.Command, proc *cli.Processor, root string, flags *clipkg.FlagBag) error {
		listen := flags.String(cmd, "listen")
		token := flags.String(cmd, "token")
		if err := flags.Err(); err != nil {
			return err
		}

		logger := logging.GetLoggerFromProfile(proc.Context().Profile)
		srv, err := httpapi.New(httpapi.Config{
			ProjectRoot: root,
			ListenAddr:  listen,
			Token:       token,
			Logger:      logger,
		})
		if err != nil {
			return errfmt.Newf("feed serve").Wrap(err)
		}

		logging.Fluent(logger).Info("feed HTTP API listening").
			String("listen", listen).
			String("schema", "zqk_feed_http_v1").
			Bool("auth_token_set", token != "").
			Log()

		ctx, stop := cli.CommandSignalContext(cmd)
		defer stop()

		if err := srv.ListenAndServe(ctx); err != nil {
			return errfmt.Newf("feed serve").Wrap(err)
		}
		logging.Fluent(logger).Info("feed HTTP API stopped").Log()
		return nil
	})(cmd, nil)
}
