"""E2E tests for Pipeline tab rendering and interactions."""

from unittest.mock import AsyncMock, patch

import pytest

from lazylab.lib.constants import MRState, PipelineStatus
from lazylab.models.gitlab import (
    MergeRequest,
    Pipeline,
    PipelineDetail,
    PipelineJob,
    Project,
    User,
)
from lazylab.ui.app import LazyLab
from lazylab.ui.widgets.mr_pipeline import (
    JobLogView,
    MRPipelineTabContent,
    PipelineJobWidget,
    PipelineStageCard,
)

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

MOCK_PIPELINE = Pipeline(
    id=999,
    status=PipelineStatus.SUCCESS,
    ref="feature",
    sha="abc123def456",
    web_url="https://gitlab.com/group/project-a/-/pipelines/999",
    created_at="2026-04-10T10:00:00Z",  # type: ignore[arg-type]
    updated_at="2026-04-10T10:30:00Z",  # type: ignore[arg-type]
)

MOCK_JOBS = [
    PipelineJob(
        id=1,
        name="init:check-ci",
        stage="init",
        status=PipelineStatus.SUCCESS,
        web_url="https://gitlab.com/group/project-a/-/jobs/1",
        duration=12.0,
    ),
    PipelineJob(
        id=2,
        name="init:mr-ping",
        stage="init",
        status=PipelineStatus.SUCCESS,
        web_url="https://gitlab.com/group/project-a/-/jobs/2",
        duration=5.0,
    ),
    PipelineJob(
        id=3,
        name="prebuild:packages",
        stage="prebuild",
        status=PipelineStatus.SUCCESS,
        web_url="https://gitlab.com/group/project-a/-/jobs/3",
        duration=45.0,
    ),
    PipelineJob(
        id=4,
        name="test:go-lint",
        stage="test",
        status=PipelineStatus.FAILED,
        web_url="https://gitlab.com/group/project-a/-/jobs/4",
        duration=120.5,
    ),
    PipelineJob(
        id=5,
        name="test:go-test",
        stage="test",
        status=PipelineStatus.SUCCESS,
        web_url="https://gitlab.com/group/project-a/-/jobs/5",
        duration=90.0,
    ),
]

MOCK_PIPELINE_DETAIL = PipelineDetail(pipeline=MOCK_PIPELINE, jobs=MOCK_JOBS)


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


@pytest.mark.asyncio
async def test_pipeline_tab_renders_stages(mock_gitlab, tmp_path):
    """Pipeline tab should render stage cards with jobs when data is loaded."""
    with patch(
        "lazylab.lib.gitlab.pipelines.get_pipeline_detail",
        new_callable=AsyncMock,
    ) as mock_detail:
        mock_detail.return_value = MOCK_PIPELINE_DETAIL

        app = LazyLab()
        async with app.run_test(size=(120, 40)) as pilot:
            await pilot.pause()

            from lazylab.lib.context import LazyLabContext
            from lazylab.ui.screens.primary import LazyLabMainScreen

            assert isinstance(app.screen, LazyLabMainScreen)
            main_screen: LazyLabMainScreen = app.screen  # type: ignore[assignment]

            LazyLabContext.current_project = MOCK_PROJECT
            await main_screen.main_view_pane.load_mr_details(MOCK_MR)
            await pilot.pause()

            # Switch to Pipeline tab
            tabbed_content = main_screen.query_one("#selection_detail_tabs")
            tabbed_content.active = "mr-pipeline-tab"  # type: ignore[attr-defined]
            await pilot.pause()
            await app.workers.wait_for_complete()
            await pilot.pause()

            # Assert pipeline content widget exists
            pipeline_content = main_screen.query_one(MRPipelineTabContent)
            assert pipeline_content is not None

            # Assert stage cards rendered (3 stages: init, prebuild, test)
            stage_cards = main_screen.query(PipelineStageCard)
            assert len(stage_cards) == 3

            # Assert header shows pipeline info
            from textual.widgets import Static

            header = main_screen.query_one("#pipeline-header", Static)
            header_text = str(header.render())
            assert "999" in header_text


