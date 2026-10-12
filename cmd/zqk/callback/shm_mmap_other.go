//go:build !unix && !darwin && !linux && !windows

package callback

import (
	"io"
	"os"
	"sync"
	"unsafe"
)

var (
	fallbackMu       sync.Mutex
	fallbackMappings = make(map[uintptr]*fallbackMapping)
)

type fallbackMapping struct {
	file *os.File
	data []byte
}

func mmapFile(f *os.File, size int) ([]byte, error) {
	data := make([]byte, size)
	_, err := f.ReadAt(data, 0)
	if err != nil && err != io.EOF {
		return nil, err
	}

	if size > 0 {
		addr := *(*uintptr)(unsafe.Pointer(&data))
		fallbackMu.Lock()
		fallbackMappings[addr] = &fallbackMapping{
			file: f,
			data: data,
		}
		fallbackMu.Unlock()
	}

	return data, nil
}

func munmapFile(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	addr := *(*uintptr)(unsafe.Pointer(&data))

	fallbackMu.Lock()
	mapping, exists := fallbackMappings[addr]
	if exists {
		delete(fallbackMappings, addr)
	}
	fallbackMu.Unlock()

	if !exists || mapping.file == nil {
		return nil
	}

	const writeOffset = 0
	_, err := mapping.file.WriteAt(mapping.data, writeOffset)
	return err
}
