//go:build windows

package id_generation

import "os"

func lockSequenceFile(_ *os.File) error {
	return nil
}

func unlockSequenceFile(_ *os.File) error {
	return nil
}