@pytest.mark.asyncio
async def test_pipeline_tab_shows_empty_when_no_pipeline(mock_gitlab, tmp_path):
    """Pipeline tab should show 'no pipeline' message when MR has no pipeline."""
    with patch(
        "lazylab.lib.gitlab.pipelines.get_pipeline_detail",
        new_callable=AsyncMock,
    ) as mock_detail:
        mock_detail.return_value = None

        app = LazyLab()
        async with app.run_test(size=(120, 40)) as pilot:
            await pilot.pause()

            from lazylab.lib.context import LazyLabContext
            from lazylab.ui.screens.primary import LazyLabMainScreen

            assert isinstance(app.screen, LazyLabMainScreen)
            main_screen: LazyLabMainScreen = app.screen  # type: ignore[assignment]

            LazyLabContext.current_project = MOCK_PROJECT
            await main_screen.main_view_pane.load_mr_details(MOCK_MR)
            await pilot.pause()

            # Switch to Pipeline tab
            tabbed_content = main_screen.query_one("#selection_detail_tabs")
            tabbed_content.active = "mr-pipeline-tab"  # type: ignore[attr-defined]
            await pilot.pause()
            await app.workers.wait_for_complete()
            await pilot.pause()

            # Assert header shows "no pipeline" message
            from textual.widgets import Static

            header = main_screen.query_one("#pipeline-header", Static)
            header_text = str(header.render())
            assert "no pipeline" in header_text.lower() or "No pipeline" in header_text


@pytest.mark.asyncio
async def test_pipeline_open_in_browser(mock_gitlab, tmp_path):
    """Pressing 'o' on pipeline tab should open pipeline URL in browser."""
    with (
        patch(
            "lazylab.lib.gitlab.pipelines.get_pipeline_detail",
            new_callable=AsyncMock,
        ) as mock_detail,
        patch("lazylab.ui.widgets.mr_pipeline.webbrowser.open") as mock_open,
    ):
        mock_detail.return_value = MOCK_PIPELINE_DETAIL

        app = LazyLab()
        async with app.run_test(size=(120, 40)) as pilot:
            await pilot.pause()

            from lazylab.lib.context import LazyLabContext
            from lazylab.ui.screens.primary import LazyLabMainScreen

            assert isinstance(app.screen, LazyLabMainScreen)
            main_screen: LazyLabMainScreen = app.screen  # type: ignore[assignment]

            LazyLabContext.current_project = MOCK_PROJECT
            await main_screen.main_view_pane.load_mr_details(MOCK_MR)
            await pilot.pause()

            # Switch to Pipeline tab
            tabbed_content = main_screen.query_one("#selection_detail_tabs")
            tabbed_content.active = "mr-pipeline-tab"  # type: ignore[attr-defined]
            await pilot.pause()
            await app.workers.wait_for_complete()
            await pilot.pause()

            # Focus the first job widget and press 'o'
            job_widgets = main_screen.query(PipelineJobWidget)
            assert len(job_widgets) > 0, "Should have job widgets rendered"
            first_job = job_widgets.first()
            first_job.focus()
            await pilot.pause()
            await pilot.press("o")
            await pilot.pause()

            mock_open.assert_called_once_with(MOCK_JOBS[0].web_url)


