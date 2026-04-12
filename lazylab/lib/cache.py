import asyncio
import inspect
import json
import os
import time
from dataclasses import dataclass
from functools import wraps
from pathlib import Path
from typing import Any, Awaitable, Callable, Iterable, ParamSpec, TypeVar

from pydantic import BaseModel, ValidationError

from lazylab.lib.logging import ll

T = TypeVar("T", bound=BaseModel)
P = ParamSpec("P")
R = TypeVar("R")


# ---------------------------------------------------------------------------
# Legacy disk cache (used by SearchableDataTable for startup table data)
# ---------------------------------------------------------------------------


def _cache_path(cache_dir: Path, project_path: str | None, cache_name: str) -> Path:
    if project_path:
        filename = f"{project_path.replace('/', '_')}_{cache_name}.json"
    else:
        filename = f"{cache_name}.json"
    return cache_dir / filename


def load_models_from_cache(
    cache_dir: Path, project_path: str | None, cache_name: str, expect_type: type[T]
) -> list[T]:
    path = _cache_path(cache_dir, project_path, cache_name)
    if not path.is_file():
        return []

    try:
        cached_objects = json.loads(path.read_text())
        return [expect_type(**raw_obj) for raw_obj in cached_objects]
    except json.JSONDecodeError as e:
        ll.warning("Failed to parse cache file '%s' as JSON: %s", path, e)
    except ValidationError as e:
        ll.warning("Cache schema mismatch in '%s' for %s: %s", path, expect_type.__name__, e)
    except Exception as e:
        ll.warning("Unexpected error loading cache from '%s': %s", path, e)

    return []


def save_models_to_cache(
    cache_dir: Path, project_path: str | None, cache_name: str, objects: Iterable[T]
) -> None:
    path = _cache_path(cache_dir, project_path, cache_name)
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    path.write_text(json.dumps([o.model_dump(mode="json") for o in objects]))
    os.chmod(path, 0o600)


# ---------------------------------------------------------------------------
# AsyncCache — in-memory + disk, stale-while-revalidate
# ---------------------------------------------------------------------------


@dataclass
class _CacheEntry:
    """A single cached value with its wall-clock creation timestamp."""

    data: Any
    created_at: float  # time.time()

    def is_stale(self, ttl: float) -> bool:
        return (time.time() - self.created_at) > ttl


class AsyncCache:
    """In-memory + disk cache with stale-while-revalidate semantics.

    Cached data is always served immediately.  When an entry is stale a
    background ``asyncio.Task`` transparently refreshes it so that the
    *next* caller gets fresh data.
    """

    def __init__(self) -> None:
        self._entries: dict[str, _CacheEntry] = {}
        self._pending_refreshes: set[str] = set()
        self._configured = False
        self._ttl: float = 600.0
        self._cache_dir: Path | None = None
        self._on_refresh: Callable[[str, str], None] | None = None

    # -- configuration ------------------------------------------------------

    def configure(self, ttl: float, cache_dir: Path) -> None:
        self._ttl = ttl
        self._cache_dir = cache_dir
        self._configured = True

    def _ensure_configured(self) -> None:
        if self._configured:
            return
        from lazylab.lib.context import LazyLabContext

        config = LazyLabContext.config
        self.configure(ttl=float(config.cache.ttl), cache_dir=config.cache.directory)

    # -- key helpers --------------------------------------------------------

    @staticmethod
    def make_key(namespace: str, arguments: dict[str, Any]) -> str:
        parts = [namespace]
        for v in arguments.values():
            if v is not None:
                parts.append(str(v))
        return ":".join(parts)

    # -- disk persistence ---------------------------------------------------

    def _disk_path(self, key: str) -> Path | None:
        if self._cache_dir is None:
            return None
        safe = key.replace(":", "__").replace("/", "_").replace("=", "_")
        return self._cache_dir / f"api_{safe}.json"

    @staticmethod
    def _serialize_value(data: Any) -> Any:
        if isinstance(data, BaseModel):
            return data.model_dump(mode="json")
        if isinstance(data, list):
            return [AsyncCache._serialize_value(item) for item in data]
        return data

    @staticmethod
    def _deserialize_value(raw: Any, model: type[BaseModel] | None) -> Any:
        if model is None or raw is None:
            return raw
        if isinstance(raw, list):
            return [model.model_validate(item) for item in raw]
        if isinstance(raw, dict):
            return model.model_validate(raw)
        return raw

    def _load_from_disk(self, key: str, model: type[BaseModel] | None) -> _CacheEntry | None:
        path = self._disk_path(key)
        if path is None or not path.is_file():
            return None
        try:
            raw = json.loads(path.read_text())
            data = self._deserialize_value(raw["data"], model)
            return _CacheEntry(data=data, created_at=raw["created_at"])
        except Exception as exc:
            ll.debug("Disk cache load failed for '%s': %s", key, exc)
            return None

    def _save_to_disk(self, key: str, entry: _CacheEntry) -> None:
        path = self._disk_path(key)
        if path is None:
            return
        try:
            path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
            payload = json.dumps(
                {"created_at": entry.created_at, "data": self._serialize_value(entry.data)}
            )
            path.write_text(payload)
            os.chmod(path, 0o600)
        except Exception as exc:
            ll.debug("Disk cache save failed for '%s': %s", key, exc)

    # -- core operations ----------------------------------------------------

    def get(self, key: str) -> _CacheEntry | None:
        return self._entries.get(key)

    def put(self, key: str, data: Any) -> None:
        entry = _CacheEntry(data=data, created_at=time.time())
        self._entries[key] = entry
        self._save_to_disk(key, entry)

    def invalidate(self, prefix: str) -> None:
        keys = [k for k in self._entries if k.startswith(prefix)]
        for k in keys:
            del self._entries[k]
        if keys:
            ll.debug("Invalidated %d cache entries with prefix '%s'", len(keys), prefix)

    def invalidate_mr(self, project_id: int, mr_iid: int) -> None:
        """Invalidate all caches related to a specific merge request."""
        self.invalidate(f"mr_list:{project_id}:")
        self.invalidate(f"mr:{project_id}:{mr_iid}")
        self.invalidate(f"mr_approvals:{project_id}:{mr_iid}")
        self.invalidate(f"mr_changes:{project_id}:{mr_iid}")
        self.invalidate(f"pipeline_latest:{project_id}:{mr_iid}")
        self.invalidate(f"pipeline_detail:{project_id}:{mr_iid}")

    def invalidate_all(self) -> None:
        count = len(self._entries)
        self._entries.clear()
        if count:
            ll.debug("Invalidated all %d cache entries", count)

    # -- background refresh -------------------------------------------------

    async def _background_refresh(
        self,
        namespace: str,
        key: str,
        fn: Callable[..., Awaitable[Any]],
        kwargs: dict[str, Any],
    ) -> None:
        if key in self._pending_refreshes:
            return
        self._pending_refreshes.add(key)
        try:
            result = await fn(**kwargs)
            self.put(key, result)
            ll.debug("Background refresh done for '%s'", key)
            if self._on_refresh is not None:
                self._on_refresh(namespace, key)
        except Exception as exc:
            ll.debug("Background refresh failed for '%s': %s", key, exc)
        finally:
            self._pending_refreshes.discard(key)


