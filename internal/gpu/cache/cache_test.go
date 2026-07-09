package cache

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

// fakeCollector is a hand-written gpu.Collector fake that counts how many
// times each method is invoked so tests can assert the cache only reaches
// through to the inner collector when it is supposed to.
type fakeCollector struct {
	mu sync.Mutex

	devices []gpu.Device

	countErr  error
	deviceErr error

	deviceCountCalls int
	deviceCalls      int

	initCalls     int
	shutdownCalls int
	backendCalls  int

	initErr     error
	shutdownErr error
	backend     string
}

func (f *fakeCollector) Init() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.initCalls++
	return f.initErr
}

func (f *fakeCollector) Shutdown() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shutdownCalls++
	return f.shutdownErr
}

func (f *fakeCollector) DeviceCount() (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deviceCountCalls++
	if f.countErr != nil {
		return 0, f.countErr
	}
	return len(f.devices), nil
}

func (f *fakeCollector) Device(i int) (*gpu.Device, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deviceCalls++
	if f.deviceErr != nil {
		return nil, f.deviceErr
	}
	if i < 0 || i >= len(f.devices) {
		return nil, errors.New("fake: index out of range")
	}
	d := f.devices[i]
	return &d, nil
}

func (f *fakeCollector) Backend() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.backendCalls++
	return f.backend
}

func (f *fakeCollector) snapshot() (dc, dev int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deviceCountCalls, f.deviceCalls
}

// fakeClock provides a mutable time source for driving TTL expiry.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newFake(devices ...gpu.Device) *fakeCollector {
	return &fakeCollector{devices: devices, backend: "fake"}
}

func sampleDevices() []gpu.Device {
	return []gpu.Device{
		{Index: 0, UUID: "GPU-0", Name: "Fake 0", ThrottleReasons: []string{"power"}},
		{Index: 1, UUID: "GPU-1", Name: "Fake 1"},
	}
}

func TestCached_readsWithinTTL_fetchInnerOnce(t *testing.T) {
	// Given a cache wrapping a counting fake with two devices and a fixed clock.
	inner := newFake(sampleDevices()...)
	clock := &fakeClock{now: time.Unix(1000, 0)}
	c := newWithClock(inner, time.Minute, clock.Now)

	// When we read twice within the TTL window.
	first, err := c.DeviceCount()
	if err != nil {
		t.Fatalf("first DeviceCount: %v", err)
	}
	dev, err := c.Device(1)
	if err != nil {
		t.Fatalf("Device(1): %v", err)
	}
	second, err := c.DeviceCount()
	if err != nil {
		t.Fatalf("second DeviceCount: %v", err)
	}

	// Then the inner collector is queried exactly once for the whole snapshot.
	if first != 2 || second != 2 {
		t.Fatalf("expected DeviceCount 2, got first=%d second=%d", first, second)
	}
	if dev.UUID != "GPU-1" {
		t.Fatalf("expected GPU-1, got %q", dev.UUID)
	}
	dc, devCalls := inner.snapshot()
	if dc != 1 {
		t.Fatalf("expected inner DeviceCount called once, got %d", dc)
	}
	// One refresh fetches all devices: 2 devices -> 2 Device calls.
	if devCalls != 2 {
		t.Fatalf("expected inner Device called twice during single refresh, got %d", devCalls)
	}
}

func TestCached_afterTTL_refreshes(t *testing.T) {
	// Given a cache read once within TTL.
	inner := newFake(sampleDevices()...)
	clock := &fakeClock{now: time.Unix(1000, 0)}
	c := newWithClock(inner, time.Minute, clock.Now)

	if _, err := c.DeviceCount(); err != nil {
		t.Fatalf("initial DeviceCount: %v", err)
	}
	dc1, _ := inner.snapshot()
	if dc1 != 1 {
		t.Fatalf("expected 1 refresh, got %d", dc1)
	}

	// When the clock advances past the TTL and we read again.
	clock.advance(time.Minute + time.Second)
	if _, err := c.DeviceCount(); err != nil {
		t.Fatalf("post-ttl DeviceCount: %v", err)
	}

	// Then a second refresh occurs.
	dc2, _ := inner.snapshot()
	if dc2 != 2 {
		t.Fatalf("expected 2 refreshes after TTL, got %d", dc2)
	}
}

func TestCached_refreshError_notCached_retriesNextRead(t *testing.T) {
	// Given an inner collector whose DeviceCount fails.
	wantErr := errors.New("boom")
	inner := newFake(sampleDevices()...)
	inner.countErr = wantErr
	clock := &fakeClock{now: time.Unix(1000, 0)}
	c := newWithClock(inner, time.Minute, clock.Now)

	// When the first read fails.
	if _, err := c.DeviceCount(); !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped boom error, got %v", err)
	}
	dc1, _ := inner.snapshot()
	if dc1 != 1 {
		t.Fatalf("expected 1 inner call, got %d", dc1)
	}

	// And a subsequent read (still within TTL) retries because the error was not cached.
	if _, err := c.DeviceCount(); !errors.Is(err, wantErr) {
		t.Fatalf("expected retry to surface boom error, got %v", err)
	}
	dc2, _ := inner.snapshot()
	if dc2 != 2 {
		t.Fatalf("expected retry to call inner again, got %d", dc2)
	}

	// When the inner recovers, the next read succeeds and caches.
	inner.mu.Lock()
	inner.countErr = nil
	inner.mu.Unlock()
	if _, err := c.DeviceCount(); err != nil {
		t.Fatalf("expected recovery, got %v", err)
	}
	dc3, _ := inner.snapshot()
	if dc3 != 3 {
		t.Fatalf("expected recovery to refresh, got %d", dc3)
	}
	// Now cached: another read does not hit inner.
	if _, err := c.DeviceCount(); err != nil {
		t.Fatalf("cached read: %v", err)
	}
	dc4, _ := inner.snapshot()
	if dc4 != 3 {
		t.Fatalf("expected cached read (no inner call), got %d", dc4)
	}
}

