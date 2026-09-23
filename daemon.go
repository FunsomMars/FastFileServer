//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// daemonize performs the classic double-fork to detach from the controlling
// terminal, similar to the well-known "daemon" library. After this returns,
// the current process runs in its own session with stdin connected to /dev/null
// and stdout/stderr connected to the writer passed in (usually /dev/null for
// the child, or the original fd captured before forking).
//
// The caller is responsible for setting up the log writer before calling this
// and for writing the PID file afterwards.
//
// Returns true if the current process should continue running the server.
// Returns false if the current process was the parent that just spawned a
// detached child and should exit immediately.
//
// alreadyDaemonized should be true when the caller has detected that the
// process is already detached (e.g. PID 1 parent, no controlling TTY, or
// already a session leader that was started by an init system). In that case
// we do not fork again.
func daemonize(stdin, stdout, stderr *os.File, alreadyDaemonized bool) (bool, error) {
	if alreadyDaemonized {
		// No need to fork: process is already detached by its supervisor.
		return true, nil
	}

	// First fork: parent exits so the child is reparented to init/PID 1.
	pid, err := syscall.Fork()
	if err != nil {
		return false, fmt.Errorf("first fork: %w", err)
	}
	if pid != 0 {
		// Parent: caller will print minimal info and exit.
		return false, nil
	}

	// Child: create a new session so we are no longer tied to the controlling
	// terminal and have no TTY under ^C / window close.
	if _, _, err := syscall.Syscall(syscall.SYS_SETSID, 0, 0, 0); err != 0 {
		return false, fmt.Errorf("setsid: %v", err)
	}

	// Second fork: ensure we can never reacquire a controlling TTY by accident.
	pid, err = syscall.Fork()
	if err != nil {
		return false, fmt.Errorf("second fork: %w", err)
	}
	if pid != 0 {
		// First child exits so the grand-child is reparented again.
		os.Exit(0)
	}

	// Grand-child: replace std fds.
	if err := dupTo(stdin, os.Stdin.Fd()); err != nil {
		return false, fmt.Errorf("dup stdin: %w", err)
	}
	if err := dupTo(stdout, os.Stdout.Fd()); err != nil {
		return false, fmt.Errorf("dup stdout: %w", err)
	}
	if err := dupTo(stderr, os.Stderr.Fd()); err != nil {
		return false, fmt.Errorf("dup stderr: %w", err)
	}

	return true, nil
}

// dupTo replaces the destination fd with a dup of the source file, so the new
// process std fds point to the desired target (e.g. /dev/null or a log file).
func dupTo(src *os.File, dst uintptr) error {
	if src == nil {
		return nil
	}
	// Ensure src is at least dst by using Dup2. Using the file directly is fine
	// because we want src's underlying descriptor.
	return syscall.Dup2(int(src.Fd()), int(dst))
}

// writePIDFile atomically writes the current PID to the given path, creating
// parent directories as needed.
func writePIDFile(path string) error {
	if path == "" {
		return nil
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir pid dir: %w", err)
		}
	}
	pid := os.Getpid()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(fmt.Sprintf("%d\n", pid)), 0o644); err != nil {
		return fmt.Errorf("write pid tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename pid: %w", err)
	}
	return nil
}

// alreadyDetached reports whether the process appears to already be running in
// a detached environment. This avoids redundant double-forks under systemd,
// Docker, or when launched via `nohup`/tty disowning. TTY detection uses
// /dev/tty which does not require cgo.
func alreadyDetached() bool {
	// PID 1 in the calling namespace => init-style supervisor.
	if os.Getpid() == 1 {
		return true
	}
	// Running as a session leader that was forked from a non-tty caller.
	// We try to open /dev/tty: if it fails, we likely have no controlling TTY.
	if _, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err != nil {
		return true
	}
	return false
}

// readExistingPID returns the PID stored in path, or an error if no usable
// value is present. Used by the launcher to refuse double-start.
func readExistingPID(path string) (int, error) {
	if path == "" {
		return 0, fmt.Errorf("no pidfile path")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("bad pidfile contents: %q", string(b))
	}
	return pid, nil
}

// logFileDisplay returns a human-friendly description of where the access log
// is going: either the configured path or a marker for stdout.
func logFileDisplay(p string) string {
	if p == "" {
		return "(stdout)"
	}
	return p
}
