package main

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// diskUsage reports free/total bytes for the filesystem holding path.
func diskUsage(path string) (free, total uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	bsize := uint64(st.Bsize)
	return st.Bavail * bsize, st.Blocks * bsize, nil
}

// serviceState reports the scout user unit state, or an error if systemd
// user manager is unavailable (e.g. headless SSH without lingering).
func serviceState() (string, error) {
	out, err := exec.Command("systemctl", "--user", "is-active", "scout").Output()
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("inactive")
	}
	return strings.TrimSpace(string(out)), nil
}
