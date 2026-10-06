package glfw

import (
	"fmt"
	"golang.org/x/sys/windows"
	"unsafe"
)

const (
	clipboardUnicodeText  = 13 // CF_UNICODETEXT.
	clipboardUTF16Bytes   = 2
	clipboardMaximumUnits = 1 << 20
)

var (
	clipboardUser32    = windows.NewLazySystemDLL("user32.dll")
	clipboardKernel32  = windows.NewLazySystemDLL("kernel32.dll")
	clipboardOpen      = clipboardUser32.NewProc("OpenClipboard")
	clipboardClose     = clipboardUser32.NewProc("CloseClipboard")
	clipboardAvailable = clipboardUser32.NewProc("IsClipboardFormatAvailable")
	clipboardData      = clipboardUser32.NewProc("GetClipboardData")
	clipboardLock      = clipboardKernel32.NewProc("GlobalLock")
	clipboardUnlock    = clipboardKernel32.NewProc("GlobalUnlock")
	clipboardSize      = clipboardKernel32.NewProc("GlobalSize")
)

func readClipboardText() (string, error) {
	if ok, _, err := clipboardOpen.Call(0); ok == 0 {
		return "", fmt.Errorf("open clipboard: %w", err)
	}
	defer clipboardClose.Call()
	if ok, _, _ := clipboardAvailable.Call(clipboardUnicodeText); ok == 0 {
		return "", nil
	}
	handle, _, err := clipboardData.Call(clipboardUnicodeText)
	if handle == 0 {
		return "", fmt.Errorf("read clipboard: %w", err)
	}
	size, _, err := clipboardSize.Call(handle)
	if size == 0 {
		return "", fmt.Errorf("clipboard size: %w", err)
	}
	address, _, err := clipboardLock.Call(handle)
	if address == 0 {
		return "", fmt.Errorf("lock clipboard: %w", err)
	}
	defer clipboardUnlock.Call(handle)
	text := unsafe.Slice((*uint16)(unsafe.Pointer(address)), min(size/clipboardUTF16Bytes, clipboardMaximumUnits))
	return windows.UTF16ToString(text), nil
}
