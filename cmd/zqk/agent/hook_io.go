package agent

import (
	"io"
	"os"
)

// safeDrainInput reads all bytes from in, avoiding blocking on character devices (like interactive terminals).
func safeDrainInput(in io.Reader) ([]byte, error) {
	if in == nil {
		return nil, nil
	}
	if f, ok := in.(*os.File); ok {
		if stat, err := f.Stat(); err == nil && (stat.Mode()&os.ModeCharDevice) != 0 {
			return nil, nil
		}
	}
	return io.ReadAll(in)
}