# Singleton used by the @cached decorator and mutation invalidation.
api_cache = AsyncCache()


# ---------------------------------------------------------------------------
# @cached decorator
# ---------------------------------------------------------------------------


def cached(namespace: str, *, model: type[BaseModel] | None = None) -> Callable:
    """Decorator adding stale-while-revalidate caching to an async function.

    Args:
        namespace: Cache key prefix (e.g. ``"mr_list"``).
        model: Pydantic model class for disk serialisation. Pass ``None``
               for plain types like ``str``.
    """

    def decorator(fn: Callable[P, Awaitable[R]]) -> Callable[P, Awaitable[R]]:
        original_fn = fn
        sig = inspect.signature(original_fn)

        @wraps(fn)
        async def wrapper(*args: P.args, **kwargs: P.kwargs) -> R:
            api_cache._ensure_configured()

            bound = sig.bind(*args, **kwargs)
            bound.apply_defaults()
            arguments = dict(bound.arguments)
            key = api_cache.make_key(namespace, arguments)

            # 1. In-memory hit
            entry = api_cache.get(key)
            if entry is not None:
                if entry.is_stale(api_cache._ttl):
                    ll.debug("Cache stale '%s', scheduling refresh", key)
                    asyncio.create_task(
                        api_cache._background_refresh(
                            namespace, key, original_fn, arguments
                        )
                    )
                else:
                    ll.debug("Cache hit '%s'", key)
                return entry.data  # type: ignore[return-value]

            # 2. Disk hit
            disk_entry = api_cache._load_from_disk(key, model)
            if disk_entry is not None:
                api_cache._entries[key] = disk_entry
                if disk_entry.is_stale(api_cache._ttl):
                    ll.debug("Disk cache stale '%s', scheduling refresh", key)
                    asyncio.create_task(
                        api_cache._background_refresh(
                            namespace, key, original_fn, arguments
                        )
                    )
                else:
                    ll.debug("Disk cache hit '%s'", key)
                return disk_entry.data  # type: ignore[return-value]

            # 3. Cache miss — fetch from API
            ll.debug("Cache miss '%s'", key)
            result = await original_fn(*args, **kwargs)
            api_cache.put(key, result)
            return result

        wrapper.__wrapped__ = original_fn  # type: ignore[attr-defined]
        return wrapper  # type: ignore[return-value]

    return decorator
