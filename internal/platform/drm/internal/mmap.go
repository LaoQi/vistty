package internal

import (
	"syscall"
)

func Mmap(fd int, offset uint64, size uint64) ([]byte, error) {
	data, err := syscall.Mmap(fd, int64(offset), int(size), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func Munmap(data []byte) error {
	return syscall.Munmap(data)
}
