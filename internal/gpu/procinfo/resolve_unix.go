//go:build unix

package procinfo

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

var statUID = func(path string) (uint32, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return stat.Uid, true
}

// Resolve maps a PID to its process name (from /proc/<pid>/comm) and owning
// username. On any failure — missing pid, unreadable comm, unstattable dir — it
// returns empty strings and never errors, keeping per-process GPU usage
// best-effort. When uid lookup fails or the environment is numeric-only, the
// User falls back to the decimal uid string.
func Resolve(pid int) (name, usr string) {
	base := filepath.Join(procRoot, strconv.Itoa(pid))
	comm, err := os.ReadFile(filepath.Join(base, "comm"))
	if err != nil {
		return "", ""
	}
	name = strings.TrimRight(string(comm), "\n")

	uid, ok := statUID(base)
	if !ok {
		return name, ""
	}
	uidStr := strconv.FormatUint(uint64(uid), 10)
	if u, err := lookupUserID(uidStr); err == nil && u.Username != "" {
		return name, u.Username
	}
	return name, uidStr
}
