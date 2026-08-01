//go:build windows

package syscallutil

import "os"

const (
	LockExNb = 0
	LockUn   = 0
)

func FileFlock(_ *os.File, _ int) error {
	return nil
}
