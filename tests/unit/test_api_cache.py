import asyncio
import time
from datetime import datetime, timezone
from pathlib import Path
from unittest.mock import AsyncMock, patch

import pytest

from lazylab.lib.cache import AsyncCache, _CacheEntry, api_cache, cached
from lazylab.lib.constants import MRState, PipelineStatus
from lazylab.models.gitlab import MergeRequest, Pipeline, Project, User

SAMPLE_USER = User(
    id=1, username="dev", name="Dev User", web_url="https://gl.com/dev"
)

SAMPLE_PROJECT = Project(
    id=10,
    name="test",
    path_with_namespace="group/test",
    default_branch="main",
    web_url="https://gl.com/group/test",
    last_activity_at=datetime(2026, 1, 1, tzinfo=timezone.utc),
)

SAMPLE_MR = MergeRequest(
    id=100,
    iid=5,
    title="Fix bug",
    state=MRState.OPENED,
    author=SAMPLE_USER,
    source_branch="fix",
    target_branch="main",
    web_url="https://gl.com/group/test/-/merge_requests/5",
    created_at=datetime(2026, 1, 1, tzinfo=timezone.utc),
    updated_at=datetime(2026, 1, 2, tzinfo=timezone.utc),
    project_path="group/test",
)

SAMPLE_PIPELINE = Pipeline(
    id=200,
    status=PipelineStatus.SUCCESS,
    ref="main",
    sha="abc123",
    web_url="https://gl.com/group/test/-/pipelines/200",
    created_at=datetime(2026, 1, 1, tzinfo=timezone.utc),
    updated_at=datetime(2026, 1, 1, tzinfo=timezone.utc),
)


# ---------------------------------------------------------------------------
# _CacheEntry
# ---------------------------------------------------------------------------


class TestCacheEntry:
    def test_fresh_entry_not_stale(self):
        entry = _CacheEntry(data="hello", created_at=time.time())
        assert not entry.is_stale(ttl=60.0)

    def test_old_entry_is_stale(self):
        entry = _CacheEntry(data="hello", created_at=time.time() - 120)
        assert entry.is_stale(ttl=60.0)

    def test_barely_within_ttl(self):
        entry = _CacheEntry(data="hello", created_at=time.time() - 59)
        assert not entry.is_stale(ttl=60.0)


# ---------------------------------------------------------------------------
# AsyncCache core operations
# ---------------------------------------------------------------------------


class TestAsyncCacheCore:
    def setup_method(self):
        self.cache = AsyncCache()
        self.cache.configure(ttl=60.0, cache_dir=Path("/tmp/nonexistent"))

    def test_put_and_get(self):
        self.cache.put("key1", "value1")
        entry = self.cache.get("key1")
        assert entry is not None
        assert entry.data == "value1"

    def test_get_miss_returns_none(self):
        assert self.cache.get("missing") is None

    def test_invalidate_by_prefix(self):
        self.cache.put("mr_list:1:group/proj:opened", [SAMPLE_MR])
        self.cache.put("mr_list:1:group/proj:closed", [])
        self.cache.put("mr_list:2:other/proj:opened", [SAMPLE_MR])

        self.cache.invalidate("mr_list:1:")
        assert self.cache.get("mr_list:1:group/proj:opened") is None
        assert self.cache.get("mr_list:1:group/proj:closed") is None
        assert self.cache.get("mr_list:2:other/proj:opened") is not None

    def test_invalidate_no_false_positive(self):
        """Prefix 'mr_list:1:' must not match 'mr_list:10:'."""
        self.cache.put("mr_list:1:a", "yes")
        self.cache.put("mr_list:10:a", "no")

        self.cache.invalidate("mr_list:1:")
        assert self.cache.get("mr_list:1:a") is None
        assert self.cache.get("mr_list:10:a") is not None

    def test_invalidate_all(self):
        self.cache.put("a", 1)
        self.cache.put("b", 2)
        self.cache.invalidate_all()
        assert self.cache.get("a") is None
        assert self.cache.get("b") is None

    def test_invalidate_mr(self):
        self.cache.put("mr_list:10:group/test:opened", [SAMPLE_MR])
        self.cache.put("mr:10:5:group/test", SAMPLE_MR)
        self.cache.put("mr_approvals:10:5", "approved")
        self.cache.put("mr_discussions:10:5", "stats")
        self.cache.put("mr_changes:10:5", "diff")
        self.cache.put("pipeline_latest:10:5", SAMPLE_PIPELINE)
        self.cache.put("pipeline_detail:10:5", None)
        # Unrelated entry
        self.cache.put("mr_list:99:other:opened", [])

        self.cache.invalidate_mr(project_id=10, mr_iid=5)

        assert self.cache.get("mr_list:10:group/test:opened") is None
        assert self.cache.get("mr:10:5:group/test") is None
        assert self.cache.get("mr_approvals:10:5") is None
        assert self.cache.get("mr_discussions:10:5") is None
        assert self.cache.get("mr_changes:10:5") is None
        assert self.cache.get("pipeline_latest:10:5") is None
        assert self.cache.get("pipeline_detail:10:5") is None
        # Unrelated entry untouched
        assert self.cache.get("mr_list:99:other:opened") is not None


