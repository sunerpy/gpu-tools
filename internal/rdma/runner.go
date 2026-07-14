package rdma

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// ErrToolNotInstalled is returned by Collect when neither ibv_devinfo nor
// ibstat can be resolved on the host.
var ErrToolNotInstalled = errors.New("no RDMA tools installed (ibv_devinfo/ibstat)")

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

var lookPath = func(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("look path %s: %w", name, err)
	}
	return path, nil
}

var defaultRunner any = osExecRunner{}
