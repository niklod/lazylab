from enum import StrEnum
from pathlib import Path

CONFIG_FOLDER = Path.home() / ".config" / "lazylab"
CONFIG_FILE = CONFIG_FOLDER / "config.yaml"

# Symbols used in UI tables
IS_FAVORITED = "[green]\u2605[/]"
IS_NOT_FAVORITED = "\u2606"
CHECKMARK = "\u2713"
X_MARK = "\u2718"
BULLET_POINT = "\u2022"


def favorite_string(favorite: bool) -> str:
    return IS_FAVORITED if favorite else IS_NOT_FAVORITED


class MRStateFilter(StrEnum):
    ALL = "all"
    OPENED = "opened"
    CLOSED = "closed"
    MERGED = "merged"


class MROwnerFilter(StrEnum):
    ALL = "all"
    MINE = "mine"
    REVIEWER = "reviewer"


class MRState(StrEnum):
    OPENED = "opened"
    CLOSED = "closed"
    MERGED = "merged"


class PipelineStatus(StrEnum):
    CREATED = "created"
    WAITING_FOR_RESOURCE = "waiting_for_resource"
    PREPARING = "preparing"
    PENDING = "pending"
    RUNNING = "running"
    SUCCESS = "success"
    FAILED = "failed"
    CANCELED = "canceled"
    SKIPPED = "skipped"
    MANUAL = "manual"
    SCHEDULED = "scheduled"


PIPELINE_JOB_STATUS_ICONS: dict[PipelineStatus, str] = {
    PipelineStatus.SUCCESS: "[green]\u2713[/]",
    PipelineStatus.FAILED: "[red]\u2718[/]",
    PipelineStatus.RUNNING: "[yellow]\u25b6[/]",
    PipelineStatus.PENDING: "[yellow]\u25cb[/]",
    PipelineStatus.CREATED: "[dim]\u25cb[/]",
    PipelineStatus.WAITING_FOR_RESOURCE: "[yellow]\u25cb[/]",
    PipelineStatus.PREPARING: "[yellow]\u25cb[/]",
    PipelineStatus.CANCELED: "[dim]\u2718[/]",
    PipelineStatus.SKIPPED: "[dim]\u2298[/]",
    PipelineStatus.MANUAL: "[cyan]\u25b6[/]",
    PipelineStatus.SCHEDULED: "[cyan]\u23f2[/]",
}