# ---------------------------------------------------------------------------
# make_key
# ---------------------------------------------------------------------------


class TestMakeKey:
    def test_basic_key(self):
        key = AsyncCache.make_key("ns", {"a": 1, "b": "hello"})
        assert key == "ns:1:hello"

    def test_none_values_excluded(self):
        key = AsyncCache.make_key("ns", {"a": 1, "b": None, "c": "x"})
        assert key == "ns:1:x"

    def test_empty_arguments(self):
        key = AsyncCache.make_key("ns", {})
        assert key == "ns"


# ---------------------------------------------------------------------------
# Disk persistence (serialization round-trip)
# ---------------------------------------------------------------------------


class TestDiskPersistence:
    def test_roundtrip_pydantic_model(self, tmp_path: Path):
        cache = AsyncCache()
        cache.configure(ttl=600, cache_dir=tmp_path)

        cache.put("project:10", SAMPLE_PROJECT)

        # Clear memory, load from disk
        cache._entries.clear()
        entry = cache._load_from_disk("project:10", Project)
        assert entry is not None
        assert isinstance(entry.data, Project)
        assert entry.data.id == SAMPLE_PROJECT.id
        assert entry.data.path_with_namespace == SAMPLE_PROJECT.path_with_namespace

    def test_roundtrip_list_of_models(self, tmp_path: Path):
        cache = AsyncCache()
        cache.configure(ttl=600, cache_dir=tmp_path)

        cache.put("mr_list:10:group/test:opened", [SAMPLE_MR])

        cache._entries.clear()
        entry = cache._load_from_disk("mr_list:10:group/test:opened", MergeRequest)
        assert entry is not None
        assert isinstance(entry.data, list)
        assert len(entry.data) == 1
        assert entry.data[0].iid == SAMPLE_MR.iid

    def test_roundtrip_none(self, tmp_path: Path):
        cache = AsyncCache()
        cache.configure(ttl=600, cache_dir=tmp_path)

        cache.put("pipeline:10:5", None)

        cache._entries.clear()
        entry = cache._load_from_disk("pipeline:10:5", Pipeline)
        assert entry is not None
        assert entry.data is None

    def test_roundtrip_string(self, tmp_path: Path):
        cache = AsyncCache()
        cache.configure(ttl=600, cache_dir=tmp_path)

        cache.put("job_trace:10:42", "build log content")

        cache._entries.clear()
        entry = cache._load_from_disk("job_trace:10:42", None)
        assert entry is not None
        assert entry.data == "build log content"

    def test_corrupted_disk_file_returns_none(self, tmp_path: Path):
        cache = AsyncCache()
        cache.configure(ttl=600, cache_dir=tmp_path)

        path = tmp_path / "api_bad__key.json"
        path.write_text("not valid json{{{")

        assert cache._load_from_disk("bad:key", None) is None

    def test_disk_staleness_check(self, tmp_path: Path):
        cache = AsyncCache()
        cache.configure(ttl=60, cache_dir=tmp_path)

        # Write entry with old timestamp
        old_entry = _CacheEntry(data="old", created_at=time.time() - 120)
        cache._save_to_disk("stale:key", old_entry)

        loaded = cache._load_from_disk("stale:key", None)
        assert loaded is not None
        assert loaded.is_stale(60.0)


