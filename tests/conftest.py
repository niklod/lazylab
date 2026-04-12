import pytest


@pytest.fixture(autouse=True)
def reset_config():
    """Reset config singleton and context between tests."""
    from lazylab.lib.config import Config
    from lazylab.lib.context import _LazyLabContext

    Config.reset()
    _LazyLabContext._config = None
    _LazyLabContext._client = None
    yield
    Config.reset()
    _LazyLabContext._config = None
    _LazyLabContext._client = None


@pytest.fixture(autouse=True)
def reset_api_cache():
    """Reset the global API cache between tests."""
    from lazylab.lib.cache import api_cache

    api_cache._entries.clear()
    api_cache._pending_refreshes.clear()
    api_cache._configured = False
    api_cache._cache_dir = None
    yield
    api_cache._entries.clear()
    api_cache._pending_refreshes.clear()
    api_cache._configured = False
    api_cache._cache_dir = None
