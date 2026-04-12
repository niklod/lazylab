from textual.message import Message

from lazylab.models.gitlab import MergeRequest, PipelineJob, Project


class RepoSelected(Message):
    def __init__(self, project: Project) -> None:
        super().__init__()
        self.project = project


class MRSelected(Message):
    def __init__(self, mr: MergeRequest, focus_details: bool = True) -> None:
        super().__init__()
        self.mr = mr
        self.focus_details = focus_details


class JobSelected(Message):
    def __init__(self, job: PipelineJob) -> None:
        super().__init__()
        self.job = job


class MRListRefreshed(Message):
    def __init__(self, project: Project, merge_requests: list[MergeRequest]) -> None:
        super().__init__()
        self.project = project
        self.merge_requests = merge_requests


class MRActionCompleted(Message):
    """Posted after a successful MR close or merge action."""

    def __init__(self, mr: MergeRequest) -> None:
        super().__init__()
        self.mr = mr


class CacheRefreshed(Message):
    """Posted when a background cache refresh completes for a stale entry."""

    def __init__(self, cache_namespace: str, cache_key: str) -> None:
        super().__init__()
        self.cache_namespace = cache_namespace
        self.cache_key = cache_key
