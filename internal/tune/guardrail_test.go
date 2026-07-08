package tune

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuardrail_nonTestTuneFilesDoNotImportOsExec(t *testing.T) {
	// Given
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob tune files: %v", err)
	}

	// When / Then
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if strings.Contains(string(data), "os/exec") {
			t.Fatalf("%s must not import os/exec; tune recommendations are read-only", file)
		}
	}
}
