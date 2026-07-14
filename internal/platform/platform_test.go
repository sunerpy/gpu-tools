package platform

import (
	"testing"
)

func TestIsLinux(t *testing.T) {
	tests := []struct {
		name     string
		osValue  string
		expected bool
	}{
		{
			name:     "linux",
			osValue:  "linux",
			expected: true,
		},
		{
			name:     "windows",
			osValue:  "windows",
			expected: false,
		},
		{
			name:     "darwin",
			osValue:  "darwin",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore goos
			restoreGoos := goos
			t.Cleanup(func() { goos = restoreGoos })

			// Override goos for this test
			goos = tt.osValue

			// Test IsLinux()
			got := IsLinux()
			if got != tt.expected {
				t.Errorf("IsLinux() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCurrentOS(t *testing.T) {
	tests := []struct {
		name     string
		osValue  string
		expected string
	}{
		{
			name:     "linux",
			osValue:  "linux",
			expected: "linux",
		},
		{
			name:     "windows",
			osValue:  "windows",
			expected: "windows",
		},
		{
			name:     "darwin",
			osValue:  "darwin",
			expected: "darwin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore goos
			restoreGoos := goos
			t.Cleanup(func() { goos = restoreGoos })

			// Override goos for this test
			goos = tt.osValue

			// Test CurrentOS()
			got := CurrentOS()
			if got != tt.expected {
				t.Errorf("CurrentOS() = %q, want %q", got, tt.expected)
			}
		})
	}
}
