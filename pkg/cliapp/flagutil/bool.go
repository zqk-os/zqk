package flagutil

import (
	"strconv"

	"github.com/spf13/pflag"
)

// IsTrue reports whether a parsed flag value is a valid true boolean.
func IsTrue(f *pflag.Flag) bool {
	if f == nil {
		return false
	}
	v, err := strconv.ParseBool(f.Value.String())
	return err == nil && v
}
