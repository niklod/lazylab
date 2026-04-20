package cache_test

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/cache"
	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/models"
)

type CacheSuite struct {
	suite.Suite
	fs    afero.Fs
	clock *fakeClock
	c     *cache.Cache
}

func (s *CacheSuite) SetupTest() {
	s.fs = afero.NewMemMapFs()
	s.clock = newFakeClock(time.Unix(1_700_000_000, 0))
	cfg := config.CacheConfig{Directory: "/cache", TTL: 60}
	s.c = cache.New(cfg, s.fs, cache.WithClock(s.clock.Now))
}

func (s *CacheSuite) TearDownTest() {
	_ = s.c.Shutdown(context.Background())
}

func (s *CacheSuite) TestDo_Miss_CallsLoaderAndStores() {
	loader, calls := countingLoader("v1", nil)

	got, err := cache.Do(context.Background(), s.c, "ns", loader, 1)

	s.Require().NoError(err)
	s.Require().Equal("v1", got)
	s.Require().Equal(int32(1), calls.Load())
}

func (s *CacheSuite) TestDo_MemoryHitFresh_SkipsLoader() {
	loader, calls := countingLoader("v1", nil)
	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	got, err := cache.Do(context.Background(), s.c, "ns", loader, 1)

	s.Require().NoError(err)
	s.Require().Equal("v1", got)
	s.Require().Equal(int32(1), calls.Load())
}

func (s *CacheSuite) TestDo_MemoryHitStale_ReturnsStaleAndRefreshes() {
	var mu sync.Mutex
	values := []string{"v1", "v2"}
	idx := 0
	loader := func(ctx context.Context) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		v := values[idx]
		if idx < len(values)-1 {
			idx++
		}

		return v, nil
	}

	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.clock.Advance(2 * time.Minute)

	got, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)
	s.Require().Equal("v1", got)

	s.Require().NoError(s.c.Shutdown(context.Background()))

	fresh := cache.New(
		config.CacheConfig{Directory: "/cache", TTL: 60},
		s.fs,
		cache.WithClock(s.clock.Now),
	)
	defer func() { _ = fresh.Shutdown(context.Background()) }()

	final, err := cache.Do(context.Background(), fresh, "ns", loader, 1)
	s.Require().NoError(err)
	s.Require().Equal("v2", final)
}

func (s *CacheSuite) TestDo_TypeMismatch_InvalidatesAndReloads() {
	stringLoader, sCalls := countingLoader("seeded", nil)
	_, err := cache.Do(context.Background(), s.c, "ns", stringLoader, 1)
	s.Require().NoError(err)
	s.Require().Equal(int32(1), sCalls.Load())

	intLoader, iCalls := countingLoader(42, nil)

	got, err := cache.Do(context.Background(), s.c, "ns", intLoader, 1)

	s.Require().NoError(err)
	s.Require().Equal(42, got)
	s.Require().Equal(int32(1), iCalls.Load())
}

func (s *CacheSuite) TestDo_LoaderError_PropagatesOnMiss() {
	wantErr := errors.New("boom")
	loader := func(ctx context.Context) (string, error) { return "", wantErr }

	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)

	s.Require().ErrorIs(err, wantErr)
}

func (s *CacheSuite) TestDo_ConcurrentStaleCalls_BackgroundRefreshOnce() {
	var calls atomic.Int32
	block := make(chan struct{})
	loader := func(_ context.Context) (string, error) { //nolint:unparam // signature required by cache.Do
		n := calls.Add(1)
		if n > 1 {
			<-block
		}

		return "v1", nil
	}

	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)
	s.Require().Equal(int32(1), calls.Load())

	s.clock.Advance(2 * time.Minute)

	const N = 20
	var wg sync.WaitGroup
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		s.Require().NoError(err)
	}

	close(block)
	s.Require().NoError(s.c.Shutdown(context.Background()))

	s.Require().Equal(int32(2), calls.Load())
}

func (s *CacheSuite) TestDo_DiskHitFresh_SkipsLoader() {
	loader, calls := countingLoader("v1", nil)
	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.Require().NoError(s.c.Shutdown(context.Background()))

	fresh := cache.New(
		config.CacheConfig{Directory: "/cache", TTL: 60},
		s.fs,
		cache.WithClock(s.clock.Now),
	)
	s.c = fresh

	got, err := cache.Do(context.Background(), fresh, "ns", loader, 1)

	s.Require().NoError(err)
	s.Require().Equal("v1", got)
	s.Require().Equal(int32(1), calls.Load())
}

