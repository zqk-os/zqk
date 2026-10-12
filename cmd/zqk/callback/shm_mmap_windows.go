//go:build windows

package callback

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	shmBitsPerWord = 32
	shmLowMask     = 0xFFFFFFFF
)

func mmapFile(f *os.File, size int) ([]byte, error) {
	if size == 0 {
		return []byte{}, nil
	}

	sizeLow := uint32(uint64(size) & shmLowMask)
	sizeHigh := uint32(uint64(size) >> shmBitsPerWord)

	hMap, err := syscall.CreateFileMapping(
		syscall.Handle(f.Fd()),
		nil,
		syscall.PAGE_READWRITE,
		sizeHigh,
		sizeLow,
		nil,
	)
	if err != nil {
		return nil, err
	}
	defer syscall.CloseHandle(hMap)

	addr, err := syscall.MapViewOfFile(
		hMap,
		syscall.FILE_MAP_READ|syscall.FILE_MAP_WRITE,
		0,
		0,
		uintptr(size),
	)
	if err != nil {
		return nil, err
	}

	rawPtr := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	return unsafe.Slice((*byte)(rawPtr), size), nil
}

func munmapFile(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	addr := *(*uintptr)(unsafe.Pointer(&data))
	return syscall.UnmapViewOfFile(addr)
}
