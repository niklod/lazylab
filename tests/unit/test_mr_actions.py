"""Unit tests for MR close and merge API functions."""

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from lazylab.lib.constants import MRState
from lazylab.lib.gitlab.merge_requests import close_merge_request, merge_merge_request

MOCK_AUTHOR = {
    "id": 1,
    "username": "testuser",
    "name": "Test User",
    "web_url": "https://gitlab.com/testuser",
}


def _make_gl_mr(*, state: str = "opened", iid: int = 42) -> MagicMock:
    """Create a mock python-gitlab MR object."""
    mr = MagicMock()
    mr.id = 100
    mr.iid = iid
    mr.title = "Test MR"
    mr.description = "A test merge request"
    mr.state = state
    mr.author = MOCK_AUTHOR
    mr.source_branch = "feature"
    mr.target_branch = "main"
    mr.web_url = f"https://gitlab.com/group/project/-/merge_requests/{iid}"
    mr.created_at = "2026-04-10T10:00:00Z"
    mr.updated_at = "2026-04-10T10:00:00Z"
    mr.merged_at = None
    mr.has_conflicts = False
    mr.merge_status = "can_be_merged"
    mr.user_notes_count = 0
    return mr


@pytest.mark.asyncio
async def test_close_merge_request_calls_save_with_close_event():
    gl_mr = _make_gl_mr(state="opened")
    closed_mr = _make_gl_mr(state="closed")
    closed_mr.state = "closed"

    mock_project = MagicMock()
    mock_project.mergerequests.get.return_value = gl_mr
    # After save(), the same object is returned but state_event was set
    gl_mr.save.return_value = None

    mock_client = AsyncMock()
    mock_client.get_raw_project = AsyncMock(return_value=mock_project)
    mock_client._run_sync = AsyncMock(side_effect=lambda fn: fn())

    with patch("lazylab.lib.gitlab.merge_requests.LazyLabContext") as mock_ctx:
        mock_ctx.client = mock_client
        result = await close_merge_request(1, 42, "group/project")

    # Verify state_event was set and save was called
    assert gl_mr.state_event == "close"
    gl_mr.save.assert_called_once()
    assert result.iid == 42


@pytest.mark.asyncio
async def test_merge_merge_request_calls_merge_and_refetches():
    gl_mr = _make_gl_mr(state="opened")
    merged_mr = _make_gl_mr(state="merged")
    merged_mr.merged_at = "2026-04-10T12:00:00Z"

    mock_project = MagicMock()
    # First get() returns the MR for merge(), second get() is the re-fetch
    mock_project.mergerequests.get.side_effect = [gl_mr, merged_mr]

    mock_client = AsyncMock()
    mock_client.get_raw_project = AsyncMock(return_value=mock_project)
    mock_client._run_sync = AsyncMock(side_effect=lambda fn: fn())

    with patch("lazylab.lib.gitlab.merge_requests.LazyLabContext") as mock_ctx:
        mock_ctx.client = mock_client
        result = await merge_merge_request(
            1, 42, "group/project",
            should_remove_source_branch=True,
            merge_when_pipeline_succeeds=False,
        )

    gl_mr.merge.assert_called_once_with(
        should_remove_source_branch=True,
        merge_when_pipeline_succeeds=False,
    )
    assert mock_project.mergerequests.get.call_count == 2
    assert result.state == MRState.MERGED


@pytest.mark.asyncio
async def test_merge_merge_request_default_options():
    gl_mr = _make_gl_mr(state="opened")
    merged_mr = _make_gl_mr(state="merged")

    mock_project = MagicMock()
    mock_project.mergerequests.get.side_effect = [gl_mr, merged_mr]

    mock_client = AsyncMock()
    mock_client.get_raw_project = AsyncMock(return_value=mock_project)
    mock_client._run_sync = AsyncMock(side_effect=lambda fn: fn())

    with patch("lazylab.lib.gitlab.merge_requests.LazyLabContext") as mock_ctx:
        mock_ctx.client = mock_client
        result = await merge_merge_request(1, 42, "group/project")

    gl_mr.merge.assert_called_once_with(
        should_remove_source_branch=False,
        merge_when_pipeline_succeeds=False,
    )
    assert result.iid == 42


@pytest.mark.asyncio
async def test_close_merge_request_returns_correct_model():
    gl_mr = _make_gl_mr(state="closed")

    mock_project = MagicMock()
    mock_project.mergerequests.get.return_value = gl_mr

    mock_client = AsyncMock()
    mock_client.get_raw_project = AsyncMock(return_value=mock_project)
    mock_client._run_sync = AsyncMock(side_effect=lambda fn: fn())

    with patch("lazylab.lib.gitlab.merge_requests.LazyLabContext") as mock_ctx:
        mock_ctx.client = mock_client
        result = await close_merge_request(1, 42, "group/project")

    assert result.id == 100
    assert result.iid == 42
    assert result.title == "Test MR"
    assert result.project_path == "group/project"
    assert result.author.username == "testuser"
