from typing import Any

from lazylab.lib.cache import api_cache, cached
from lazylab.lib.context import LazyLabContext
from lazylab.lib.logging import ll
from lazylab.models.gitlab import ApprovalStatus, MergeRequest, MRDiffData, MRDiffFile, User


def _user_from_dict(data: dict[str, Any]) -> User:
    return User(
        id=data["id"],
        username=data["username"],
        name=data["name"],
        web_url=data["web_url"],
        avatar_url=data.get("avatar_url"),
    )


def _mr_to_model(mr: Any, project_path: str) -> MergeRequest:
    return MergeRequest(
        id=mr.id,
        iid=mr.iid,
        title=mr.title,
        description=getattr(mr, "description", None),
        state=mr.state,
        author=_user_from_dict(mr.author),
        source_branch=mr.source_branch,
        target_branch=mr.target_branch,
        web_url=mr.web_url,
        created_at=mr.created_at,
        updated_at=mr.updated_at,
        merged_at=getattr(mr, "merged_at", None),
        has_conflicts=getattr(mr, "has_conflicts", False),
        merge_status=getattr(mr, "merge_status", ""),
        user_notes_count=getattr(mr, "user_notes_count", 0),
        project_path=project_path,
    )


@cached("mr_list", model=MergeRequest)
async def list_merge_requests(
    project_id: int,
    project_path: str,
    state: str = "opened",
    author_id: int | None = None,
    reviewer_id: int | None = None,
) -> list[MergeRequest]:
    ll.debug(f"Listing MRs for project {project_id} (state={state})")
    client = LazyLabContext.client

    gl_project = await client.get_raw_project(project_id)

    def _fetch() -> Any:
        kwargs: dict[str, Any] = {"state": state, "order_by": "updated_at", "sort": "desc"}
        if author_id is not None:
            kwargs["author_id"] = author_id
        if reviewer_id is not None:
            kwargs["reviewer_id"] = reviewer_id
        return gl_project.mergerequests.list(get_all=True, **kwargs)

    gl_mrs = await client._run_sync(_fetch)
    return [_mr_to_model(mr, project_path) for mr in gl_mrs]


@cached("mr", model=MergeRequest)
async def get_merge_request(project_id: int, mr_iid: int, project_path: str) -> MergeRequest:
    ll.debug(f"Getting MR !{mr_iid} for project {project_id}")
    client = LazyLabContext.client

    gl_project = await client.get_raw_project(project_id)

    def _fetch() -> Any:
        return gl_project.mergerequests.get(mr_iid)

    gl_mr = await client._run_sync(_fetch)
    return _mr_to_model(gl_mr, project_path)


@cached("mr_changes", model=MRDiffData)
async def get_mr_changes(project_id: int, mr_iid: int) -> MRDiffData:
    ll.debug(f"Getting changes for MR !{mr_iid} in project {project_id}")
    client = LazyLabContext.client

    gl_project = await client.get_raw_project(project_id)

    def _fetch() -> Any:
        mr = gl_project.mergerequests.get(mr_iid)
        return mr.changes()

    result = await client._run_sync(_fetch)
    files = [
        MRDiffFile(
            old_path=c["old_path"],
            new_path=c["new_path"],
            diff=c.get("diff", ""),
            new_file=c.get("new_file", False),
            renamed_file=c.get("renamed_file", False),
            deleted_file=c.get("deleted_file", False),
        )
        for c in result.get("changes", [])
    ]
    return MRDiffData(files=files)


async def close_merge_request(project_id: int, mr_iid: int, project_path: str) -> MergeRequest:
    ll.debug(f"Closing MR !{mr_iid} for project {project_id}")
    client = LazyLabContext.client

    gl_project = await client.get_raw_project(project_id)

    def _fetch() -> Any:
        mr = gl_project.mergerequests.get(mr_iid)
        mr.state_event = "close"
        mr.save()
        return mr

    gl_mr = await client._run_sync(_fetch)
    api_cache.invalidate_mr(project_id, mr_iid)
    return _mr_to_model(gl_mr, project_path)


async def merge_merge_request(
    project_id: int,
    mr_iid: int,
    project_path: str,
    should_remove_source_branch: bool = False,
    merge_when_pipeline_succeeds: bool = False,
) -> MergeRequest:
    ll.debug(f"Merging MR !{mr_iid} for project {project_id}")
    client = LazyLabContext.client

    gl_project = await client.get_raw_project(project_id)

    def _fetch() -> Any:
        mr = gl_project.mergerequests.get(mr_iid)
        mr.merge(
            should_remove_source_branch=should_remove_source_branch,
            merge_when_pipeline_succeeds=merge_when_pipeline_succeeds,
        )
        return gl_project.mergerequests.get(mr_iid)

    gl_mr = await client._run_sync(_fetch)
    api_cache.invalidate_mr(project_id, mr_iid)
    return _mr_to_model(gl_mr, project_path)


@cached("mr_approvals", model=ApprovalStatus)
async def get_mr_approvals(project_id: int, mr_iid: int) -> ApprovalStatus:
    ll.debug(f"Getting approvals for MR !{mr_iid}")
    client = LazyLabContext.client

    gl_project = await client.get_raw_project(project_id)

    def _fetch() -> Any:
        mr = gl_project.mergerequests.get(mr_iid)
        return mr.approvals.get()

    approval_data = await client._run_sync(_fetch)
    approved_by = [_user_from_dict(a["user"]) for a in getattr(approval_data, "approved_by", [])]
    return ApprovalStatus(
        approved=getattr(approval_data, "approved", False),
        approvals_required=getattr(approval_data, "approvals_required", 0),
        approvals_left=getattr(approval_data, "approvals_left", 0),
        approved_by=approved_by,
    )
