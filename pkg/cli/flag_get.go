package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/errfmt"
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

// StringTrimmed reads a string flag and strips leading/trailing whitespace.
func (b *FlagBag) StringTrimmed(cmd *cobra.Command, name string) string {
	return strings.TrimSpace(b.String(cmd, name))
}

// StringTrimmedOrDefault reads a string flag, strips whitespace, and returns def if empty.
func (b *FlagBag) StringTrimmedOrDefault(cmd *cobra.Command, name, def string) string {
	v := b.StringTrimmed(cmd, name)
	if v == emptyValue {
		return def
	}
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

// IntOrDefault reads an int flag and returns def when the value is <= 0.
func (b *FlagBag) IntOrDefault(cmd *cobra.Command, name string, def int) int {
	v := b.Int(cmd, name)
	if v <= 0 {
		return def
	}
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

// StringSliceTrimmed reads a stringSlice flag, trims each entry, and drops empties.
func (b *FlagBag) StringSliceTrimmed(cmd *cobra.Command, name string) []string {
	raw := b.StringSlice(cmd, name)
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		trimmed := strings.TrimSpace(s)
		if trimmed != emptyValue {
			out = append(out, trimmed)
		}
	}
	return out
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

// DurationOrDefault reads a duration flag and returns def when the value is <= 0.
func (b *FlagBag) DurationOrDefault(cmd *cobra.Command, name string, def time.Duration) time.Duration {
	v := b.Duration(cmd, name)
	if v <= 0 {
		return def
	}
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

// ReadDataInputFlags extracts common file, data, and field flags when defined on the command.
func (b *FlagBag) ReadDataInputFlags(cmd *cobra.Command) (filePath, dataStr string, fields []string) {
	if !b.ready(cmd) {
		return
	}
	if cmd.Flags().Lookup("file") != nil {
		filePath = b.String(cmd, "file")
	}
	if cmd.Flags().Lookup("data") != nil {
		dataStr = b.String(cmd, "data")
	}
	if cmd.Flags().Lookup("field") != nil {
		fields = b.StringArray(cmd, "field")
	}
	return
}
