package cache

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/afero"

	"github.com/niklod/lazylab/internal/config"
)

// RefreshFunc is invoked on the cache's refresh goroutine after a background
// refresh has successfully swapped a new, non-equal payload into memory+disk.
// ctx derives from the cache's rootCtx — it cancels when Shutdown runs, so
// consumers that spawn further work should honour it to drain cleanly.
// The consumer must not block — the expected pattern is to hand off to the
// TUI main loop via gocui.Update and return immediately. See ADR 021.
//
// Declared as a named type (not an alias) so signature drift at the call
// site produces a compile error rather than a silent no-op.
type RefreshFunc func(ctx context.Context, namespace, key string)

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
// refreshes that change stored data fire the RefreshFunc installed via
// SetOnRefresh so consumers (the TUI) can fan out a selective re-render
// of views whose displayed namespace matches (see ADR 021 superseding
// ADR 009).
type Cache struct {
	mu      sync.RWMutex
	entries map[string]*entry

	pending sync.Map

	fs  afero.Fs
	dir string
	ttl time.Duration

	logf LogFunc
	now  func() time.Time

	// onRefresh is read on every successful background refresh and written
	// at most once at TUI startup. atomic.Pointer avoids contending with mu
	// on the hot refresh path.
	onRefresh atomic.Pointer[RefreshFunc]

	// rootCtx is the parent context for background refresh goroutines; canceling
	// it during Shutdown propagates cancellation to in-flight loaders.
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
		MakeKey("mr_conversation", projectID, mrIID),
		MakeKey("mr_changes", projectID, mrIID),
		MakeKey("mr_pipeline", projectID, mrIID),
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

// put stores data at key in both memory and disk. The memory swap happens
// under c.mu so it serializes with Invalidate; the disk write is moved
// outside the lock so concurrent invalidations do not queue behind the
// syscall. See refreshIfPresent for the background-refresh variant that
// additionally checks the entry still exists before writing.
func (c *Cache) put(key string, data any) {
	c.mu.Lock()
	e := &entry{data: data, createdAt: c.now()}
	c.entries[key] = e
	c.mu.Unlock()

	c.ensureDir()
	if err := saveDisk(c.fs, c.dir, key, e.createdAt, data); err != nil {
		c.debugf("disk save failed for %q: %v", key, err)
	}
}

// SetOnRefresh installs the callback invoked after a successful background
// refresh whose payload differs from the previous entry. Passing nil clears
// any previous callback. Safe to call concurrently; the callback itself must
// not block the cache goroutine.
func (c *Cache) SetOnRefresh(fn RefreshFunc) {
	if fn == nil {
		c.onRefresh.Store(nil)

		return
	}
	c.onRefresh.Store(&fn)
}

func (c *Cache) fireRefresh(ctx context.Context, namespace, key string) {
	p := c.onRefresh.Load()
	if p == nil {
		return
	}
	(*p)(ctx, namespace, key)
}

// refreshIfPresent stores background-refreshed data only if the entry is
// still in the cache. An Invalidate that ran while the loader was in-flight
// removes the entry; this check prevents the refresh from silently
// resurrecting stale (pre-mutation) data under the original key.
//
// Returns (stored, changed): stored is false when the entry was invalidated
// mid-flight; changed is true when the swapped-in payload differs from the
// previous one (reflect.DeepEqual). Callers use changed to skip firing the
// OnRefresh callback on no-op repaints — see ADR 021.
//
// Disk I/O happens OUTSIDE c.mu: the write lock is released as soon as the
// memory swap lands, so a concurrent Invalidate / InvalidateMR does not
// serialize behind the disk syscall (which can be tens of KB on mr_list).
func (c *Cache) refreshIfPresent(key string, data any) (stored, changed bool) {
	c.mu.Lock()
	old, ok := c.entries[key]
	if !ok {
		c.mu.Unlock()

		return false, false
	}
	changed = !reflect.DeepEqual(old.data, data)
	e := &entry{data: data, createdAt: c.now()}
	c.entries[key] = e
	c.mu.Unlock()

	c.ensureDir()
	if err := saveDisk(c.fs, c.dir, key, e.createdAt, data); err != nil {
		c.debugf("disk save failed for %q: %v", key, err)
	}

	return true, changed
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