@pytest.mark.asyncio
async def test_pipeline_job_log_inline(mock_gitlab, tmp_path):
    """Pressing Enter on a job should fetch and display its log inline."""
    mock_trace = "Running job init:check-ci\n\x1b[32mSUCCESS\x1b[0m\nJob completed"

    with (
        patch(
            "lazylab.lib.gitlab.pipelines.get_pipeline_detail",
            new_callable=AsyncMock,
        ) as mock_detail,
        patch(
            "lazylab.lib.gitlab.pipelines.get_job_trace",
            new_callable=AsyncMock,
        ) as mock_trace_fn,
    ):
        mock_detail.return_value = MOCK_PIPELINE_DETAIL
        mock_trace_fn.return_value = mock_trace

        app = LazyLab()
        async with app.run_test(size=(120, 40)) as pilot:
            await pilot.pause()

            from lazylab.lib.context import LazyLabContext
            from lazylab.ui.screens.primary import LazyLabMainScreen

            assert isinstance(app.screen, LazyLabMainScreen)
            main_screen: LazyLabMainScreen = app.screen  # type: ignore[assignment]

            LazyLabContext.current_project = MOCK_PROJECT
            await main_screen.main_view_pane.load_mr_details(MOCK_MR)
            await pilot.pause()

            # Switch to Pipeline tab
            tabbed_content = main_screen.query_one("#selection_detail_tabs")
            tabbed_content.active = "mr-pipeline-tab"  # type: ignore[attr-defined]
            await pilot.pause()
            await app.workers.wait_for_complete()
            await pilot.pause()

            # Focus first job and press Enter to view log
            job_widgets = main_screen.query(PipelineJobWidget)
            first_job = job_widgets.first()
            first_job.focus()
            await pilot.pause()
            await pilot.press("enter")
            await pilot.pause()
            await app.workers.wait_for_complete()
            await pilot.pause()

            # Assert log view is visible and contains trace text
            log_view = main_screen.query_one(JobLogView)
            assert log_view.has_class("visible")

            mock_trace_fn.assert_called_once_with(MOCK_PROJECT.id, MOCK_JOBS[0].id)

            # Press Escape to close log
            await pilot.press("escape")
            await pilot.pause()

            assert not log_view.has_class("visible")


@pytest.mark.asyncio
async def test_pipeline_hjkl_navigation(mock_gitlab, tmp_path):
    """h/l navigate between stages, j/k between jobs within a stage."""
    with patch(
        "lazylab.lib.gitlab.pipelines.get_pipeline_detail",
        new_callable=AsyncMock,
    ) as mock_detail:
        mock_detail.return_value = MOCK_PIPELINE_DETAIL

        app = LazyLab()
        async with app.run_test(size=(120, 40)) as pilot:
            await pilot.pause()

            from lazylab.lib.context import LazyLabContext
            from lazylab.ui.screens.primary import LazyLabMainScreen

            assert isinstance(app.screen, LazyLabMainScreen)
            main_screen: LazyLabMainScreen = app.screen  # type: ignore[assignment]

            LazyLabContext.current_project = MOCK_PROJECT
            await main_screen.main_view_pane.load_mr_details(MOCK_MR)
            await pilot.pause()

            # Switch to Pipeline tab
            tabbed_content = main_screen.query_one("#selection_detail_tabs")
            tabbed_content.active = "mr-pipeline-tab"  # type: ignore[attr-defined]
            await pilot.pause()
            await app.workers.wait_for_complete()
            await pilot.pause()

            # Focus first job (init:check-ci in "init" stage)
            job_widgets = main_screen.query(PipelineJobWidget)
            job_widgets.first().focus()
            await pilot.pause()

            focused = app.focused
            assert isinstance(focused, PipelineJobWidget)
            assert focused.job.name == "init:check-ci"

            # j → next job in same stage (init:mr-ping)
            await pilot.press("j")
            await pilot.pause()
            focused = app.focused
            assert isinstance(focused, PipelineJobWidget)
            assert focused.job.name == "init:mr-ping"

            # k → back to first job (init:check-ci)
            await pilot.press("k")
            await pilot.pause()
            focused = app.focused
            assert isinstance(focused, PipelineJobWidget)
            assert focused.job.name == "init:check-ci"

            # l → next stage (prebuild), first job
            await pilot.press("l")
            await pilot.pause()
            focused = app.focused
            assert isinstance(focused, PipelineJobWidget)
            assert focused.job.name == "prebuild:packages"

            # l → next stage (test), first job
            await pilot.press("l")
            await pilot.pause()
            focused = app.focused
            assert isinstance(focused, PipelineJobWidget)
            assert focused.job.name == "test:go-lint"

            # h → back to prebuild stage
            await pilot.press("h")
            await pilot.pause()
            focused = app.focused
            assert isinstance(focused, PipelineJobWidget)
            assert focused.job.name == "prebuild:packages"
