//go:build !windows && !linux && !darwin

package main

import "syscall"

// platformFork wraps fork(2) on BSDs, where Go's syscall package exposes a
// high-level syscall.Fork() helper.
func platformFork() (int, error) {
	pid, err := syscall.Fork()
	return pid, err
}

// platformDup2 duplicates src onto dst.
func platformDup2(src, dst uintptr) error {
	return syscall.Dup2(int(src), int(dst))
}

// platformSetsid creates a new session.
func platformSetsid() error {
	if _, _, err := syscall.Syscall(syscall.SYS_SETSID, 0, 0, 0); err != 0 {
		return err
	}
	return nil
}