# ---------------------------------------------------------------------------
# @cached decorator
# ---------------------------------------------------------------------------


class TestCachedDecorator:
    def setup_method(self):
        # Reset the global cache for each test
        api_cache._entries.clear()
        api_cache._pending_refreshes.clear()
        api_cache._configured = True
        api_cache._ttl = 60.0
        api_cache._cache_dir = None  # disable disk for decorator tests

    @pytest.mark.asyncio
    async def test_cache_miss_calls_function(self):
        mock_fn = AsyncMock(return_value=[SAMPLE_MR])

        @cached("test_ns", model=MergeRequest)
        async def fetch(project_id: int) -> list[MergeRequest]:
            return await mock_fn(project_id)

        result = await fetch(10)
        assert result == [SAMPLE_MR]
        mock_fn.assert_called_once_with(10)

    @pytest.mark.asyncio
    async def test_cache_hit_skips_function(self):
        call_count = 0

        @cached("test_hit", model=MergeRequest)
        async def fetch(project_id: int) -> list[MergeRequest]:
            nonlocal call_count
            call_count += 1
            return [SAMPLE_MR]

        await fetch(10)
        await fetch(10)
        assert call_count == 1

    @pytest.mark.asyncio
    async def test_different_args_different_keys(self):
        call_count = 0

        @cached("test_diff", model=MergeRequest)
        async def fetch(project_id: int) -> list[MergeRequest]:
            nonlocal call_count
            call_count += 1
            return [SAMPLE_MR]

        await fetch(10)
        await fetch(20)
        assert call_count == 2

    @pytest.mark.asyncio
    async def test_stale_returns_cached_and_schedules_refresh(self):
        refresh_called = asyncio.Event()

        @cached("test_stale", model=Project)
        async def fetch(pid: int) -> Project:
            refresh_called.set()
            return SAMPLE_PROJECT

        # Populate cache
        await fetch(1)
        refresh_called.clear()

        # Make entry stale
        key = api_cache.make_key("test_stale", {"pid": 1})
        api_cache._entries[key].created_at = time.time() - 120

        # Should return stale data immediately
        result = await fetch(1)
        assert result.id == SAMPLE_PROJECT.id

        # Wait for background refresh
        await asyncio.wait_for(refresh_called.wait(), timeout=2.0)
        # Entry should be fresh now
        assert not api_cache._entries[key].is_stale(60.0)

    @pytest.mark.asyncio
    async def test_cached_none_result(self):
        call_count = 0

        @cached("test_none", model=Pipeline)
        async def fetch(pid: int, mr_iid: int) -> Pipeline | None:
            nonlocal call_count
            call_count += 1
            return None

        result1 = await fetch(10, 5)
        result2 = await fetch(10, 5)
        assert result1 is None
        assert result2 is None
        assert call_count == 1

    @pytest.mark.asyncio
    async def test_cached_string_result(self):
        @cached("test_str")
        async def fetch(job_id: int) -> str:
            return f"log for {job_id}"

        result = await fetch(42)
        assert result == "log for 42"

        # Second call should be cached
        result2 = await fetch(42)
        assert result2 == "log for 42"

    @pytest.mark.asyncio
    async def test_background_refresh_deduplication(self):
        call_count = 0
        gate = asyncio.Event()

        @cached("test_dedup", model=Project)
        async def fetch(pid: int) -> Project:
            nonlocal call_count
            call_count += 1
            if call_count > 1:
                await gate.wait()
            return SAMPLE_PROJECT

        # Populate and make stale
        await fetch(1)
        key = api_cache.make_key("test_dedup", {"pid": 1})
        api_cache._entries[key].created_at = time.time() - 120

        # Trigger multiple stale reads — should only schedule one refresh
        await fetch(1)
        await fetch(1)
        await asyncio.sleep(0.05)

        # Only one background refresh should be pending/running
        # call_count should be 2 (initial + one refresh), not 3
        gate.set()
        await asyncio.sleep(0.05)
        assert call_count == 2

    @pytest.mark.asyncio
    async def test_background_refresh_failure_keeps_stale_data(self):
        first_call = True

        @cached("test_fail", model=Project)
        async def fetch(pid: int) -> Project:
            nonlocal first_call
            if first_call:
                first_call = False
                return SAMPLE_PROJECT
            raise RuntimeError("API down")

        await fetch(1)
        key = api_cache.make_key("test_fail", {"pid": 1})
        api_cache._entries[key].created_at = time.time() - 120

        result = await fetch(1)
        assert result.id == SAMPLE_PROJECT.id
        await asyncio.sleep(0.1)

        # Stale data should still be there (refresh failed)
        assert api_cache.get(key) is not None
        assert api_cache.get(key).data.id == SAMPLE_PROJECT.id  # type: ignore[union-attr]