func (s *CacheSuite) TestInvalidate_ClearsMemoryAndDisk() {
	loader, _ := countingLoader("v1", nil)
	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	path := "/cache/api_ns_1.json"
	exists, statErr := afero.Exists(s.fs, path)
	s.Require().NoError(statErr)
	s.Require().True(exists)

	s.c.Invalidate("ns:")

	exists, statErr = afero.Exists(s.fs, path)
	s.Require().NoError(statErr)
	s.Require().False(exists)

	loader2, calls := countingLoader("v2", nil)
	got, err := cache.Do(context.Background(), s.c, "ns", loader2, 1)
	s.Require().NoError(err)
	s.Require().Equal("v2", got)
	s.Require().Equal(int32(1), calls.Load())
}

func (s *CacheSuite) TestInvalidate_DiscardsInFlightRefresh() {
	var calls atomic.Int32
	refreshGate := make(chan struct{})
	refreshReleased := make(chan struct{})
	loader := func(ctx context.Context) (string, error) {
		n := calls.Add(1)
		if n == 1 {
			return "seed", nil
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

	path := "/cache/api_ns_1.json"
	exists, statErr := afero.Exists(s.fs, path)
	s.Require().NoError(statErr)
	s.Require().False(exists, "invalidated disk file was resurrected by background refresh")
}

func (s *CacheSuite) TestInvalidateMR_ClearsAllMRNamespaces() {
	loader := func(ctx context.Context) (string, error) { return "x", nil }

	namespaces := []string{
		"mr_list", "mr", "mr_approvals", "mr_discussions", "mr_conversation",
		"mr_changes", "pipeline_latest", "pipeline_detail",
	}
	for _, ns := range namespaces {
		_, err := cache.Do(context.Background(), s.c, ns, loader, 10, 99)
		s.Require().NoError(err)
	}

	s.c.InvalidateMR(10, 99)

	for _, ns := range namespaces {
		calls := 0
		countingFn := func(_ context.Context) (string, error) {
			calls++

			return "reloaded", nil
		}
		_, err := cache.Do(context.Background(), s.c, ns, countingFn, 10, 99)
		s.Require().NoError(err)
		s.Require().Equal(1, calls, "namespace %s should have been invalidated", ns)
	}
}

func (s *CacheSuite) TestInvalidateAll_ClearsEverything() {
	loader := func(ctx context.Context) (string, error) { return "x", nil }
	for i := 0; i < 5; i++ {
		_, err := cache.Do(context.Background(), s.c, "ns", loader, i)
		s.Require().NoError(err)
	}

	s.c.InvalidateAll()

	for i := 0; i < 5; i++ {
		path := "/cache/api_ns_" + strconv.Itoa(i) + ".json"
		exists, statErr := afero.Exists(s.fs, path)
		s.Require().NoError(statErr)
		s.Require().False(exists, "file %s should be gone", path)
	}
}

func (s *CacheSuite) TestShutdown_WaitsForInFlightRefresh() {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	loader := func(ctx context.Context) (string, error) {
		n := calls.Add(1)
		if n == 2 {
			close(started)
			<-release
		}

		return "v", nil
	}

	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.clock.Advance(2 * time.Minute)
	_, err = cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	<-started

	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- s.c.Shutdown(context.Background())
	}()

	select {
	case <-shutdownDone:
		s.T().Fatal("Shutdown returned before refresh drained")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-shutdownDone:
		s.Require().NoError(err)
	case <-time.After(time.Second):
		s.T().Fatal("Shutdown did not complete after release")
	}
}

func (s *CacheSuite) TestShutdown_CtxCancelled_ReturnsCtxErr() {
	var calls atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{})
	loader := func(ctx context.Context) (string, error) {
		n := calls.Add(1)
		if n == 1 {
			return "seed", nil
		}
		close(started)
		<-release

		return "bg", nil
	}

	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.clock.Advance(2 * time.Minute)
	_, err = cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err = s.c.Shutdown(ctx)
	s.Require().ErrorIs(err, context.DeadlineExceeded)

	close(release)
	s.Require().NoError(s.c.Shutdown(context.Background()))
}

