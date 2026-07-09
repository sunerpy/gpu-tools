// Package cache provides a TTL caching wrapper around a gpu.Collector.
//
// The wrapper takes a single consistent snapshot of all devices per refresh:
// the first read after expiry calls the inner DeviceCount() once and then
// Device(0..n-1) once each, stores the resulting []gpu.Device with a fetch
// timestamp, and serves every read within the TTL window from that snapshot
// without touching the inner collector. Errors during a refresh are never
// cached — they propagate to the caller and leave the cache invalid so the
// next read retries. Init, Shutdown and Backend pass straight through.
package cache

import (
	"fmt"
	"sync"
	"time"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

// Cached wraps a gpu.Collector and serves DeviceCount/Device from a snapshot
// refreshed at most once per ttl. All snapshot state is mutex-guarded.
type Cached struct {
	inner gpu.Collector
	ttl   time.Duration
	now   func() time.Time

	mu        sync.Mutex
	snapshot  []gpu.Device
	fetchedAt time.Time
	valid     bool
}

var _ gpu.Collector = (*Cached)(nil)

// New returns a Cached wrapping inner with the given ttl, using time.Now as
// its clock.
func New(inner gpu.Collector, ttl time.Duration) *Cached {
	return newWithClock(inner, ttl, time.Now)
}

// newWithClock is the test seam: it lets callers inject a deterministic clock.
func newWithClock(inner gpu.Collector, ttl time.Duration, now func() time.Time) *Cached {
	return &Cached{inner: inner, ttl: ttl, now: now}
}

// Init passes through to the inner collector.
func (c *Cached) Init() error { return c.inner.Init() }

// Shutdown passes through to the inner collector.
func (c *Cached) Shutdown() error { return c.inner.Shutdown() }

// Backend passes through to the inner collector.
func (c *Cached) Backend() string { return c.inner.Backend() }

// DeviceCount returns the number of devices in the current snapshot,
// refreshing first if the snapshot is missing or expired.
func (c *Cached) DeviceCount() (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureFreshLocked(); err != nil {
		return 0, err
	}
	return len(c.snapshot), nil
}

// Device returns a copy pointer to the device at index i from the current
// snapshot, refreshing first if the snapshot is missing or expired. It returns
// an error if i is out of range.
func (c *Cached) Device(i int) (*gpu.Device, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureFreshLocked(); err != nil {
		return nil, err
	}
	if i < 0 || i >= len(c.snapshot) {
		return nil, fmt.Errorf("cache: device index %d out of range [0,%d)", i, len(c.snapshot))
	}
	d := c.snapshot[i]
	return &d, nil
}

// ensureFreshLocked refreshes the snapshot when it is invalid or older than
// ttl. It must be called with c.mu held. A failed refresh leaves the cache
// invalid so the next call retries; the error is never cached.
func (c *Cached) ensureFreshLocked() error {
	if c.valid && c.now().Sub(c.fetchedAt) < c.ttl {
		return nil
	}
	return c.refreshLocked()
}

// refreshLocked pulls a fresh snapshot from the inner collector. It must be
// called with c.mu held. On any inner error it invalidates the cache and
// returns the error without storing it.
func (c *Cached) refreshLocked() error {
	c.valid = false
	c.snapshot = nil

	n, err := c.inner.DeviceCount()
	if err != nil {
		return fmt.Errorf("cache: refresh device count: %w", err)
	}

	devices := make([]gpu.Device, 0, n)
	for i := range n {
		d, derr := c.inner.Device(i)
		if derr != nil {
			return fmt.Errorf("cache: refresh device %d: %w", i, derr)
		}
		devices = append(devices, *d)
	}

	c.snapshot = devices
	c.fetchedAt = c.now()
	c.valid = true
	return nil
}
