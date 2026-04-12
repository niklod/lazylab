from datetime import datetime, timezone

from lazylab.lib.constants import PIPELINE_JOB_STATUS_ICONS, PipelineStatus
from lazylab.models.gitlab import Pipeline, PipelineDetail, PipelineJob
from lazylab.ui.widgets.mr_pipeline import _format_duration, _group_jobs_by_stage


def _make_pipeline(**overrides) -> Pipeline:
    defaults = {
        "id": 1,
        "status": PipelineStatus.SUCCESS,
        "ref": "main",
        "sha": "abc123def456",
        "web_url": "https://gitlab.com/group/project/-/pipelines/1",
        "created_at": datetime(2026, 4, 10, 10, 0, 0, tzinfo=timezone.utc),
        "updated_at": datetime(2026, 4, 10, 10, 30, 0, tzinfo=timezone.utc),
    }
    return Pipeline(**{**defaults, **overrides})


def _make_job(**overrides) -> PipelineJob:
    defaults = {
        "id": 1,
        "name": "test-job",
        "stage": "test",
        "status": PipelineStatus.SUCCESS,
        "web_url": "https://gitlab.com/group/project/-/jobs/1",
    }
    return PipelineJob(**{**defaults, **overrides})


class TestPipelineJobModel:
    def test_create_with_required_fields(self):
        job = _make_job()
        assert job.id == 1
        assert job.name == "test-job"
        assert job.stage == "test"
        assert job.status == PipelineStatus.SUCCESS
        assert job.web_url == "https://gitlab.com/group/project/-/jobs/1"

    def test_defaults(self):
        job = _make_job()
        assert job.duration is None
        assert job.allow_failure is False

    def test_with_duration_and_allow_failure(self):
        job = _make_job(duration=123.5, allow_failure=True)
        assert job.duration == 123.5
        assert job.allow_failure is True

    def test_different_statuses(self):
        for status in PipelineStatus:
            job = _make_job(status=status)
            assert job.status == status


class TestPipelineDetailModel:
    def test_create_with_pipeline_and_jobs(self):
        pipeline = _make_pipeline()
        jobs = [
            _make_job(id=1, name="lint", stage="test"),
            _make_job(id=2, name="build", stage="build"),
        ]
        detail = PipelineDetail(pipeline=pipeline, jobs=jobs)
        assert detail.pipeline.id == 1
        assert len(detail.jobs) == 2

    def test_empty_jobs_default(self):
        pipeline = _make_pipeline()
        detail = PipelineDetail(pipeline=pipeline)
        assert detail.jobs == []


class TestPipelineJobStatusIcons:
    def test_all_statuses_have_icons(self):
        for status in PipelineStatus:
            assert status in PIPELINE_JOB_STATUS_ICONS, (
                f"PipelineStatus.{status.name} missing from PIPELINE_JOB_STATUS_ICONS"
            )

    def test_icons_are_nonempty_strings(self):
        for status, icon in PIPELINE_JOB_STATUS_ICONS.items():
            assert isinstance(icon, str)
            assert len(icon) > 0


class TestGroupJobsByStage:
    def test_groups_preserving_order(self):
        jobs = [
            _make_job(id=1, name="init:check", stage="init"),
            _make_job(id=2, name="init:ping", stage="init"),
            _make_job(id=3, name="build:go", stage="build"),
            _make_job(id=4, name="test:lint", stage="test"),
            _make_job(id=5, name="test:unit", stage="test"),
        ]
        stages = _group_jobs_by_stage(jobs)
        stage_names = list(stages.keys())
        assert stage_names == ["init", "build", "test"]
        assert len(stages["init"]) == 2
        assert len(stages["build"]) == 1
        assert len(stages["test"]) == 2

    def test_empty_jobs(self):
        stages = _group_jobs_by_stage([])
        assert stages == {}

    def test_single_stage(self):
        jobs = [
            _make_job(id=1, name="job-a", stage="deploy"),
            _make_job(id=2, name="job-b", stage="deploy"),
        ]
        stages = _group_jobs_by_stage(jobs)
        assert list(stages.keys()) == ["deploy"]
        assert len(stages["deploy"]) == 2


class TestFormatDuration:
    def test_none_duration(self):
        assert _format_duration(None) == ""

    def test_seconds_only(self):
        result = _format_duration(45.0)
        assert "45s" in result

    def test_minutes_and_seconds(self):
        result = _format_duration(125.0)
        assert "2m" in result
        assert "5s" in result

    def test_zero_duration(self):
        result = _format_duration(0.0)
        assert "0s" in result
