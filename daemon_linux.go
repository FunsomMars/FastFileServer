//go:build linux

package main

import "golang.org/x/sys/unix"

// platformFork performs a fork(2)-equivalent on Linux using clone(2) with
// SIGCHLD, which is portable across Linux architectures (amd64, arm64, 386,
// riscv64, ...). Linux/amd64 still exports SYS_FORK, but Linux/arm64 only
// exposes clone(2), so we use clone everywhere for consistency.
func platformFork() (int, error) {
	pid, _, errno := unix.Syscall(unix.SYS_CLONE, uintptr(unix.SIGCHLD), 0, 0)
	if errno != 0 {
		return -1, errno
	}
	return int(pid), nil
}

// platformDup2 duplicates src onto dst. On Linux the dup2 syscall is
// deprecated but unix.Dup2 wraps dup3(2) for forward compatibility and is
// available on all Linux architectures (amd64, arm64, ...).
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
