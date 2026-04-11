from datetime import datetime, timezone

from lazylab.lib.constants import MRState
from lazylab.models.gitlab import MergeRequest, Project, User


class TestUserModel:
    def test_parse_user(self):
        user = User(
            id=1,
            username="johndoe",
            name="John Doe",
            web_url="https://gitlab.com/johndoe",
        )
        assert user.username == "johndoe"
        assert user.avatar_url is None

    def test_parse_user_with_avatar(self):
        user = User(
            id=1,
            username="johndoe",
            name="John Doe",
            web_url="https://gitlab.com/johndoe",
            avatar_url="https://gitlab.com/uploads/avatar.png",
        )
        assert user.avatar_url == "https://gitlab.com/uploads/avatar.png"


class TestProjectModel:
    def test_parse_project(self):
        project = Project(
            id=42,
            name="my-project",
            path_with_namespace="group/my-project",
            default_branch="main",
            web_url="https://gitlab.com/group/my-project",
            last_activity_at=datetime(2026, 4, 10, 14, 30, 0, tzinfo=timezone.utc),
        )
        assert project.id == 42
        assert project.path_with_namespace == "group/my-project"
        assert project.archived is False

    def test_parse_archived_project(self):
        project = Project(
            id=1,
            name="old",
            path_with_namespace="group/old",
            web_url="https://gitlab.com/group/old",
            last_activity_at=datetime(2020, 1, 1, tzinfo=timezone.utc),
            archived=True,
        )
        assert project.archived is True


class TestMergeRequestModel:
    def test_parse_mr(self):
        mr = MergeRequest(
            id=100,
            iid=5,
            title="Fix bug",
            state=MRState.OPENED,
            author=User(id=1, username="dev", name="Dev", web_url="https://gitlab.com/dev"),
            source_branch="fix-bug",
            target_branch="main",
            web_url="https://gitlab.com/group/project/-/merge_requests/5",
            created_at=datetime(2026, 4, 10, 10, 0, 0, tzinfo=timezone.utc),
            updated_at=datetime(2026, 4, 10, 12, 0, 0, tzinfo=timezone.utc),
        )
        assert mr.iid == 5
        assert mr.state == MRState.OPENED
        assert mr.has_conflicts is False
        assert mr.author.username == "dev"

    def test_parse_merged_mr(self):
        mr = MergeRequest(
            id=101,
            iid=6,
            title="Feature",
            state=MRState.MERGED,
            author=User(id=1, username="dev", name="Dev", web_url="https://gitlab.com/dev"),
            source_branch="feature",
            target_branch="main",
            web_url="https://gitlab.com/group/project/-/merge_requests/6",
            created_at=datetime(2026, 4, 1, tzinfo=timezone.utc),
            updated_at=datetime(2026, 4, 5, tzinfo=timezone.utc),
            merged_at=datetime(2026, 4, 5, tzinfo=timezone.utc),
        )
        assert mr.state == MRState.MERGED
        assert mr.merged_at is not None
