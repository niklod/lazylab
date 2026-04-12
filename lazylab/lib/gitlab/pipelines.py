from typing import Any

from lazylab.lib.cache import cached
from lazylab.lib.context import LazyLabContext
from lazylab.lib.logging import ll
from lazylab.models.gitlab import Pipeline, PipelineDetail, PipelineJob


def _to_pipeline(gl_pipeline: Any) -> Pipeline:
    """Convert a raw python-gitlab pipeline object to a Pipeline model."""
    return Pipeline(
        id=gl_pipeline.id,
        status=gl_pipeline.status,
        ref=gl_pipeline.ref,
        sha=gl_pipeline.sha,
        web_url=gl_pipeline.web_url,
        created_at=gl_pipeline.created_at,
        updated_at=gl_pipeline.updated_at,
    )


def _to_pipeline_job(gl_job: Any) -> PipelineJob:
    """Convert a raw python-gitlab job object to a PipelineJob model."""
    return PipelineJob(
        id=gl_job.id,
        name=gl_job.name,
        stage=gl_job.stage,
        status=gl_job.status,
        web_url=gl_job.web_url,
        duration=getattr(gl_job, "duration", None),
        allow_failure=getattr(gl_job, "allow_failure", False),
    )


@cached("pipeline_latest", model=Pipeline)
async def get_latest_pipeline_for_mr(project_id: int, mr_iid: int) -> Pipeline | None:
    """Get the latest pipeline associated with a merge request."""
    ll.debug(f"Getting latest pipeline for MR !{mr_iid} in project {project_id}")
    client = LazyLabContext.client

    gl_project = await client.get_raw_project(project_id)

    def _fetch() -> Any:
        mr = gl_project.mergerequests.get(mr_iid)
        pipelines = mr.pipelines.list(per_page=1, get_all=False)
        return pipelines[0] if pipelines else None

    gl_pipeline = await client._run_sync(_fetch)
    if gl_pipeline is None:
        return None

    return _to_pipeline(gl_pipeline)


@cached("pipeline_detail", model=PipelineDetail)
async def get_pipeline_detail(project_id: int, mr_iid: int) -> PipelineDetail | None:
    """Get the latest pipeline with all jobs for a merge request."""
    ll.debug(f"Getting pipeline detail for MR !{mr_iid} in project {project_id}")
    client = LazyLabContext.client

    gl_project = await client.get_raw_project(project_id)

    def _fetch() -> tuple[Any, list[Any]]:
        mr = gl_project.mergerequests.get(mr_iid)
        pipelines = mr.pipelines.list(per_page=1, get_all=False)
        if not pipelines:
            return None, []
        pipeline = gl_project.pipelines.get(pipelines[0].id)
        jobs = pipeline.jobs.list(get_all=True)
        return pipeline, jobs

    gl_pipeline, gl_jobs = await client._run_sync(_fetch)
    if gl_pipeline is None:
        return None

    return PipelineDetail(
        pipeline=_to_pipeline(gl_pipeline),
        jobs=[_to_pipeline_job(j) for j in gl_jobs],
    )


@cached("job_trace")
async def get_job_trace(project_id: int, job_id: int) -> str:
    """Get the log output (trace) of a pipeline job."""
    ll.debug(f"Getting trace for job {job_id} in project {project_id}")
    client = LazyLabContext.client

    gl_project = await client.get_raw_project(project_id)

    def _fetch() -> bytes:
        job = gl_project.jobs.get(job_id)
        return job.trace()

    trace_bytes = await client._run_sync(_fetch)
    return trace_bytes.decode("utf-8", errors="replace")
