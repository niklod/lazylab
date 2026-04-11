"""E2E tests for app launch and basic UI rendering.

These tests use a mock GitLab client to avoid real API calls.
"""

from unittest.mock import AsyncMock, patch

import pytest
from textual.widgets import Static

from lazylab.lib.constants import MRState
from lazylab.models.gitlab import MergeRequest, MRDiffData, MRDiffFile, Project, User
from lazylab.ui.app import LazyLab
from lazylab.ui.widgets.mr_diff import DiffContentView, DiffFileTree

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

MOCK_DIFF_DATA = MRDiffData(
    files=[
        MRDiffFile(
            old_path="src/main.py",
            new_path="src/main.py",
            diff="@@ -1,3 +1,4 @@\n import os\n-old()\n+new()\n+extra()",
        ),
        MRDiffFile(
            old_path="/dev/null",
            new_path="src/lib/helper.py",
            diff="+def helper():\n+    pass",
            new_file=True,
        ),
        MRDiffFile(
            old_path="README.md",
            new_path="README.md",
            diff="@@ -1 +1 @@\n-old readme\n+new readme",
        ),
    ]
)


@pytest.mark.asyncio
async def test_app_starts_and_shows_main_screen(mock_gitlab, tmp_path):
    """App should launch and push the main screen after auth."""
    _mock_config(tmp_path)
    app = LazyLab()
    async with app.run_test(size=(120, 40)):
        # The app should have pushed the main screen
        assert len(app.screen_stack) >= 1


@pytest.mark.asyncio
async def test_diff_tab_renders_file_tree(mock_gitlab, tmp_path):
    """Diff tab should render file tree and diff content when MR is selected."""
    _mock_config(tmp_path)

    mock_gitlab.get_current_user = AsyncMock(return_value=MOCK_USER)
    mock_gitlab.list_projects = AsyncMock(return_value=MOCK_PROJECTS)

    with patch("lazylab.lib.gitlab.merge_requests.get_mr_changes", new_callable=AsyncMock) as mock_changes:
        mock_changes.return_value = MOCK_DIFF_DATA

        app = LazyLab()
        async with app.run_test(size=(120, 40)) as pilot:
            await pilot.pause()

            from lazylab.lib.context import LazyLabContext
            from lazylab.ui.screens.primary import LazyLabMainScreen

            # Main screen should be pushed after auth
            assert isinstance(app.screen, LazyLabMainScreen)
            main_screen: LazyLabMainScreen = app.screen  # type: ignore[assignment]

            LazyLabContext.current_project = MOCK_PROJECTS[0]
            await main_screen.main_view_pane.load_mr_details(MOCK_MR)
            await pilot.pause()

            # Switch to Diff tab and wait for @work to complete
            tabbed_content = main_screen.query_one("#selection_detail_tabs")
            tabbed_content.active = "mr-diff-tab"  # type: ignore[attr-defined]
            await pilot.pause()
            await app.workers.wait_for_complete()
            await pilot.pause()

            # Assert diff widgets exist and have rendered content
            file_tree = main_screen.query_one(DiffFileTree)
            assert file_tree.root.children, "File tree should have children after loading diff"

            main_screen.query_one(DiffContentView)  # assert exists
            diff_static = main_screen.query_one("#diff-content-static", Static)
            rendered = str(diff_static.render())
            assert "import" in rendered or "new" in rendered or "old" in rendered, (
                f"Diff content should show actual diff text, got: {rendered!r}"
            )


@pytest.mark.asyncio
async def test_tab_switching_with_bracket_keys(mock_gitlab, tmp_path):
    """Tab switching with [ and ] must work from every tab, including Diff."""
    _mock_config(tmp_path)

    with patch("lazylab.lib.gitlab.merge_requests.get_mr_changes", new_callable=AsyncMock) as mock_changes:
        mock_changes.return_value = MOCK_DIFF_DATA

        app = LazyLab()
        async with app.run_test(size=(120, 40)) as pilot:
            await pilot.pause()

            from lazylab.lib.context import LazyLabContext
            from lazylab.ui.screens.primary import LazyLabMainScreen

            assert isinstance(app.screen, LazyLabMainScreen)
            main_screen: LazyLabMainScreen = app.screen  # type: ignore[assignment]

            LazyLabContext.current_project = MOCK_PROJECTS[0]
            await main_screen.main_view_pane.load_mr_details(MOCK_MR)
            await pilot.pause()
            await app.workers.wait_for_complete()
            await pilot.pause()

            tc = main_screen.query_one("#selection_detail_tabs")
            tab_ids = ["mr-overview-tab", "mr-diff-tab", "mr-conversation-tab", "mr-pipeline-tab"]

            # Verify starting on Overview
            assert tc.active == tab_ids[0]  # type: ignore[attr-defined]

            # Cycle forward through ALL tabs with ]
            for i in range(1, len(tab_ids) + 1):
                await pilot.press("]")
                await pilot.pause()
                expected = tab_ids[i % len(tab_ids)]
                actual = tc.active  # type: ignore[attr-defined]
                assert actual == expected, (
                    f"After {i} presses of ], expected tab {expected!r} but got {actual!r}"
                )
                # Focus must never be None (that breaks all subsequent keybindings)
                assert app.focused is not None, (
                    f"Focus is None after switching to tab {actual!r}"
                )

            # Now cycle backward through ALL tabs with [
            current_idx = 0  # we wrapped around to overview
            for i in range(1, len(tab_ids) + 1):
                await pilot.press("[")
                await pilot.pause()
                expected = tab_ids[(current_idx - i) % len(tab_ids)]
                actual = tc.active  # type: ignore[attr-defined]
                assert actual == expected, (
                    f"After {i} presses of [, expected tab {expected!r} but got {actual!r}"
                )
                assert app.focused is not None, (
                    f"Focus is None after switching to tab {actual!r}"
                )
