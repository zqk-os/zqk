package cli

import (
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/spf13/cobra"
)

// ladders without swallowing errors.

// FlagBag reads cobra flags with first-error retention. Later getters short-circuit
// after an error so call sites stay linear:
//
//	var f clipkg.FlagBag
//	channel := f.String(cmd, "channel")
//	noAck := f.Bool(cmd, "no-ack")
//	if err := f.Err(); err != nil {
//		return err
//	}
//
// Use a value (`var f FlagBag`) or a non-nil pointer from the caller. Methods do not
// re-check the receiver for nil after construction — same posture as when.Try.
type FlagBag struct {
	err error
}

// Err returns the first flag-read error (nil if all getters succeeded).
func (b *FlagBag) Err() error {
	return b.err
}

func (b *FlagBag) fail(name string, err error) {
	if err == nil || b.err != nil {
		return
	}
	b.err = errfmt.Newf("flag %q", name).Wrap(err)
}

func (b *FlagBag) ready(cmd *cobra.Command) bool {
	if b.err != nil {
		return false
	}
	if cmd == nil {
		b.err = errfmt.Errorf("cobra.Command is nil")
		return false
	}
	return true
}

// String reads a string flag.
func (b *FlagBag) String(cmd *cobra.Command, name string) string {
	if !b.ready(cmd) {
		return emptyValue
	}
	v, err := cmd.Flags().GetString(name)
	b.fail(name, err)
	return v
}

// Bool reads a bool flag.
func (b *FlagBag) Bool(cmd *cobra.Command, name string) bool {
	if !b.ready(cmd) {
		return false
	}
	v, err := cmd.Flags().GetBool(name)
	b.fail(name, err)
	return v
}

// Int reads an int flag.
func (b *FlagBag) Int(cmd *cobra.Command, name string) int {
	if !b.ready(cmd) {
		return 0
	}
	v, err := cmd.Flags().GetInt(name)
	b.fail(name, err)
	return v
}

// StringArray reads a stringArray flag.
func (b *FlagBag) StringArray(cmd *cobra.Command, name string) []string {
	if !b.ready(cmd) {
		return nil
	}
	v, err := cmd.Flags().GetStringArray(name)
	b.fail(name, err)
	return v
}

// StringSlice reads a stringSlice flag.
func (b *FlagBag) StringSlice(cmd *cobra.Command, name string) []string {
	if !b.ready(cmd) {
		return nil
	}
	v, err := cmd.Flags().GetStringSlice(name)
	b.fail(name, err)
	return v
}

// Duration reads a duration flag.
func (b *FlagBag) Duration(cmd *cobra.Command, name string) time.Duration {
	if !b.ready(cmd) {
		return 0
	}
	v, err := cmd.Flags().GetDuration(name)
	b.fail(name, err)
	return v
}

// Float64 reads a float64 flag.
func (b *FlagBag) Float64(cmd *cobra.Command, name string) float64 {
	if !b.ready(cmd) {
		return 0
	}
	v, err := cmd.Flags().GetFloat64(name)
	b.fail(name, err)
	return v
}

// StringToString reads a stringToString flag.
func (b *FlagBag) StringToString(cmd *cobra.Command, name string) map[string]string {
	if !b.ready(cmd) {
		return nil
	}
	v, err := cmd.Flags().GetStringToString(name)
	b.fail(name, err)
	return v
}
