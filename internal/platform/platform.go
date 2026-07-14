package platform

import "runtime"

var goos = runtime.GOOS

// IsLinux returns true if the current operating system is Linux.
func IsLinux() bool {
	return goos == "linux"
}

// CurrentOS returns the current operating system as a string.
func CurrentOS() string {
	return goos
}
