package cache

import "context"

// Do is the stale-while-revalidate read path. Flow:
//  1. In-memory hit  → return data; if stale, schedule a background refresh.
//  2. Disk hit       → promote to memory + return; if stale, schedule refresh.
//  3. Miss           → call loader synchronously, store, return.
//
// The generic parameter T obviates the Python decorator's explicit `model=`
// argument — json.Unmarshal reconstructs T directly on disk hit.
//
// Callers must use a stable T for a given namespace. Mixing types under the
// same namespace causes a type assertion to fail, which invalidates the entry
// and forces a fresh loader call.
func Do[T any](
	ctx context.Context,
	c *Cache,
	namespace string,
	loader func(context.Context) (T, error),
	args ...any,
) (T, error) {
	var zero T
	key := MakeKey(namespace, args...)

	if e, ok := c.getEntry(key); ok {
		if data, typeOK := e.data.(T); typeOK {
			if e.isStale(c.now(), c.ttl) {
				c.debugf("cache stale %q, scheduling refresh", key)
				c.scheduleRefresh(namespace, key, wrapLoader(loader))
			} else {
				c.debugf("cache hit %q", key)
			}

			return data, nil
		}
		c.debugf("cache type mismatch %q, reloading", key)
		c.Invalidate(key)
	}

	if data, createdAt, ok := loadDisk[T](c.fs, c.dir, key); ok {
		e := &entry{data: data, createdAt: createdAt}
		c.putEntry(key, e)
		if e.isStale(c.now(), c.ttl) {
			c.debugf("disk cache stale %q, scheduling refresh", key)
			c.scheduleRefresh(namespace, key, wrapLoader(loader))
		} else {
			c.debugf("disk cache hit %q", key)
		}

		return data, nil
	}

	c.debugf("cache miss %q", key)
	data, err := loader(ctx)
	if err != nil {
		return zero, err
	}
	c.put(key, data)

	return data, nil
}

// wrapLoader adapts a typed loader into the untyped signature expected by
// scheduleRefresh. Only allocated on the stale path, not on cache hits.
func wrapLoader[T any](loader func(context.Context) (T, error)) func(context.Context) (any, error) {
	return func(ctx context.Context) (any, error) {
		return loader(ctx)
	}
}
