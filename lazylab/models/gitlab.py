from datetime import datetime

from pydantic import BaseModel

from lazylab.lib.constants import MRState, PipelineStatus


class User(BaseModel):
    id: int
    username: str
    name: str
    web_url: str
    avatar_url: str | None = None


class Project(BaseModel):
    id: int
    name: str
    path_with_namespace: str
    default_branch: str | None = None
    web_url: str
    last_activity_at: datetime
    archived: bool = False


class ApprovalStatus(BaseModel):
    approved: bool
    approvals_required: int
    approvals_left: int
    approved_by: list[User] = []


class Pipeline(BaseModel):
    id: int
    status: PipelineStatus
    ref: str
    sha: str
    web_url: str
    created_at: datetime
    updated_at: datetime


class MergeRequest(BaseModel):
    id: int
    iid: int
    title: str
    description: str | None = None
    state: MRState
    author: User
    source_branch: str
    target_branch: str
    web_url: str
    created_at: datetime
    updated_at: datetime
    merged_at: datetime | None = None
    has_conflicts: bool = False
    merge_status: str = ""
    user_notes_count: int = 0
    # Project path for context
    project_path: str = ""


class MRDiffFile(BaseModel):
    old_path: str
    new_path: str
    diff: str
    new_file: bool = False
    renamed_file: bool = False
    deleted_file: bool = False


class MRDiffData(BaseModel):
    files: list[MRDiffFile] = []