func (s *CacheSuite) TestShutdown_CalledTwice_Idempotent() {
	loader, _ := countingLoader("v", nil)
	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.Require().NoError(s.c.Shutdown(context.Background()))
	s.Require().NoError(s.c.Shutdown(context.Background()))
}

func (s *CacheSuite) TestScheduleRefresh_AfterShutdown_IsNoop() {
	var calls atomic.Int32
	loader := func(_ context.Context) (string, error) {
		calls.Add(1)

		return "v", nil
	}

	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)
	s.Require().Equal(int32(1), calls.Load())

	s.Require().NoError(s.c.Shutdown(context.Background()))

	s.clock.Advance(2 * time.Minute)
	got, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)
	s.Require().Equal("v", got)

	s.Require().NoError(s.c.Shutdown(context.Background()))
	s.Require().Equal(int32(1), calls.Load(),
		"scheduleRefresh after Shutdown must not start a new loader call")
}

func (s *CacheSuite) TestDo_BackgroundRefreshFailure_PreservesStaleEntry() {
	var calls atomic.Int32
	loader := func(_ context.Context) (string, error) {
		n := calls.Add(1)
		if n == 1 {
			return "good", nil
		}

		return "", errors.New("transient fail")
	}

	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.clock.Advance(2 * time.Minute)

	got, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)
	s.Require().Equal("good", got)

	s.Require().NoError(s.c.Shutdown(context.Background()))

	fresh := cache.New(
		config.CacheConfig{Directory: "/cache", TTL: 60},
		s.fs,
		cache.WithClock(s.clock.Now),
	)
	defer func() { _ = fresh.Shutdown(context.Background()) }()

	after, err := cache.Do(context.Background(), fresh, "ns", loader, 1)
	s.Require().NoError(err)
	s.Require().Equal("good", after)
}

func (s *CacheSuite) TestDo_BackgroundRefreshPanic_Recovered() {
	var calls atomic.Int32
	loader := func(ctx context.Context) (string, error) {
		n := calls.Add(1)
		if n == 1 {
			return "good", nil
		}
		panic("boom")
	}

	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.clock.Advance(2 * time.Minute)
	_, err = cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	s.Require().NoError(s.c.Shutdown(context.Background()))
}

func (s *CacheSuite) TestDo_WithDomainModel_SurvivesRestart() {
	want := models.Project{ID: 42, Name: "demo", PathWithNamespace: "group/demo"}
	loader, calls := countingLoader(want, nil)
	_, err := cache.Do(context.Background(), s.c, "project", loader, 42)
	s.Require().NoError(err)

	s.Require().NoError(s.c.Shutdown(context.Background()))

	fresh := cache.New(
		config.CacheConfig{Directory: "/cache", TTL: 60},
		s.fs,
		cache.WithClock(s.clock.Now),
	)
	s.c = fresh

	got, err := cache.Do(context.Background(), fresh, "project", loader, 42)
	s.Require().NoError(err)
	s.Require().Equal(want, got)
	s.Require().Equal(int32(1), calls.Load())
}

func (s *CacheSuite) TestWithLogger_CapturesDebugEvents() {
	var mu sync.Mutex
	var events []string
	cfg := config.CacheConfig{Directory: "/cache", TTL: 60}
	_ = s.c.Shutdown(context.Background())
	s.c = cache.New(cfg, s.fs,
		cache.WithClock(s.clock.Now),
		cache.WithLogger(func(format string, args ...any) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, format)
		}),
	)

	loader, _ := countingLoader("v", nil)
	_, err := cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)
	_, err = cache.Do(context.Background(), s.c, "ns", loader, 1)
	s.Require().NoError(err)

	mu.Lock()
	defer mu.Unlock()
	s.Require().Contains(events, "cache miss %q")
	s.Require().Contains(events, "cache hit %q")
}

func TestCacheSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(CacheSuite))
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(t time.Time) *fakeClock {
	return &fakeClock{now: t}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

func countingLoader[T any](value T, err error) (func(context.Context) (T, error), *atomic.Int32) {
	var calls atomic.Int32
	fn := func(_ context.Context) (T, error) {
		calls.Add(1)

		return value, err
	}

	return fn, &calls
}
