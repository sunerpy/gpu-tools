package nvml

import (
	"errors"
	"testing"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

func TestNewCollector_returnsErrBackendUnavailable_whenNVMLSharedLibraryIsMissing(t *testing.T) {
	// Given

	// When
	collector, err := newCollector()

	// Then
	if collector != nil {
		t.Fatalf("expected nil collector, got %#v", collector)
	}
	if !errors.Is(err, gpu.ErrBackendUnavailable) {
		t.Fatalf("expected ErrBackendUnavailable, got %v", err)
	}
}
