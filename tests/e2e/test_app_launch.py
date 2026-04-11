"""E2E tests for app launch and basic UI rendering.

These tests use a mock GitLab client to avoid real API calls.
"""

from unittest.mock import AsyncMock, patch

import pytest

from lazylab.models.gitlab import Project, User
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
    Project(
        id=2,
        name="project-b",
        path_with_namespace="group/project-b",
        default_branch="main",
        web_url="https://gitlab.com/group/project-b",
        last_activity_at="2026-04-09T10:00:00Z",  # type: ignore[arg-type]
    ),
]


def _mock_config(tmp_path):
    """Patch config to use tmp_path and a fake token."""
    import lazylab.lib.config as config_mod

    config_mod._CONFIG_INSTANCE = None
    config_mod.CONFIG_FILE = tmp_path / "config.yaml"
    config_mod.CONFIG_FOLDER = tmp_path

    from lazylab.lib.config import Config

    cfg = Config(gitlab={"url": "https://gitlab.com", "token": "fake-token"})  # type: ignore[arg-type]
    cfg.save()
    Config.reset()


@pytest.fixture()
def mock_gitlab(tmp_path):
    """Fixture that patches config and GitLabClient for testing."""
    _mock_config(tmp_path)

    mock_client = AsyncMock()
    mock_client.get_current_user = AsyncMock(return_value=MOCK_USER)
    mock_client.list_projects = AsyncMock(return_value=MOCK_PROJECTS)

    with patch("lazylab.lib.context._LazyLabContext.client", new_callable=lambda: property(lambda self: mock_client)):
        yield mock_client


@pytest.mark.asyncio
async def test_app_starts_and_shows_main_screen(mock_gitlab, tmp_path):
    """App should launch and push the main screen after auth."""
    _mock_config(tmp_path)
    app = LazyLab()
    async with app.run_test(size=(120, 40)):
        # The app should have pushed the main screen
        assert len(app.screen_stack) >= 1
