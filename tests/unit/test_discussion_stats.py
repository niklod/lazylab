"""Unit tests for discussion stats: model, API function, and UI helper."""

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from lazylab.lib.gitlab.merge_requests import get_mr_discussion_stats
from lazylab.models.gitlab import DiscussionStats
from lazylab.ui.widgets.merge_requests import _comments_text


# ---------------------------------------------------------------------------
# DiscussionStats model
# ---------------------------------------------------------------------------


class TestDiscussionStatsModel:
    def test_defaults(self):
        stats = DiscussionStats()
        assert stats.total_resolvable == 0
        assert stats.resolved == 0

    def test_with_values(self):
        stats = DiscussionStats(total_resolvable=5, resolved=3)
        assert stats.total_resolvable == 5
        assert stats.resolved == 3


# ---------------------------------------------------------------------------
# _comments_text helper
# ---------------------------------------------------------------------------


class TestCommentsText:
    def test_no_resolvable_discussions(self):
        stats = DiscussionStats(total_resolvable=0, resolved=0)
        result = _comments_text(10, stats)
        assert result == "10"

    def test_all_resolved(self):
        stats = DiscussionStats(total_resolvable=4, resolved=4)
        result = _comments_text(12, stats)
        assert "12" in result
        assert "4/4 resolved" in result
        assert "[green]" in result

    def test_partially_resolved(self):
        stats = DiscussionStats(total_resolvable=5, resolved=2)
        result = _comments_text(8, stats)
        assert "8" in result
        assert "2/5 resolved" in result
        assert "[yellow]" in result

    def test_none_resolved(self):
        stats = DiscussionStats(total_resolvable=3, resolved=0)
        result = _comments_text(6, stats)
        assert "6" in result
        assert "0/3 resolved" in result
        assert "[yellow]" in result


# ---------------------------------------------------------------------------
# get_mr_discussion_stats API function
# ---------------------------------------------------------------------------


def _make_discussion(notes: list[dict]) -> MagicMock:
    disc = MagicMock()
    disc.attributes = {"notes": notes}
    return disc


@pytest.mark.asyncio
async def test_discussion_stats_no_discussions():
    mock_mr = MagicMock()
    mock_mr.discussions.list.return_value = []

    mock_project = MagicMock()
    mock_project.mergerequests.get.return_value = mock_mr

    mock_client = AsyncMock()
    mock_client.get_raw_project = AsyncMock(return_value=mock_project)
    mock_client._run_sync = AsyncMock(side_effect=lambda fn: fn())

    with patch("lazylab.lib.gitlab.merge_requests.LazyLabContext") as mock_ctx:
        mock_ctx.client = mock_client
        stats = await get_mr_discussion_stats.__wrapped__(1, 42)

    assert stats.total_resolvable == 0
    assert stats.resolved == 0


@pytest.mark.asyncio
async def test_discussion_stats_mixed_discussions():
    discussions = [
        # Non-resolvable (system note)
        _make_discussion([{"resolvable": False, "resolved": False}]),
        # Resolvable, resolved
        _make_discussion([
            {"resolvable": True, "resolved": True},
            {"resolvable": True, "resolved": True},
        ]),
        # Resolvable, not resolved
        _make_discussion([
            {"resolvable": True, "resolved": False},
        ]),
        # Resolvable, partially resolved notes (thread not resolved)
        _make_discussion([
            {"resolvable": True, "resolved": True},
            {"resolvable": True, "resolved": False},
        ]),
    ]

    mock_mr = MagicMock()
    mock_mr.discussions.list.return_value = discussions

    mock_project = MagicMock()
    mock_project.mergerequests.get.return_value = mock_mr

    mock_client = AsyncMock()
    mock_client.get_raw_project = AsyncMock(return_value=mock_project)
    mock_client._run_sync = AsyncMock(side_effect=lambda fn: fn())

    with patch("lazylab.lib.gitlab.merge_requests.LazyLabContext") as mock_ctx:
        mock_ctx.client = mock_client
        stats = await get_mr_discussion_stats.__wrapped__(1, 42)

    assert stats.total_resolvable == 3
    assert stats.resolved == 1


@pytest.mark.asyncio
async def test_discussion_stats_all_resolved():
    discussions = [
        _make_discussion([{"resolvable": True, "resolved": True}]),
        _make_discussion([{"resolvable": True, "resolved": True}]),
    ]

    mock_mr = MagicMock()
    mock_mr.discussions.list.return_value = discussions

    mock_project = MagicMock()
    mock_project.mergerequests.get.return_value = mock_mr

    mock_client = AsyncMock()
    mock_client.get_raw_project = AsyncMock(return_value=mock_project)
    mock_client._run_sync = AsyncMock(side_effect=lambda fn: fn())

    with patch("lazylab.lib.gitlab.merge_requests.LazyLabContext") as mock_ctx:
        mock_ctx.client = mock_client
        stats = await get_mr_discussion_stats.__wrapped__(1, 42)

    assert stats.total_resolvable == 2
    assert stats.resolved == 2


@pytest.mark.asyncio
async def test_discussion_stats_empty_notes_skipped():
    discussions = [
        _make_discussion([]),
        _make_discussion([{"resolvable": True, "resolved": True}]),
    ]

    mock_mr = MagicMock()
    mock_mr.discussions.list.return_value = discussions

    mock_project = MagicMock()
    mock_project.mergerequests.get.return_value = mock_mr

    mock_client = AsyncMock()
    mock_client.get_raw_project = AsyncMock(return_value=mock_project)
    mock_client._run_sync = AsyncMock(side_effect=lambda fn: fn())

    with patch("lazylab.lib.gitlab.merge_requests.LazyLabContext") as mock_ctx:
        mock_ctx.client = mock_client
        stats = await get_mr_discussion_stats.__wrapped__(1, 42)

    assert stats.total_resolvable == 1
    assert stats.resolved == 1
