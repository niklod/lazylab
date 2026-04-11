from enum import StrEnum
from pathlib import Path

CONFIG_FOLDER = Path.home() / ".config" / "gitlab-tui"
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
