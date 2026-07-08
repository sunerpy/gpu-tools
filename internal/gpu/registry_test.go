package gpu

import (
	"errors"
	"testing"

	"github.com/sunerpy/gpu-tools/core"
)

var errTestBackend = errors.New("test backend failure")

type fakeCollector struct {
	backend string
}

func (c *fakeCollector) Init() error { return nil }

func (c *fakeCollector) Shutdown() error { return nil }

func (c *fakeCollector) DeviceCount() (int, error) { return 0, nil }

func (c *fakeCollector) Device(_ int) (*Device, error) { return nil, nil }

func (c *fakeCollector) Backend() string { return c.backend }

func TestSelect_returnsLowestPriorityAvailableCollector_whenBackendIsAuto(t *testing.T) {
	// Given
	resetRegistry()
	register("slow", 20, func() (Collector, error) {
		return &fakeCollector{backend: "slow"}, nil
	})
	register("fast", 10, func() (Collector, error) {
		return &fakeCollector{backend: "fast"}, nil
	})

	// When
	got, err := Select("auto")

	// Then
	requireNoError(t, err)
	requireBackend(t, got, "fast")
}

func TestSelect_skipsUnavailableCollector_whenBackendIsAuto(t *testing.T) {
	// Given
	resetRegistry()
	register("missing", 10, func() (Collector, error) {
		return nil, ErrBackendUnavailable
	})
	register("available", 20, func() (Collector, error) {
		return &fakeCollector{backend: "available"}, nil
	})

	// When
	got, err := Select("auto")

	// Then
	requireNoError(t, err)
	requireBackend(t, got, "available")
}

func TestSelect_returnsErrNoBackend_whenAutoBackendsAreUnavailable(t *testing.T) {
	// Given
	resetRegistry()
	register("missing", 10, func() (Collector, error) {
		return nil, ErrBackendUnavailable
	})
	register("also-missing", 20, func() (Collector, error) {
		return nil, ErrBackendUnavailable
	})

	// When
	got, err := Select("auto")

	// Then
	if got != nil {
		t.Fatalf("expected nil collector, got %#v", got)
	}
	requireErrorIs(t, err, ErrNoBackend)
}

func TestSelect_returnsErrNoBackend_whenRegistryIsEmpty(t *testing.T) {
	// Given
	resetRegistry()

	// When
	got, err := Select("auto")

	// Then
	if got != nil {
		t.Fatalf("expected nil collector, got %#v", got)
	}
	requireErrorIs(t, err, ErrNoBackend)
}

func TestSelect_returnsUnknownBackendError_whenBackendNameIsUnknown(t *testing.T) {
	// Given
	resetRegistry()
	register("known", 10, func() (Collector, error) {
		return &fakeCollector{backend: "known"}, nil
	})

	// When
	got, err := Select("bogus")

	// Then
	if got != nil {
		t.Fatalf("expected nil collector, got %#v", got)
	}
	var unknown *UnknownBackendError
	if !errors.As(err, &unknown) {
		t.Fatalf("expected UnknownBackendError, got %T: %v", err, err)
	}
	if unknown.Name != "bogus" {
		t.Fatalf("expected unknown backend name bogus, got %q", unknown.Name)
	}
	if unknown.Error() != `unknown gpu backend "bogus"` {
		t.Fatalf("unexpected error string: %q", unknown.Error())
	}
}

func TestSelect_returnsNamedCollector_whenBackendNameIsRegistered(t *testing.T) {
	// Given
	resetRegistry()
	register("other", 10, func() (Collector, error) {
		return &fakeCollector{backend: "other"}, nil
	})
	register("wanted", 20, func() (Collector, error) {
		return &fakeCollector{backend: "wanted"}, nil
	})

	// When
	got, err := Select("wanted")

	// Then
	requireNoError(t, err)
	requireBackend(t, got, "wanted")
}

func TestSelect_returnsCtorErrorAsIs_whenSpecificBackendCtorFails(t *testing.T) {
	// Given
	resetRegistry()
	register("broken", 10, func() (Collector, error) {
		return nil, errTestBackend
	})

	// When
	got, err := Select("broken")

	// Then
	if got != nil {
		t.Fatalf("expected nil collector, got %#v", got)
	}
	requireErrorIs(t, err, errTestBackend)
}

func TestSelect_returnsCtorError_whenHighestPriorityAutoBackendFailsForOtherReason(t *testing.T) {
	// Given
	resetRegistry()
	register("broken", 10, func() (Collector, error) {
		return nil, errTestBackend
	})
	register("available", 20, func() (Collector, error) {
		return &fakeCollector{backend: "available"}, nil
	})

	// When
	got, err := Select("auto")

	// Then
	if got != nil {
		t.Fatalf("expected nil collector, got %#v", got)
	}
	requireErrorIs(t, err, errTestBackend)
}

func TestDefaultFactory_selectsConfiguredBackend_whenBackendIsDefaultOrOverride(t *testing.T) {
	// Given
	resetRegistry()
	register("auto-choice", 10, func() (Collector, error) {
		return &fakeCollector{backend: "auto-choice"}, nil
	})
	register("override", 20, func() (Collector, error) {
		return &fakeCollector{backend: "override"}, nil
	})

	// When
	defaultCollector, defaultErr := DefaultFactory(core.Config{Backend: core.BackendAuto})
	overrideCollector, overrideErr := DefaultFactory(core.Config{Backend: "override"})

	// Then
	requireNoError(t, defaultErr)
	requireBackend(t, defaultCollector, "auto-choice")
	requireNoError(t, overrideErr)
	requireBackend(t, overrideCollector, "override")
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func requireErrorIs(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("expected error %v to match %v", err, target)
	}
}

func requireBackend(t *testing.T, collector Collector, want string) {
	t.Helper()
	if collector == nil {
		t.Fatal("expected collector, got nil")
	}
	if got := collector.Backend(); got != want {
		t.Fatalf("expected backend %q, got %q", want, got)
	}
}
