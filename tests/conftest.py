import pytest


@pytest.fixture(autouse=True)
def reset_config():
    """Reset config singleton between tests."""
    from lazylab.lib.config import Config

    Config.reset()
    yield
    Config.reset()
