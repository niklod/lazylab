package cache

import (
	"fmt"
	"strings"
)

// MakeKey builds a deterministic cache key from a namespace and positional
// arguments. Nil args are skipped (mirroring Python's None-skipping so
// optional filters do not split the cache). All other values are rendered
// with %v.
func MakeKey(namespace string, args ...any) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, namespace)
	for _, a := range args {
		if a == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%v", a))
	}

	return strings.Join(parts, ":")
}
