package topo

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// ErrToolNotInstalled is returned when nvidia-smi cannot be located on PATH.
var ErrToolNotInstalled = errors.New("nvidia-smi not installed")

// execRunner abstracts the external command execution seam so tests can inject
// a fake runner instead of shelling out to nvidia-smi.
type execRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type osExecRunner struct{}

func (r osExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("run %s: %w", name, err)
	}
	return out, nil
}

// lookPath is a package-level indirection over exec.LookPath so tests can
// override nvidia-smi resolution.
var lookPath = func(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("look path %s: %w", name, err)
	}
	return path, nil
}

var defaultRunner execRunner = osExecRunner{}
