from lazylab.lib.context import LazyLabContext
from lazylab.models.gitlab import Project


async def list_projects(
    membership: bool = True,
    archived: bool = False,
    order_by: str = "last_activity_at",
    sort: str = "desc",
) -> list[Project]:
    return await LazyLabContext.client.list_projects(
        membership=membership,
        archived=archived,
        order_by=order_by,
        sort=sort,
    )


async def get_project(project_id: int) -> Project:
    return await LazyLabContext.client.get_project(project_id)


async def get_project_by_path(path: str) -> Project:
    return await LazyLabContext.client.get_project_by_path(path)
