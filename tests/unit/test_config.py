from pathlib import Path

import yaml

from lazylab.lib.config import (
    AppearanceSettings,
    Config,
    GitLabConnectionSettings,
    MergeRequestSettings,
    RepositorySettings,
)
from lazylab.lib.constants import MROwnerFilter, MRStateFilter


class TestConfigDefaults:
    def test_default_gitlab_url(self):
        config = Config()
        assert config.gitlab.url == "https://gitlab.com"

    def test_default_token_empty(self):
        config = Config()
        assert config.gitlab.token == ""

    def test_default_theme(self):
        config = Config()
        assert config.appearance.theme.name == "textual-dark"

    def test_default_diff_pager_none(self):
        config = Config()
        assert config.appearance.diff_pager is None

    def test_default_favorites_empty(self):
        config = Config()
        assert config.repositories.favorites == []

    def test_default_mr_state_filter(self):
        config = Config()
        assert config.merge_requests.state_filter == MRStateFilter.OPENED

    def test_default_mr_owner_filter(self):
        config = Config()
        assert config.merge_requests.owner_filter == MROwnerFilter.ALL

    def test_default_cache_ttl(self):
        config = Config()
        assert config.cache.ttl == 600


class TestConfigLoadSave:
    def test_save_and_load(self, tmp_path: Path, monkeypatch):
        config_file = tmp_path / "config.yaml"
        monkeypatch.setattr("lazylab.lib.config.CONFIG_FILE", config_file)
        monkeypatch.setattr("lazylab.lib.config.CONFIG_FOLDER", tmp_path)

        config = Config(
            gitlab=GitLabConnectionSettings(url="https://my-gitlab.com", token="test-token"),
            repositories=RepositorySettings(favorites=["group/project"]),
        )
        config.save()

        assert config_file.exists()
        raw = yaml.safe_load(config_file.read_text())
        assert raw["gitlab"]["url"] == "https://my-gitlab.com"
        assert raw["gitlab"]["token"] == "test-token"
        assert raw["repositories"]["favorites"] == ["group/project"]

    def test_load_config_creates_default(self, tmp_path: Path, monkeypatch):
        config_file = tmp_path / "config.yaml"
        monkeypatch.setattr("lazylab.lib.config.CONFIG_FILE", config_file)
        monkeypatch.setattr("lazylab.lib.config.CONFIG_FOLDER", tmp_path)

        config = Config.load_config()
        assert config.gitlab.url == "https://gitlab.com"
        assert config_file.exists()

    def test_load_existing_config(self, tmp_path: Path, monkeypatch):
        config_file = tmp_path / "config.yaml"
        config_file.write_text(
            yaml.dump(
                {
                    "gitlab": {"url": "https://custom.gitlab.com", "token": "my-token"},
                    "repositories": {"favorites": ["a/b"]},
                }
            )
        )
        monkeypatch.setattr("lazylab.lib.config.CONFIG_FILE", config_file)
        monkeypatch.setattr("lazylab.lib.config.CONFIG_FOLDER", tmp_path)

        config = Config.load_config()
        assert config.gitlab.url == "https://custom.gitlab.com"
        assert config.gitlab.token == "my-token"
        assert config.repositories.favorites == ["a/b"]

    def test_roundtrip(self, tmp_path: Path, monkeypatch):
        config_file = tmp_path / "config.yaml"
        monkeypatch.setattr("lazylab.lib.config.CONFIG_FILE", config_file)
        monkeypatch.setattr("lazylab.lib.config.CONFIG_FOLDER", tmp_path)

        original = Config(
            gitlab=GitLabConnectionSettings(url="https://test.com", token="tok"),
            merge_requests=MergeRequestSettings(
                state_filter=MRStateFilter.MERGED, owner_filter=MROwnerFilter.MINE
            ),
        )
        original.save()

        Config.reset()
        loaded = Config.load_config()
        assert loaded.gitlab.url == "https://test.com"
        assert loaded.gitlab.token == "tok"
        assert loaded.merge_requests.state_filter == MRStateFilter.MERGED
        assert loaded.merge_requests.owner_filter == MROwnerFilter.MINE

    def test_to_edit_context_manager(self, tmp_path: Path, monkeypatch):
        config_file = tmp_path / "config.yaml"
        monkeypatch.setattr("lazylab.lib.config.CONFIG_FILE", config_file)
        monkeypatch.setattr("lazylab.lib.config.CONFIG_FOLDER", tmp_path)

        Config.load_config()
        with Config.to_edit() as cfg:
            cfg.repositories.favorites.append("new/repo")

        Config.reset()
        reloaded = Config.load_config()
        assert "new/repo" in reloaded.repositories.favorites


class TestConfigValidation:
    def test_url_strips_trailing_slash(self):
        config = Config(gitlab=GitLabConnectionSettings(url="https://gitlab.com/", token=""))
        assert config.gitlab.url == "https://gitlab.com"

    def test_invalid_theme_falls_back(self):
        config = Config(
            appearance=AppearanceSettings.model_validate({"theme": "nonexistent-theme"})
        )
        assert config.appearance.theme.name == "textual-dark"
