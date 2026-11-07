//go:build linux

package clientbundle

import "syscall"

// memoryTotalBytes returns physical memory size, or 0 when unavailable.
func memoryTotalBytes() uint64 {
	var info syscall.Sysinfo_t
	if err := syscall.Sysinfo(&info); err != nil {
		return 0
	}
	// Totalram is in units of mem_unit bytes.
	return uint64(info.Totalram) * uint64(info.Unit)
}
