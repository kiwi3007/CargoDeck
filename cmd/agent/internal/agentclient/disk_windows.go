//go:build windows

package agentclient

import (
	"os"
	"syscall"
	"unsafe"
)

var getDiskFreeSpaceExW = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

func diskFree(path string) int64 {
	// Ensure path exists so the syscall has a valid directory
	_ = os.MkdirAll(path, 0755)

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return -1
	}
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	ret, _, _ := getDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if ret == 0 {
		return -1
	}
	return int64(freeBytesAvailable)
}
