"""E2E tests for MR close and merge actions."""

from unittest.mock import AsyncMock, patch

import pytest

from lazylab.lib.constants import MRState
from lazylab.models.gitlab import MergeRequest, Project, User
from lazylab.ui.app import LazyLab
from lazylab.ui.screens.mr_actions import CloseMRScreen, MergeMRScreen

MOCK_USER = User(
    id=1,
    username="testuser",
    name="Test User",
    web_url="https://gitlab.com/testuser",
)

MOCK_PROJECT = Project(
    id=1,
    name="project-a",
    path_with_namespace="group/project-a",
    default_branch="main",
    web_url="https://gitlab.com/group/project-a",
    last_activity_at="2026-04-10T10:00:00Z",  # type: ignore[arg-type]
)

MOCK_MR_OPENED = MergeRequest(
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

MOCK_MR_CLOSED = MergeRequest(
    id=100,
    iid=1,
    title="Test MR",
    state=MRState.CLOSED,
    author=MOCK_USER,
    source_branch="feature",
    target_branch="main",
    web_url="https://gitlab.com/group/project-a/-/merge_requests/1",
    created_at="2026-04-10T10:00:00Z",  # type: ignore[arg-type]
    updated_at="2026-04-10T10:00:00Z",  # type: ignore[arg-type]
    project_path="group/project-a",
)

MOCK_MR_MERGED = MergeRequest(
    id=100,
    iid=1,
    title="Test MR",
    state=MRState.MERGED,
    author=MOCK_USER,
    source_branch="feature",
    target_branch="main",
    web_url="https://gitlab.com/group/project-a/-/merge_requests/1",
    created_at="2026-04-10T10:00:00Z",  # type: ignore[arg-type]
    updated_at="2026-04-10T10:00:00Z",  # type: ignore[arg-type]
    merged_at="2026-04-10T12:00:00Z",  # type: ignore[arg-type]
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
    mock_client.list_projects = AsyncMock(return_value=[MOCK_PROJECT])

    with patch(
        "lazylab.lib.context._LazyLabContext.client",
        new_callable=lambda: property(lambda self: mock_client),
    ):
        yield mock_client


async def _setup_mr_list(app, pilot, mr_list):
    """Load MRs into the MR container and focus the table."""
    await pilot.pause()

    from lazylab.lib.context import LazyLabContext
    from lazylab.ui.screens.primary import LazyLabMainScreen

    assert isinstance(app.screen, LazyLabMainScreen)
    main_screen: LazyLabMainScreen = app.screen  # type: ignore[assignment]

    LazyLabContext.current_project = MOCK_PROJECT

    # Directly populate the MR container's table
    mr_container = main_screen.main_view_pane.selections.merge_requests
    mr_container._current_project = MOCK_PROJECT
    mr_container.searchable_table.clear_rows()
    mr_container.searchable_table.add_items(mr_list, write_to_cache=False)
    await pilot.pause()

    # Focus the MR table
    mr_container.table.focus()
    await pilot.pause()

    return main_screen


@pytest.mark.asyncio
async def test_close_mr_shows_modal(mock_gitlab, tmp_path):
    """Pressing 'x' on an opened MR should push CloseMRScreen."""
    _mock_config(tmp_path)

    app = LazyLab()
    async with app.run_test(size=(120, 40)) as pilot:
        await _setup_mr_list(app, pilot, [MOCK_MR_OPENED])

        await pilot.press("x")
        await pilot.pause()

        assert isinstance(app.screen, CloseMRScreen)


@pytest.mark.asyncio
async def test_close_mr_executes(mock_gitlab, tmp_path):
    """Confirming close modal should call close API and refresh list."""
    _mock_config(tmp_path)

    with patch(
        "lazylab.lib.gitlab.merge_requests.close_merge_request",
        new_callable=AsyncMock,
    ) as mock_close:
        mock_close.return_value = MOCK_MR_CLOSED

        app = LazyLab()
        async with app.run_test(size=(120, 40)) as pilot:
            await _setup_mr_list(app, pilot, [MOCK_MR_OPENED])

            await pilot.press("x")
            await pilot.pause()

            assert isinstance(app.screen, CloseMRScreen)

            # Click the Close button (submit)
            await pilot.click("#submit")
            await pilot.pause()
            await app.workers.wait_for_complete()
            await pilot.pause()

            mock_close.assert_called_once_with(
                MOCK_PROJECT.id, MOCK_MR_OPENED.iid, MOCK_PROJECT.path_with_namespace
            )


@pytest.mark.asyncio
async def test_close_mr_cancel(mock_gitlab, tmp_path):
    """Pressing Escape on close modal should not call the API."""
    _mock_config(tmp_path)

    with patch(
        "lazylab.lib.gitlab.merge_requests.close_merge_request",
        new_callable=AsyncMock,
    ) as mock_close:
        app = LazyLab()
        async with app.run_test(size=(120, 40)) as pilot:
            await _setup_mr_list(app, pilot, [MOCK_MR_OPENED])

            await pilot.press("x")
            await pilot.pause()

            assert isinstance(app.screen, CloseMRScreen)

            await pilot.press("escape")
            await pilot.pause()

            mock_close.assert_not_called()


@pytest.mark.asyncio
async def test_close_mr_guard_non_opened(mock_gitlab, tmp_path):
    """Pressing 'x' on a closed MR should not push modal."""
    _mock_config(tmp_path)

    app = LazyLab()
    async with app.run_test(size=(120, 40)) as pilot:
        await _setup_mr_list(app, pilot, [MOCK_MR_CLOSED])

        await pilot.press("x")
        await pilot.pause()

        # Should NOT push the modal screen
        assert not isinstance(app.screen, CloseMRScreen)


@pytest.mark.asyncio
async def test_merge_mr_shows_modal(mock_gitlab, tmp_path):
    """Pressing 'M' on an opened MR should push MergeMRScreen."""
    _mock_config(tmp_path)

    app = LazyLab()
    async with app.run_test(size=(120, 40)) as pilot:
        await _setup_mr_list(app, pilot, [MOCK_MR_OPENED])

        await pilot.press("M")
        await pilot.pause()

        assert isinstance(app.screen, MergeMRScreen)


@pytest.mark.asyncio
async def test_merge_mr_executes(mock_gitlab, tmp_path):
    """Confirming merge modal should call merge API."""
    _mock_config(tmp_path)

    with patch(
        "lazylab.lib.gitlab.merge_requests.merge_merge_request",
        new_callable=AsyncMock,
    ) as mock_merge:
        mock_merge.return_value = MOCK_MR_MERGED

        app = LazyLab()
        async with app.run_test(size=(120, 40)) as pilot:
            await _setup_mr_list(app, pilot, [MOCK_MR_OPENED])

            await pilot.press("M")
            await pilot.pause()

            assert isinstance(app.screen, MergeMRScreen)

            # Click the Merge button (submit)
            await pilot.click("#submit")
            await pilot.pause()
            await app.workers.wait_for_complete()
            await pilot.pause()

            mock_merge.assert_called_once_with(
                MOCK_PROJECT.id,
                MOCK_MR_OPENED.iid,
                MOCK_PROJECT.path_with_namespace,
                should_remove_source_branch=False,
                merge_when_pipeline_succeeds=False,
            )


@pytest.mark.asyncio
async def test_merge_mr_cancel(mock_gitlab, tmp_path):
    """Pressing Escape on merge modal should not call the API."""
    _mock_config(tmp_path)

    with patch(
        "lazylab.lib.gitlab.merge_requests.merge_merge_request",
        new_callable=AsyncMock,
    ) as mock_merge:
        app = LazyLab()
        async with app.run_test(size=(120, 40)) as pilot:
            await _setup_mr_list(app, pilot, [MOCK_MR_OPENED])

            await pilot.press("M")
            await pilot.pause()

            assert isinstance(app.screen, MergeMRScreen)

            await pilot.press("escape")
            await pilot.pause()

            mock_merge.assert_not_called()


@pytest.mark.asyncio
async def test_merge_mr_guard_non_opened(mock_gitlab, tmp_path):
    """Pressing 'M' on a merged MR should not push modal."""
    _mock_config(tmp_path)

    app = LazyLab()
    async with app.run_test(size=(120, 40)) as pilot:
        await _setup_mr_list(app, pilot, [MOCK_MR_MERGED])

        await pilot.press("M")
        await pilot.pause()

        assert not isinstance(app.screen, MergeMRScreen)
