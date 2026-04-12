"""E2E tests for the API caching layer.

Verifies that cached data is served on repeat access and that
mutations invalidate the relevant cache entries.
"""

from unittest.mock import AsyncMock, patch

import pytest

from lazylab.lib.cache import api_cache
from lazylab.lib.constants import MRState
from lazylab.models.gitlab import MergeRequest, Project, User
from lazylab.ui.app import LazyLab

MOCK_USER = User(
    id=1,
    username="testuser",
    name="Test User",
    web_url="https://gitlab.com/testuser",
)

MOCK_PROJECTS = [
    Project(
        id=1,
        name="project-a",
        path_with_namespace="group/project-a",
        default_branch="main",
        web_url="https://gitlab.com/group/project-a",
        last_activity_at="2026-04-10T10:00:00Z",  # type: ignore[arg-type]
    ),
]

MOCK_MR = MergeRequest(
    id=100,
    iid=1,
    title="Test MR",
    state=MRState.OPENED,
    author=MOCK_USER,
    source_branch="feature",
    target_branch="main",
    web_url="https://gitlab.com/group/project-a/-/merge_requests/1",
    created_at="2026-04-10T10:00:00Z",  # type: ignore[arg-type]
    updated_at="2026-04-10T10:00:00Z",  # type: ignore[arg-type]
    project_path="group/project-a",
)


def _mock_config(tmp_path):
    import lazylab.lib.config as config_mod

    config_mod._CONFIG_INSTANCE = None
    config_mod.CONFIG_FILE = tmp_path / "config.yaml"
    config_mod.CONFIG_FOLDER = tmp_path

    from lazylab.lib.config import Config

    cfg = Config(
        gitlab={"url": "https://gitlab.com", "token": "fake-token"},  # type: ignore[arg-type]
        cache={"directory": str(tmp_path / ".cache"), "ttl": 600},
    )
    cfg.save()
    Config.reset()


@pytest.fixture()
def mock_gitlab(tmp_path):
    _mock_config(tmp_path)

    mock_client = AsyncMock()
    mock_client.get_current_user = AsyncMock(return_value=MOCK_USER)
    mock_client.list_projects = AsyncMock(return_value=MOCK_PROJECTS)

    with patch(
        "lazylab.lib.context._LazyLabContext.client",
        new_callable=lambda: property(lambda self: mock_client),
    ):
        yield mock_client


@pytest.mark.asyncio
async def test_project_list_cached_on_second_load(mock_gitlab, tmp_path):
    """After loading projects once, a second call should not hit the API."""
    _mock_config(tmp_path)

    # Pre-configure api_cache to use tmp_path (avoid disk leaks from other tests)
    api_cache.configure(ttl=600.0, cache_dir=tmp_path / ".api_cache")

    app = LazyLab()
    async with app.run_test(size=(120, 40)) as pilot:
        await pilot.pause()
        await app.workers.wait_for_complete()
        await pilot.pause()

        # list_projects was called once during app load
        initial_call_count = mock_gitlab.list_projects.call_count
        assert initial_call_count >= 1

        # Directly call the cached API function again
        import lazylab.lib.gitlab.projects as projects_api

        result = await projects_api.list_projects()
        assert len(result) >= 1

        # Client should NOT have been called again (served from cache)
        assert mock_gitlab.list_projects.call_count == initial_call_count


@pytest.mark.asyncio
async def test_mr_cache_invalidated_after_close(tmp_path):
    """Closing an MR should invalidate MR-related cache entries."""
    api_cache.configure(ttl=600.0, cache_dir=tmp_path / ".cache")

    key = api_cache.make_key(
        "mr_list",
        {
            "project_id": 1,
            "project_path": "group/project-a",
            "state": "opened",
            "author_id": None,
            "reviewer_id": None,
        },
    )
    api_cache.put(key, [MOCK_MR])
    assert api_cache.get(key) is not None

    api_cache.invalidate_mr(project_id=1, mr_iid=1)

    assert api_cache.get(key) is None
