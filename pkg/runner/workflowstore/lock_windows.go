//go:build windows

package workflowstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

const (
	windowsLockfileFailImmediately = 0x00000001
	windowsLockfileExclusiveLock   = 0x00000002
	windowsErrorLockViolation      = syscall.Errno(33)
)

var (
	windowsKernel32     = syscall.NewLazyDLL("kernel32.dll")
	windowsLockFileEx   = windowsKernel32.NewProc("LockFileEx")
	windowsUnlockFileEx = windowsKernel32.NewProc("UnlockFileEx")
)

func acquirePlatformLock(ctx context.Context, file *os.File) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var overlapped syscall.Overlapped
		result, _, callErr := windowsLockFileEx.Call(
			file.Fd(),
			windowsLockfileExclusiveLock|windowsLockfileFailImmediately,
			0,
			1,
			0,
			uintptr(unsafe.Pointer(&overlapped)),
		)
		runtime.KeepAlive(&overlapped)
		if result != 0 {
			return nil
		}
		if !errors.Is(callErr, windowsErrorLockViolation) {
			return fmt.Errorf("LockFileEx: %w", callErr)
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func releasePlatformLock(file *os.File) error {
	var overlapped syscall.Overlapped
	result, _, callErr := windowsUnlockFileEx.Call(
		file.Fd(),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	runtime.KeepAlive(&overlapped)
	if result == 0 {
		return fmt.Errorf("UnlockFileEx: %w", callErr)
	}
	return nil
}

func syncDirectory(directory string) error {
	path, err := syscall.UTF16PtrFromString(directory)
	if err != nil {
		return err
	}
	handle, err := syscall.CreateFile(
		path,
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return err
	}
	return errors.Join(syscall.FlushFileBuffers(handle), syscall.CloseHandle(handle))
}
