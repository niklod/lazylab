import os
from contextlib import contextmanager
from pathlib import Path
from typing import Any, Generator

import yaml
from pydantic import BaseModel, field_serializer, field_validator
from textual.theme import BUILTIN_THEMES, Theme

from lazylab.lib.constants import CONFIG_FILE, CONFIG_FOLDER, MROwnerFilter, MRStateFilter


class GitLabConnectionSettings(BaseModel):
    url: str = "https://gitlab.com"
    token: str = ""

    @field_validator("url", mode="before")
    @classmethod
    def strip_trailing_slash(cls, v: str) -> str:
        return v.rstrip("/")


class AppearanceSettings(BaseModel):
    theme: Theme = BUILTIN_THEMES["textual-dark"]
    diff_pager: str | None = None

    @field_serializer("theme")
    @classmethod
    def serialize_theme(cls, theme: Theme | str) -> str:
        return theme.name if isinstance(theme, Theme) else theme

    @field_validator("theme", mode="before")
    @classmethod
    def validate_theme(cls, theme_name: Any) -> Theme:
        if isinstance(theme_name, Theme):
            return theme_name
        return BUILTIN_THEMES.get(theme_name, BUILTIN_THEMES["textual-dark"])


class RepositorySettings(BaseModel):
    favorites: list[str] = []
    sort_by: str = "last_activity"


class MergeRequestSettings(BaseModel):
    state_filter: MRStateFilter = MRStateFilter.OPENED
    owner_filter: MROwnerFilter = MROwnerFilter.ALL


class CacheSettings(BaseModel):
    directory: Path = CONFIG_FOLDER / ".cache"
    ttl: int = 600


class CoreConfig(BaseModel):
    logfile: Path = CONFIG_FOLDER / "lazylab.log"
    logfile_max_bytes: int = 5_000_000
    logfile_count: int = 5


_CONFIG_INSTANCE: "Config | None" = None


class Config(BaseModel):
    gitlab: GitLabConnectionSettings = GitLabConnectionSettings()
    appearance: AppearanceSettings = AppearanceSettings()
    repositories: RepositorySettings = RepositorySettings()
    merge_requests: MergeRequestSettings = MergeRequestSettings()
    cache: CacheSettings = CacheSettings()
    core: CoreConfig = CoreConfig()

    @classmethod
    def load_config(cls) -> "Config":
        global _CONFIG_INSTANCE
        if _CONFIG_INSTANCE is None:
            if CONFIG_FILE.exists():
                raw = yaml.safe_load(CONFIG_FILE.read_text()) or {}
                _CONFIG_INSTANCE = cls(**raw)
            else:
                _CONFIG_INSTANCE = cls()
                _CONFIG_INSTANCE.save()
        return _CONFIG_INSTANCE

    @classmethod
    def reset(cls) -> None:
        global _CONFIG_INSTANCE
        _CONFIG_INSTANCE = None

    def save(self) -> None:
        CONFIG_FOLDER.mkdir(parents=True, exist_ok=True, mode=0o700)
        data = self.model_dump(mode="json")
        CONFIG_FILE.write_text(yaml.dump(data, default_flow_style=False, sort_keys=False))
        os.chmod(CONFIG_FILE, 0o600)

    @classmethod
    @contextmanager
    def to_edit(cls) -> Generator["Config", None, None]:
        current_config = cls.load_config()
        yield current_config
        current_config.save()
