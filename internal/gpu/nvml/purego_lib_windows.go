//go:build windows

package nvml

import "errors"

// newPuregoLib is unavailable on Windows: purego's dlopen is POSIX-only and the
// nvml.dll FFI path is intentionally not shipped in v1. The nvidia-smi backend
// handles Windows. Returning an error makes newCollector() map to
// gpu.ErrBackendUnavailable, so the registry falls back to nvidia-smi.
func newPuregoLib() (nvmlLib, error) {
	return nil, errors.New("nvml purego backend not supported on windows; use nvidia-smi backend")
}
