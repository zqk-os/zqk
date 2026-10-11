//go:build !unix && !darwin && !linux

package callback

import (
	"io"
	"os"
)

func mmapFile(f *os.File, size int) ([]byte, error) {
	data := make([]byte, size)
	_, err := f.ReadAt(data, 0)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return data, nil
}

func munmapFile(data []byte) error {
	return nil
}
