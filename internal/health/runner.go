package health

import (
	"context"
	"fmt"
	"os/exec"
)

// execRunner is the external-tool seam. Probes call read-only query commands
// through it so tests can inject canned output without touching the host.
type execRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// osExecRunner is the production execRunner backed by os/exec.
type osExecRunner struct{}

func (r osExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("run %s: %w", name, err)
	}
	return out, nil
}

// lookPath resolves an executable in PATH. It is a package variable so tests
// can override it to simulate missing tools.
var lookPath = func(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("look path %s: %w", name, err)
	}
	return path, nil
}