func TestCached_deviceRefreshError_notCached(t *testing.T) {
	// Given DeviceCount succeeds but Device() fails during refresh.
	wantErr := errors.New("device boom")
	inner := newFake(sampleDevices()...)
	inner.deviceErr = wantErr
	clock := &fakeClock{now: time.Unix(1000, 0)}
	c := newWithClock(inner, time.Minute, clock.Now)

	if _, err := c.DeviceCount(); !errors.Is(err, wantErr) {
		t.Fatalf("expected device error, got %v", err)
	}
	dc1, _ := inner.snapshot()
	if dc1 != 1 {
		t.Fatalf("expected 1 count call, got %d", dc1)
	}

	// A subsequent read retries because the failed refresh was not cached.
	if _, err := c.Device(0); !errors.Is(err, wantErr) {
		t.Fatalf("expected retry device error, got %v", err)
	}
	dc2, _ := inner.snapshot()
	if dc2 != 2 {
		t.Fatalf("expected retry to call inner again, got %d", dc2)
	}
}

func TestCached_deviceOutOfRange_error(t *testing.T) {
	// Given a cache with two devices.
	inner := newFake(sampleDevices()...)
	clock := &fakeClock{now: time.Unix(1000, 0)}
	c := newWithClock(inner, time.Minute, clock.Now)

	// When we request an index past the end.
	if _, err := c.Device(2); err == nil {
		t.Fatal("expected out-of-range error for Device(2)")
	}
	// And a negative index.
	if _, err := c.Device(-1); err == nil {
		t.Fatal("expected out-of-range error for Device(-1)")
	}
	// A valid index still works from the same snapshot.
	d, err := c.Device(0)
	if err != nil {
		t.Fatalf("Device(0): %v", err)
	}
	if d.UUID != "GPU-0" {
		t.Fatalf("expected GPU-0, got %q", d.UUID)
	}
}

func TestCached_passThrough_initShutdownBackend(t *testing.T) {
	// Given a fake with a known backend and injected Init/Shutdown errors.
	initErr := errors.New("init boom")
	shutdownErr := errors.New("shutdown boom")
	inner := newFake(sampleDevices()...)
	inner.backend = "nvml"
	inner.initErr = initErr
	inner.shutdownErr = shutdownErr
	clock := &fakeClock{now: time.Unix(1000, 0)}
	c := newWithClock(inner, time.Minute, clock.Now)

	// When/Then Init passes through error and count.
	if err := c.Init(); !errors.Is(err, initErr) {
		t.Fatalf("expected init error passthrough, got %v", err)
	}
	if err := c.Shutdown(); !errors.Is(err, shutdownErr) {
		t.Fatalf("expected shutdown error passthrough, got %v", err)
	}
	if got := c.Backend(); got != "nvml" {
		t.Fatalf("expected backend nvml, got %q", got)
	}
	if inner.initCalls != 1 || inner.shutdownCalls != 1 || inner.backendCalls != 1 {
		t.Fatalf("expected one call each, got init=%d shutdown=%d backend=%d",
			inner.initCalls, inner.shutdownCalls, inner.backendCalls)
	}
}

func TestNew_usesWallClock(t *testing.T) {
	// Given a cache built via the public constructor (now = time.Now).
	inner := newFake(sampleDevices()...)
	c := New(inner, time.Hour)

	// When we read twice back-to-back.
	if _, err := c.DeviceCount(); err != nil {
		t.Fatalf("DeviceCount: %v", err)
	}
	if _, err := c.DeviceCount(); err != nil {
		t.Fatalf("DeviceCount: %v", err)
	}

	// Then the long TTL keeps it served from a single refresh.
	dc, _ := inner.snapshot()
	if dc != 1 {
		t.Fatalf("expected single refresh under long TTL, got %d", dc)
	}
}

func TestCached_concurrentReads_noRace(t *testing.T) {
	// Given a cache with a short TTL and a mutable clock.
	inner := newFake(sampleDevices()...)
	clock := &fakeClock{now: time.Unix(1000, 0)}
	c := newWithClock(inner, 10*time.Millisecond, clock.Now)

	// When many goroutines read while the clock advances.
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				if n, err := c.DeviceCount(); err == nil && n > 0 {
					_, _ = c.Device(0)
				}
				clock.advance(time.Millisecond)
			}
		}()
	}
	wg.Wait()

	// Then no race is detected (run with -race) and reads succeeded.
	if n, err := c.DeviceCount(); err != nil || n != 2 {
		t.Fatalf("final read n=%d err=%v", n, err)
	}
}

var _ gpu.Collector = (*Cached)(nil)
