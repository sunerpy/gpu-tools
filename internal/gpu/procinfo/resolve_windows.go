//go:build windows

package procinfo

// Resolve is a no-op on Windows: the NVML purego backend is not shipped there
// (nvidia-smi handles Windows) and there is no /proc to read. It returns empty
// strings so per-process resolution stays best-effort and this package still
// compiles for `make build-check` (R8).
func Resolve(pid int) (name, usr string) {
	_ = pid
	return "", ""
}
