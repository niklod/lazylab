package cache_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/cache"
	"github.com/niklod/lazylab/internal/config"
)

type RefreshEventSuite struct {
	suite.Suite
	fs    afero.Fs
	clock *fakeClock
	c     *cache.Cache

	mu     sync.Mutex
	events []refreshEvent
}

type refreshEvent struct {
	namespace string
	key       string
}

func (s *RefreshEventSuite) SetupTest() {
	s.fs = afero.NewMemMapFs()
	s.clock = newFakeClock(time.Unix(1_700_000_000, 0))
	cfg := config.CacheConfig{Directory: "/cache", TTL: 60}
	s.c = cache.New(cfg, s.fs, cache.WithClock(s.clock.Now))
	s.events = nil
	s.c.SetOnRefresh(func(_ context.Context, namespace, key string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.events = append(s.events, refreshEvent{namespace, key})
	})
}

func (s *RefreshEventSuite) TearDownTest() {
	_ = s.c.Shutdown(context.Background())
}

func (s *RefreshEventSuite) recordedEvents() []refreshEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]refreshEvent, len(s.events))
	copy(out, s.events)

	return out
}

func (s *RefreshEventSuite) TestOnRefresh_FiresAfterBackgroundRefresh() {
	var calls atomic.Int32
	loader := func(_ context.Context) (string, error) {
		n := calls.Add(1)
		if n == 1 {
			return "v1", nil
		}

		return "v2", nil
	}

	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.clock.Advance(2 * time.Minute)
	_, err = cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.Require().NoError(s.c.Shutdown(context.Background()))

	ev := s.recordedEvents()
	s.Require().Len(ev, 1)
	s.Require().Equal("ns", ev[0].namespace)
	s.Require().Equal("ns:1", ev[0].key)
}

func (s *RefreshEventSuite) TestOnRefresh_DoesNotFireOnMiss() {
	loader, _ := countingLoader("v1", nil)

	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.Require().NoError(s.c.Shutdown(context.Background()))
	s.Require().Empty(s.recordedEvents(), "miss must not fire OnRefresh")
}

func (s *RefreshEventSuite) TestOnRefresh_DoesNotFireWhenInvalidatedMidFlight() {
	var calls atomic.Int32
	refreshGate := make(chan struct{})
	refreshReleased := make(chan struct{})
	loader := func(_ context.Context) (string, error) {
		n := calls.Add(1)
		if n == 1 {
			return "seed-v1", nil
		}
		<-refreshGate
		close(refreshReleased)

		return "bg-refresh", nil
	}

	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.clock.Advance(2 * time.Minute)
	_, err = cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.c.Invalidate("ns:")

	close(refreshGate)
	<-refreshReleased
	s.Require().NoError(s.c.Shutdown(context.Background()))

	s.Require().Empty(s.recordedEvents(), "invalidated-mid-flight refresh must not fire OnRefresh")
}

func (s *RefreshEventSuite) TestOnRefresh_DoesNotFireWhenPayloadUnchanged() {
	loader, _ := countingLoader("identical", nil)

	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.clock.Advance(2 * time.Minute)
	_, err = cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.Require().NoError(s.c.Shutdown(context.Background()))
	s.Require().Empty(s.recordedEvents(), "byte-equal refresh must not fire OnRefresh")
}

func (s *RefreshEventSuite) TestOnRefresh_DoesNotFireOnLoaderError() {
	var calls atomic.Int32
	loader := func(_ context.Context) (string, error) {
		n := calls.Add(1)
		if n == 1 {
			return "v1", nil
		}

		return "", errors.New("transient fail")
	}

	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.clock.Advance(2 * time.Minute)
	_, err = cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.Require().NoError(s.c.Shutdown(context.Background()))
	s.Require().Empty(s.recordedEvents(), "failed refresh must not fire OnRefresh")
}

func (s *RefreshEventSuite) TestOnRefresh_DoesNotFireOnLoaderPanic() {
	var calls atomic.Int32
	loader := func(_ context.Context) (string, error) {
		n := calls.Add(1)
		if n == 1 {
			return "v1", nil
		}
		panic("boom")
	}

	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.clock.Advance(2 * time.Minute)
	_, err = cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.Require().NoError(s.c.Shutdown(context.Background()))
	s.Require().Empty(s.recordedEvents(), "panicking refresh must not fire OnRefresh")
}

func (s *RefreshEventSuite) TestOnRefresh_NilCallbackIsSafe() {
	s.c.SetOnRefresh(nil)

	var calls atomic.Int32
	loader := func(_ context.Context) (string, error) {
		n := calls.Add(1)
		if n == 1 {
			return "v1", nil
		}

		return "v2", nil
	}

	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.clock.Advance(2 * time.Minute)
	_, err = cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.Require().NoError(s.c.Shutdown(context.Background()))
}

func (s *RefreshEventSuite) TestOnRefresh_NamespaceForwardedAsEmitted() {
	var calls atomic.Int32
	loader := func(_ context.Context) ([]int, error) {
		n := calls.Add(1)
		if n == 1 {
			return []int{1, 2}, nil
		}

		return []int{1, 2, 3}, nil
	}

	_, err := cache.Do(context.Background(), s.c, "mr_list", loader, 42, "opened")
	s.Require().NoError(err)

	s.clock.Advance(2 * time.Minute)
	_, err = cache.Do(context.Background(), s.c, "mr_list", loader, 42, "opened")
	s.Require().NoError(err)

	s.Require().NoError(s.c.Shutdown(context.Background()))

	ev := s.recordedEvents()
	s.Require().Len(ev, 1)
	s.Require().Equal("mr_list", ev[0].namespace)
	s.Require().Equal("mr_list:42:opened", ev[0].key)
}

func (s *RefreshEventSuite) TestOnRefresh_ConcurrentRefreshesFireOncePerKey() {
	var counters sync.Map
	loader := func(ns, arg string) func(context.Context) (string, error) {
		return func(_ context.Context) (string, error) {
			v, _ := counters.LoadOrStore(ns+":"+arg, new(atomic.Int32))
			n := v.(*atomic.Int32).Add(1)
			if n == 1 {
				return "seed-" + arg, nil
			}

			return "refreshed-" + arg, nil
		}
	}

	keys := []string{"a", "b", "c", "d"}
	for _, k := range keys {
		_, err := cache.Do(context.Background(), s.c, "ns", loader("ns", k), k)
		s.Require().NoError(err)
	}

	s.clock.Advance(2 * time.Minute)

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		k := keys[i%len(keys)]
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = cache.Do(context.Background(), s.c, "ns", loader("ns", k), k)
		}()
	}
	wg.Wait()

	s.Require().NoError(s.c.Shutdown(context.Background()))

	seen := map[string]int{}
	for _, ev := range s.recordedEvents() {
		seen[ev.key]++
	}
	for _, k := range keys {
		s.Require().Equal(1, seen["ns:"+k], "expected exactly one refresh event for %q", k)
	}
}

func TestRefreshEventSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(RefreshEventSuite))
}
