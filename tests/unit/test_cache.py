import json
from datetime import datetime, timezone
from pathlib import Path

from lazylab.lib.cache import load_models_from_cache, save_models_to_cache
from lazylab.models.gitlab import Project

SAMPLE_PROJECT = Project(
    id=1,
    name="test-project",
    path_with_namespace="group/test-project",
    default_branch="main",
    web_url="https://gitlab.com/group/test-project",
    last_activity_at=datetime(2026, 1, 1, tzinfo=timezone.utc),
    archived=False,
)


class TestSaveModels:
    def test_save_creates_file(self, tmp_path: Path):
        save_models_to_cache(tmp_path, "group/project", "projects", [SAMPLE_PROJECT])
        expected = tmp_path / "group_project_projects.json"
        assert expected.exists()

    def test_save_without_project_path(self, tmp_path: Path):
        save_models_to_cache(tmp_path, None, "all_projects", [SAMPLE_PROJECT])
        expected = tmp_path / "all_projects.json"
        assert expected.exists()

    def test_save_content_is_valid_json(self, tmp_path: Path):
        save_models_to_cache(tmp_path, None, "projects", [SAMPLE_PROJECT])
        path = tmp_path / "projects.json"
        data = json.loads(path.read_text())
        assert len(data) == 1
        assert data[0]["name"] == "test-project"


class TestLoadModels:
    def test_load_roundtrip(self, tmp_path: Path):
        save_models_to_cache(tmp_path, None, "projects", [SAMPLE_PROJECT])
        loaded = load_models_from_cache(tmp_path, None, "projects", Project)
        assert len(loaded) == 1
        assert loaded[0].id == SAMPLE_PROJECT.id
        assert loaded[0].path_with_namespace == SAMPLE_PROJECT.path_with_namespace

    def test_load_nonexistent_returns_empty(self, tmp_path: Path):
        loaded = load_models_from_cache(tmp_path, None, "missing", Project)
        assert loaded == []

    def test_load_corrupted_json_returns_empty(self, tmp_path: Path):
        path = tmp_path / "bad.json"
        path.write_text("not valid json{{{")
        loaded = load_models_from_cache(tmp_path, None, "bad", Project)
        assert loaded == []

    def test_load_schema_mismatch_returns_empty(self, tmp_path: Path):
        path = tmp_path / "wrong.json"
        path.write_text(json.dumps([{"unexpected_field": "value"}]))
        loaded = load_models_from_cache(tmp_path, None, "wrong", Project)
        assert loaded == []
