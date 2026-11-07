//go:build !windows && !linux

package clientbundle

// memoryTotalBytes returns physical memory size, or 0 when unavailable.
func memoryTotalBytes() uint64 { return 0 }
