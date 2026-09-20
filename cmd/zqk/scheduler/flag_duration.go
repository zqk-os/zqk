package scheduler

import (
	"time"

	"github.com/spf13/cobra"
)

// flagDuration reads a duration flag that may be registered as duration or string
// (codegen historically emitted string for DNA type "duration").
// TRACK: BLI-1785903708509306000-a6d8dc5b — codegen now emits AddDurationFlag.
func flagDuration(cmd *cobra.Command, name string, fallback time.Duration) time.Duration {
	f := cmd.Flags().Lookup(name)
	if f == nil {
		return fallback
	}
	switch f.Value.Type() {
	case "duration":
		d, err := cmd.Flags().GetDuration(name)
		if err != nil {
			return fallback
		}
		return d
	default:
		s, err := cmd.Flags().GetString(name)
		if err != nil || s == "" {
			return fallback
		}
		d, err := time.ParseDuration(s)
		if err != nil {
			return fallback
		}
		return d
	}
}
