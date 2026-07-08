package mocks

import (
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

func TestMockCollector_recordsCollectorInteractions(t *testing.T) {
	ctrl := gomock.NewController(t)
	collector := NewMockCollector(ctrl)
	device := &gpu.Device{Index: 2, UUID: "GPU-test"}

	collector.EXPECT().Init().Return(nil)
	collector.EXPECT().Shutdown().Return(nil)
	collector.EXPECT().DeviceCount().Return(1, nil)
	collector.EXPECT().Device(2).Return(device, nil)
	collector.EXPECT().Backend().Return("mock")

	requireNoError(t, collector.Init())
	requireNoError(t, collector.Shutdown())
	count, countErr := collector.DeviceCount()
	requireNoError(t, countErr)
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}
	gotDevice, deviceErr := collector.Device(2)
	requireNoError(t, deviceErr)
	if gotDevice != device {
		t.Fatalf("expected device %#v, got %#v", device, gotDevice)
	}
	if gotBackend := collector.Backend(); gotBackend != "mock" {
		t.Fatalf("expected backend mock, got %q", gotBackend)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
