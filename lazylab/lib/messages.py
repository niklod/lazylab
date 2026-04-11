from textual.message import Message

from lazylab.models.gitlab import MergeRequest, Project


class RepoSelected(Message):
    def __init__(self, project: Project) -> None:
        super().__init__()
        self.project = project


class MRSelected(Message):
    def __init__(self, mr: MergeRequest, focus_details: bool = True) -> None:
        super().__init__()
        self.mr = mr
        self.focus_details = focus_details


class MRListRefreshed(Message):
    def __init__(self, project: Project, merge_requests: list[MergeRequest]) -> None:
        super().__init__()
        self.project = project
        self.merge_requests = merge_requests
