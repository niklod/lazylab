package cache

import "time"

type entry struct {
	data      any
	createdAt time.Time
}

func (e *entry) isStale(now time.Time, ttl time.Duration) bool {
	return now.Sub(e.createdAt) > ttl
}
