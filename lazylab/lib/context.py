from __future__ import annotations

from logging.handlers import RotatingFileHandler
from typing import TYPE_CHECKING

from lazylab.lib.config import Config
from lazylab.lib.logging import LazyLabLogFormatter, ll

if TYPE_CHECKING:
    from lazylab.lib.gitlab.client import GitLabClient
    from lazylab.models.gitlab import Project


class _LazyLabContext:
    _config: Config | None = None
    _client: GitLabClient | None = None

    current_project: Project | None = None

    @classmethod
    def _setup_logging_handler(cls, config: Config) -> None:
        try:
            config.core.logfile.parent.mkdir(parents=True, exist_ok=True)
            handler = RotatingFileHandler(
                filename=config.core.logfile,
                maxBytes=config.core.logfile_max_bytes,
                backupCount=config.core.logfile_count,
            )
            handler.setFormatter(LazyLabLogFormatter())
            ll.addHandler(handler)
        except Exception:
            ll.exception("Failed to setup file logger")

    @property
    def config(self) -> Config:
        if self._config is None:
            self._config = Config.load_config()
            self._setup_logging_handler(self._config)
        return self._config

    @property
    def client(self) -> GitLabClient:
        if self._client is None:
            from lazylab.lib.gitlab.client import GitLabClient

            self._client = GitLabClient(self.config)
        return self._client


LazyLabContext = _LazyLabContext()
