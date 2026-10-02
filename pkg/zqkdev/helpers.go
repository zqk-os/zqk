package zqkdev

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

// cmdLogger resolves a logger for a cobra command based on its context profile.
func cmdLogger(cmd *cobra.Command) logging.Logger {
	ctx := cli.GetContext(cmd)
	if ctx != nil && ctx.Profile != EmptyValue {
		return logging.GetLoggerFromProfile(ctx.Profile)
	}
	return logging.GetLoggerFromProfile(SystemProfileHuman)
}

// logSummaryAndCheckErrors logs the generation summary and returns an error if any errors occurred.
func logSummaryAndCheckErrors(logger logging.Logger, generated, skipped, errors int) error {
	logging.Fluent(logger).Info(fmt.Sprintf("Summary: Generated %d, Skipped %d, Errors %d", generated, skipped, errors)).Log()
	if errors > 0 {
		return errfmt.Errorf("generation completed with %d errors", errors)
	}
	return nil
}