# ---------------------------------------------------------------------------
# on_refresh callback
# ---------------------------------------------------------------------------


class TestOnRefreshCallback:
    def setup_method(self):
        api_cache._entries.clear()
        api_cache._pending_refreshes.clear()
        api_cache._configured = True
        api_cache._ttl = 60.0
        api_cache._cache_dir = None
        api_cache._on_refresh = None

    def teardown_method(self):
        api_cache._on_refresh = None

    @pytest.mark.asyncio
    async def test_callback_fires_after_background_refresh(self):
        refreshed: list[tuple[str, str]] = []

        api_cache._on_refresh = lambda ns, key: refreshed.append((ns, key))

        @cached("test_cb", model=Project)
        async def fetch(pid: int) -> Project:
            return SAMPLE_PROJECT

        # Populate and make stale
        await fetch(1)
        key = api_cache.make_key("test_cb", {"pid": 1})
        api_cache._entries[key].created_at = time.time() - 120

        await fetch(1)
        await asyncio.sleep(0.1)

        assert len(refreshed) == 1
        assert refreshed[0] == ("test_cb", key)

    @pytest.mark.asyncio
    async def test_callback_not_fired_on_cache_hit(self):
        refreshed: list[tuple[str, str]] = []

        api_cache._on_refresh = lambda ns, key: refreshed.append((ns, key))

        @cached("test_no_cb", model=Project)
        async def fetch(pid: int) -> Project:
            return SAMPLE_PROJECT

        await fetch(1)
        await fetch(1)  # fresh hit — no refresh
        await asyncio.sleep(0.05)

        assert len(refreshed) == 0

    @pytest.mark.asyncio
    async def test_callback_not_fired_on_refresh_failure(self):
        refreshed: list[tuple[str, str]] = []
        first_call = True

        api_cache._on_refresh = lambda ns, key: refreshed.append((ns, key))

        @cached("test_fail_cb", model=Project)
        async def fetch(pid: int) -> Project:
            nonlocal first_call
            if first_call:
                first_call = False
                return SAMPLE_PROJECT
            raise RuntimeError("fail")

        await fetch(1)
        key = api_cache.make_key("test_fail_cb", {"pid": 1})
        api_cache._entries[key].created_at = time.time() - 120

        await fetch(1)
        await asyncio.sleep(0.1)

        assert len(refreshed) == 0


# ---------------------------------------------------------------------------
# Lazy configuration
# ---------------------------------------------------------------------------


class TestLazyConfiguration:
    def setup_method(self):
        api_cache._entries.clear()
        api_cache._configured = False

    def teardown_method(self):
        api_cache._configured = True
        api_cache._ttl = 60.0
        api_cache._cache_dir = None

    @pytest.mark.asyncio
    async def test_ensure_configured_reads_from_context(self, tmp_path: Path):
        from lazylab.lib.config import CacheSettings, Config

        mock_cache_settings = CacheSettings(directory=tmp_path, ttl=300)

        with patch("lazylab.lib.context._LazyLabContext.config", new_callable=lambda: property(
            lambda self: Config(cache=mock_cache_settings)
        )):
            @cached("test_lazy")
            async def fetch() -> str:
                return "ok"

            result = await fetch()
            assert result == "ok"
            assert api_cache._ttl == 300.0
            assert api_cache._cache_dir == tmp_path
