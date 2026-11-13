package main

import (
	"errors"
	"os"
	"syscall"
)

// ERROR_SHARING_VIOLATION from WinError.h; another controller owns the handle.
const windowsSharingViolation syscall.Errno = 32

func lockReservation(path string) (*os.File, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if errors.Is(err, windowsSharingViolation) {
		return nil, errReserved
	}
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}
