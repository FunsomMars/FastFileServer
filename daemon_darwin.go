//go:build darwin

package main

import "golang.org/x/sys/unix"

// platformFork wraps fork(2) on Darwin. Darwin does not expose fork() in the
// Go syscall package, but the syscall is still present in the kernel (number
// 2) and accessible via unix.Syscall.
func platformFork() (int, error) {
	pid, _, errno := unix.Syscall(unix.SYS_FORK, 0, 0, 0)
	if errno != 0 {
		return -1, errno
	}
	return int(pid), nil
}

// platformDup2 duplicates src onto dst. unix.Dup2 is implemented on darwin.
func platformDup2(src, dst uintptr) error {
	return unix.Dup2(int(src), int(dst))
}

// platformSetsid creates a new session.
func platformSetsid() error {
	if _, _, errno := unix.Syscall(unix.SYS_SETSID, 0, 0, 0); errno != 0 {
		return errno
	}
	return nil
}
