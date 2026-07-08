package gpu

import (
	"errors"
	"fmt"
	"sort"

	"github.com/sunerpy/gpu-tools/core"
)

type backendRegistration struct {
	name     string
	priority int
	ctor     func() (Collector, error)
}

type UnknownBackendError struct {
	Name string
}

func (e *UnknownBackendError) Error() string {
	return fmt.Sprintf("unknown gpu backend %q", e.Name)
}

var registeredBackends []backendRegistration

func register(name string, priority int, ctor func() (Collector, error)) {
	registeredBackends = append(registeredBackends, backendRegistration{
		name:     name,
		priority: priority,
		ctor:     ctor,
	})
	sort.SliceStable(registeredBackends, func(i, j int) bool {
		return registeredBackends[i].priority < registeredBackends[j].priority
	})
}

func Register(name string, priority int, ctor func() (Collector, error)) {
	register(name, priority, ctor)
}

func Select(backend string) (Collector, error) {
	if backend == core.BackendAuto {
		return selectAuto()
	}
	return selectNamed(backend)
}

var DefaultFactory = func(cfg core.Config) (Collector, error) {
	return Select(cfg.Backend)
}

func selectAuto() (Collector, error) {
	for _, registered := range registeredBackends {
		collector, err := registered.ctor()
		if err == nil {
			return collector, nil
		}
		if errors.Is(err, ErrBackendUnavailable) {
			continue
		}
		return nil, err
	}
	return nil, ErrNoBackend
}

func selectNamed(backend string) (Collector, error) {
	for _, registered := range registeredBackends {
		if registered.name == backend {
			return registered.ctor()
		}
	}
	return nil, &UnknownBackendError{Name: backend}
}
