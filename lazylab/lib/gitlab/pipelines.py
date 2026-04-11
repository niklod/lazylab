from typing import Any

from lazylab.lib.context import LazyLabContext
from lazylab.lib.logging import ll
from lazylab.models.gitlab import Pipeline


async def get_latest_pipeline_for_mr(project_id: int, mr_iid: int) -> Pipeline | None:
    """Get the latest pipeline associated with a merge request."""
    ll.debug(f"Getting latest pipeline for MR !{mr_iid} in project {project_id}")
    client = LazyLabContext.client

    gl_project = await client.get_raw_project(project_id)

    def _fetch() -> Any:
        mr = gl_project.mergerequests.get(mr_iid)
        pipelines = mr.pipelines.list(per_page=1)
        return pipelines[0] if pipelines else None

    gl_pipeline = await client._run_sync(_fetch)
    if gl_pipeline is None:
        return None

    return Pipeline(
        id=gl_pipeline.id,
        status=gl_pipeline.status,
        ref=gl_pipeline.ref,
        sha=gl_pipeline.sha,
        web_url=gl_pipeline.web_url,
        created_at=gl_pipeline.created_at,
        updated_at=gl_pipeline.updated_at,
    )
