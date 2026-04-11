import asyncio
from typing import Any, Callable, TypeVar

import gitlab
from anyio import CapacityLimiter, to_thread

from lazylab.lib.config import Config
from lazylab.lib.logging import ll
from lazylab.models.gitlab import Project, User

T = TypeVar("T")


def _to_project(gl_project: Any) -> Project:
    return Project(
        id=gl_project.id,
        name=gl_project.name,
        path_with_namespace=gl_project.path_with_namespace,
        default_branch=getattr(gl_project, "default_branch", None),
        web_url=gl_project.web_url,
        last_activity_at=gl_project.last_activity_at,
        archived=gl_project.archived,
    )


class GitLabClient:
    def __init__(self, config: Config) -> None:
        if not config.gitlab.token:
            raise ValueError("GitLab token is not configured")
        self._gl = gitlab.Gitlab(
            url=config.gitlab.url,
            private_token=config.gitlab.token,
        )
        self._user: User | None = None
        self._user_lock = asyncio.Lock()
        # Serialize all python-gitlab calls — requests.Session is not thread-safe
        self._limiter = CapacityLimiter(1)

    async def _run_sync(self, fn: Callable[..., T], *args: Any) -> T:
        return await to_thread.run_sync(lambda: fn(*args), limiter=self._limiter)

    async def get_current_user(self) -> User:
        async with self._user_lock:
            if self._user is None:
                ll.debug("Fetching current user")
                await self._run_sync(self._gl.auth)
                gl_user = self._gl.user
                if gl_user is None:
                    raise RuntimeError("GitLab authentication failed: no user returned")
                self._user = User(
                    id=gl_user.id,
                    username=gl_user.username,
                    name=gl_user.name,
                    web_url=gl_user.web_url,
                    avatar_url=getattr(gl_user, "avatar_url", None),
                )
        return self._user

    async def get_raw_project(self, project_id: int | str) -> Any:
        """Get raw python-gitlab project object for internal use."""
        return await self._run_sync(self._gl.projects.get, project_id)

    async def list_projects(
        self,
        membership: bool = True,
        archived: bool = False,
        order_by: str = "last_activity_at",
        sort: str = "desc",
    ) -> list[Project]:
        ll.debug(f"Listing projects (membership={membership}, order_by={order_by})")

        def _fetch() -> Any:
            return self._gl.projects.list(
                membership=membership,
                archived=archived,
                order_by=order_by,
                sort=sort,
                get_all=True,
            )

        gl_projects = await self._run_sync(_fetch)
        return [_to_project(p) for p in gl_projects]

    async def get_project(self, project_id: int) -> Project:
        ll.debug(f"Getting project {project_id}")
        gl_project = await self.get_raw_project(project_id)
        return _to_project(gl_project)

    async def get_project_by_path(self, path: str) -> Project:
        ll.debug(f"Getting project by path: {path}")
        gl_project = await self.get_raw_project(path)
        return _to_project(gl_project)
