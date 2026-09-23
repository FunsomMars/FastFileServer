//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Windows does not support a clean POSIX-style double-fork without cgo or
// external helpers. We expose the same surface as daemon.go so callers do not
// have to special-case platforms; the implementation relies on the recommended
// idioms on this platform: running under NSSM / Task Scheduler / a `start /B`
// shell wrapper. See README for guidance.
//
// In -daemon mode on Windows we simply release the std streams from the
// current console (where available) so the launcher process can return to its
// shell prompt without blocking on the file server's lifetime.
func daemonize(stdin, stdout, stderr *os.File, alreadyDaemonized bool) (bool, error) {
	if stdin != nil {
		_ = stdin.Close()
	}
	if stdout != nil {
		_ = stdout.Close()
	}
	if stderr != nil {
		_ = stderr.Close()
	}
	return true, nil
}

// writePIDFile is identical across platforms.
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

func alreadyDetached() bool {
	// True under Service Control Manager (PID 1 of the service sandbox).
	return os.Getpid() == 1
}

// readExistingPID is shared across platforms; defined here so the launcher can
// detect already-running instances.
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
