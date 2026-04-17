package cache

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/spf13/afero"

	"github.com/niklod/lazylab/internal/config"
)

// LogFunc is an optional sink for debug-level cache events. nil disables logging.
type LogFunc func(format string, args ...any)

type options struct {
	logf LogFunc
	now  func() time.Time
}

type Option func(*options)

func WithLogger(logf LogFunc) Option {
	return func(o *options) { o.logf = logf }
}

// WithClock overrides the clock source used for staleness checks and entry
// timestamps. Intended for deterministic tests.
func WithClock(now func() time.Time) Option {
	return func(o *options) { o.now = now }
}

// Cache is an in-memory + disk stale-while-revalidate cache. Background
// refreshes update memory+disk silently — by design, they do NOT emit any
// event. Fresh data surfaces only on the next caller-driven Do call
// (see ADR 009).
type Cache struct {
	mu      sync.RWMutex
	entries map[string]*entry

	pending sync.Map

	fs  afero.Fs
	dir string
	ttl time.Duration

	logf LogFunc
	now  func() time.Time

	// rootCtx is the parent context for background refresh goroutines; canceling
	// it during Shutdown propagates cancellation to in-flight loaders. See ADR 009.
	rootCtx      context.Context //nolint:containedctx // shutdown cancellation of background refreshes
	cancel       context.CancelFunc
	shutdownOnce sync.Once
	wg           sync.WaitGroup

	dirOnce sync.Once
	dirErr  error
}

func New(cfg config.CacheConfig, fsys afero.Fs, opts ...Option) *Cache {
	o := &options{now: time.Now}
	for _, opt := range opts {
		opt(o)
	}

	rootCtx, cancel := context.WithCancel(context.Background())

	return &Cache{
		entries: make(map[string]*entry),
		fs:      fsys,
		dir:     cfg.Directory,
		ttl:     time.Duration(cfg.TTL) * time.Second,
		logf:    o.logf,
		now:     o.now,
		rootCtx: rootCtx,
		cancel:  cancel,
	}
}

// Invalidate removes every entry whose key starts with prefix, from both
// memory and disk.
func (c *Cache) Invalidate(prefix string) {
	c.mu.Lock()
	var matched []string
	for k := range c.entries {
		if strings.HasPrefix(k, prefix) {
			matched = append(matched, k)
			delete(c.entries, k)
		}
	}
	c.mu.Unlock()

	c.removeDiskFiles(matched, "invalidate")
	if len(matched) > 0 {
		c.debugf("invalidated %d entries with prefix %q", len(matched), prefix)
	}
}

func (c *Cache) InvalidateMR(projectID, mrIID int) {
	prefixes := []string{
		MakeKey("mr_list", projectID) + ":",
		MakeKey("mr", projectID, mrIID),
		MakeKey("mr_approvals", projectID, mrIID),
		MakeKey("mr_discussions", projectID, mrIID),
		MakeKey("mr_changes", projectID, mrIID),
		MakeKey("pipeline_latest", projectID, mrIID),
		MakeKey("pipeline_detail", projectID, mrIID),
	}
	for _, p := range prefixes {
		c.Invalidate(p)
	}
}

func (c *Cache) InvalidateAll() {
	c.mu.Lock()
	keys := make([]string, 0, len(c.entries))
	for k := range c.entries {
		keys = append(keys, k)
	}
	c.entries = make(map[string]*entry)
	c.mu.Unlock()

	c.removeDiskFiles(keys, "invalidate-all")
	if len(keys) > 0 {
		c.debugf("invalidated all %d entries", len(keys))
	}
}

// Shutdown cancels any in-flight background refreshes and waits for them to
// drain. Safe to call multiple times; subsequent calls are no-ops except for
// ctx enforcement. Honors ctx for the wait deadline.
//
// The mu.Lock around cancel synchronizes with scheduleRefresh's RLock so a
// goroutine cannot be wg.Add'd after this call's wg.Wait begins.
func (c *Cache) Shutdown(ctx context.Context) error {
	c.shutdownOnce.Do(func() {
		c.mu.Lock()
		c.cancel()
		c.mu.Unlock()
	})

	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("cache: shutdown wait: %w", ctx.Err())
	}
}

func (c *Cache) getEntry(key string) (*entry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]

	return e, ok
}

func (c *Cache) putEntry(key string, e *entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = e
}

// put stores data at key in both memory and disk. Held under c.mu so it
// serializes with Invalidate — see refreshIfPresent for the background
// variant that additionally checks the entry still exists before writing.
func (c *Cache) put(key string, data any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ensureDir()
	e := &entry{data: data, createdAt: c.now()}
	c.entries[key] = e
	if err := saveDisk(c.fs, c.dir, key, e.createdAt, data); err != nil {
		c.debugf("disk save failed for %q: %v", key, err)
	}
}

// refreshIfPresent stores background-refreshed data only if the entry is
// still in the cache. An Invalidate that ran while the loader was in-flight
// removes the entry; this check prevents the refresh from silently
// resurrecting stale (pre-mutation) data under the original key.
func (c *Cache) refreshIfPresent(key string, data any) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.entries[key]; !ok {
		return false
	}
	c.ensureDir()
	e := &entry{data: data, createdAt: c.now()}
	c.entries[key] = e
	if err := saveDisk(c.fs, c.dir, key, e.createdAt, data); err != nil {
		c.debugf("disk save failed for %q: %v", key, err)
	}

	return true
}

func (c *Cache) ensureDir() {
	c.dirOnce.Do(func() {
		c.dirErr = ensureCacheDir(c.fs, c.dir)
	})
	if c.dirErr != nil {
		c.debugf("cache dir init failed: %v", c.dirErr)
	}
}

func (c *Cache) removeDiskFiles(keys []string, reason string) {
	for _, k := range keys {
		if err := removeDiskFile(c.fs, c.dir, k); err != nil {
			c.debugf("%s disk remove failed for %q: %v", reason, k, err)
		}
	}
}

func (c *Cache) debugf(format string, args ...any) {
	if c.logf == nil {
		return
	}
	c.logf(format, args...)
}
