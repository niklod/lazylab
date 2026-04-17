package cache

import "context"

// scheduleRefresh starts a background goroutine to refetch key via loader.
// Dedup is enforced via sync.Map: if a refresh for the same key is already
// in flight the call is a no-op. On success the refreshed value is stored
// via refreshIfPresent, which no-ops when the entry was Invalidated during
// the fetch.
//
// **No event is emitted on success** — by contract, background refreshes
// must never trigger a TUI re-render (see ADR 009).
//
// The c.mu.RLock around wg.Add synchronizes with Shutdown's cancel: Shutdown
// holds the write lock, so it cannot complete wg.Wait() while a
// scheduleRefresh call is mid-Add. Without this interlock the goroutine
// could be registered after Wait returned, leaking past Shutdown.
func (c *Cache) scheduleRefresh(key string, loader func(context.Context) (any, error)) {
	c.mu.RLock()
	if c.rootCtx.Err() != nil {
		c.mu.RUnlock()

		return
	}
	if _, loaded := c.pending.LoadOrStore(key, struct{}{}); loaded {
		c.mu.RUnlock()

		return
	}
	c.wg.Add(1)
	c.mu.RUnlock()

	go func() {
		defer c.wg.Done()
		defer c.pending.Delete(key)
		defer func() {
			if r := recover(); r != nil {
				c.debugf("background refresh panicked for %q: %v", key, r)
			}
		}()

		data, err := loader(c.rootCtx)
		if err != nil {
			c.debugf("background refresh failed for %q: %v", key, err)

			return
		}
		if !c.refreshIfPresent(key, data) {
			c.debugf("background refresh discarded for %q (invalidated)", key)

			return
		}
		c.debugf("background refresh done for %q", key)
	}()
}
