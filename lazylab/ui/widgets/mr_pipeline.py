import webbrowser

from rich.markup import escape
from rich.text import Text
from textual import on, work
from textual.app import ComposeResult
from textual.containers import HorizontalScroll, Vertical, VerticalScroll
from textual.content import Content
from textual.widgets import Static

import lazylab.lib.gitlab.pipelines as pipeline_api
from lazylab.lib.bindings import LazyLabBindings
from lazylab.lib.constants import PIPELINE_JOB_STATUS_ICONS, PipelineStatus
from lazylab.lib.context import LazyLabContext
from lazylab.lib.logging import ll
from lazylab.lib.messages import JobSelected
from lazylab.models.gitlab import PipelineDetail, PipelineJob


def _job_status_icon(status: PipelineStatus) -> str:
    return PIPELINE_JOB_STATUS_ICONS.get(status, f"[dim]{status}[/]")


def _format_duration(seconds: float | None) -> str:
    if seconds is None:
        return ""
    minutes, secs = divmod(int(seconds), 60)
    if minutes > 0:
        return f" [dim]{minutes}m {secs}s[/]"
    return f" [dim]{secs}s[/]"


def _group_jobs_by_stage(jobs: list[PipelineJob]) -> dict[str, list[PipelineJob]]:
    """Group jobs by stage, preserving stage order from API response."""
    stages: dict[str, list[PipelineJob]] = {}
    for job in jobs:
        stages.setdefault(job.stage, []).append(job)
    return stages


def _strip_ansi(text: str) -> Text:
    """Convert ANSI escape codes in CI logs to Rich Text."""
    return Text.from_ansi(text)


class PipelineJobWidget(Static):
    DEFAULT_CSS = """
    PipelineJobWidget {
        width: 100%;
        height: 1;
        padding: 0 1;
    }
    PipelineJobWidget:focus {
        background: $accent;
    }
    """

    can_focus = True

    BINDINGS = [
        LazyLabBindings.OPEN_IN_BROWSER,
        LazyLabBindings.SELECT_ENTRY,
    ]

    def __init__(self, job: PipelineJob) -> None:
        super().__init__(id=f"pipeline-job-{job.id}")
        self.job = job

    def render(self) -> Content:
        icon = _job_status_icon(self.job.status)
        duration = _format_duration(self.job.duration)
        name = escape(self.job.name)
        return Content.from_markup(f"{icon} {name}{duration}")

    def action_select_cursor(self) -> None:
        self.post_message(JobSelected(self.job))

    def action_open_in_browser(self) -> None:
        webbrowser.open(self.job.web_url)


class PipelineStageCard(Vertical):
    DEFAULT_CSS = """
    PipelineStageCard {
        width: auto;
        min-width: 28;
        max-width: 45;
        height: auto;
        border: solid $primary-lighten-3;
        margin: 0 1;
        padding: 1 0;
    }
    """

    def __init__(self, stage_name: str, jobs: list[PipelineJob]) -> None:
        super().__init__()
        self._stage_name = stage_name
        self._jobs = jobs

    def compose(self) -> ComposeResult:
        self.border_title = Content.from_markup(f"[bold]{escape(self._stage_name)}[/]")
        for job in self._jobs:
            yield PipelineJobWidget(job)


class PipelineStagesView(HorizontalScroll):
    DEFAULT_CSS = """
    PipelineStagesView {
        width: 100%;
        height: 1fr;
        padding: 1 0;
    }
    """

    def set_stages(self, stages: dict[str, list[PipelineJob]]) -> None:
        self.remove_children()
        for stage_name, jobs in stages.items():
            self.mount(PipelineStageCard(stage_name, jobs))


class JobLogView(VerticalScroll):
    DEFAULT_CSS = """
    JobLogView {
        width: 100%;
        height: 2fr;
        border-top: solid $primary-lighten-3;
        display: none;
    }
    JobLogView.visible {
        display: block;
    }
    JobLogView Static {
        width: 100%;
    }
    JobLogView .log-header {
        padding: 0 2;
        height: 1;
    }
    JobLogView .log-content {
        padding: 0 1;
    }
    """

    BINDINGS = [
        LazyLabBindings.CLOSE_LOG,
        LazyLabBindings.LOG_SCROLL_DOWN_LINE,
        LazyLabBindings.LOG_SCROLL_UP_LINE,
        LazyLabBindings.HALF_PAGE_DOWN,
        LazyLabBindings.HALF_PAGE_UP,
        LazyLabBindings.LOG_SCROLL_TOP,
        LazyLabBindings.LOG_SCROLL_BOTTOM,
    ]

    can_focus = True

    def __init__(self) -> None:
        super().__init__(id="job-log-view")
        self._log_header = Static(classes="log-header")
        self._log_content = Static(classes="log-content")

    def compose(self) -> ComposeResult:
        yield self._log_header
        yield self._log_content

    def show_log(self, job: PipelineJob, trace: str) -> None:
        icon = _job_status_icon(job.status)
        duration = _format_duration(job.duration)
        self._log_header.update(
            Content.from_markup(
                f"{icon} [bold]{escape(job.name)}[/]{duration}  "
                f"[dim]Press [bold]Esc[/] to close, [bold]o[/] to open in browser[/]"
            )
        )
        rich_text = _strip_ansi(trace)
        self._log_content.update(rich_text)
        self.add_class("visible")
        self.scroll_home(animate=False)
        self.focus()

    def show_loading(self, job_name: str) -> None:
        self._log_header.update(
            Content.from_markup(f"[dim]Loading log for {escape(job_name)}...[/]")
        )
        self._log_content.update("")
        self.add_class("visible")
        self.focus()

    def hide(self) -> None:
        self.remove_class("visible")

    def action_close_log(self) -> None:
        self.hide()
        # Restore focus to the previously focused job widget
        try:
            content = self.screen.query_one(MRPipelineTabContent)
            last = content._last_focused_job
            if last is not None and last.is_mounted:
                last.focus()
                return
        except Exception:
            pass
        # Fallback: focus first job
        try:
            stages = self.screen.query_one(PipelineStagesView)
            first_job = stages.query(PipelineJobWidget).first()
            first_job.focus()
        except Exception:
            pass

    def action_log_scroll_down_line(self) -> None:
        self.scroll_relative(y=1)

    def action_log_scroll_up_line(self) -> None:
        self.scroll_relative(y=-1)

    def action_half_page_down(self) -> None:
        self.scroll_relative(y=self.size.height // 2)

    def action_half_page_up(self) -> None:
        self.scroll_relative(y=-(self.size.height // 2))

    def action_log_scroll_top(self) -> None:
        self.scroll_home(animate=False)

    def action_log_scroll_bottom(self) -> None:
        self.scroll_end(animate=False)


class MRPipelineTabContent(Vertical):
    DEFAULT_CSS = """
    MRPipelineTabContent {
        width: 100%;
        height: 100%;
    }
    MRPipelineTabContent .pipeline-header {
        width: 100%;
        height: auto;
        padding: 1 2;
    }
    """

    BINDINGS = [
        LazyLabBindings.OPEN_IN_BROWSER,
        LazyLabBindings.PIPELINE_PREV_STAGE,
        LazyLabBindings.PIPELINE_NEXT_STAGE,
        LazyLabBindings.PIPELINE_PREV_JOB,
        LazyLabBindings.PIPELINE_NEXT_JOB,
    ]

    def __init__(self, **kwargs) -> None:
        super().__init__(**kwargs)
        self._pipeline_detail: PipelineDetail | None = None
        self._header = Static(id="pipeline-header", classes="pipeline-header")
        self._stages_view = PipelineStagesView(id="pipeline-stages-view")
        self._log_view = JobLogView()
        self._current_log_job: PipelineJob | None = None
        self._focused_job_pos: tuple[int, int] | None = None
        self._last_focused_job: PipelineJobWidget | None = None

    def compose(self) -> ComposeResult:
        yield self._header
        yield self._stages_view
        yield self._log_view

    @on(JobSelected)
    def on_job_selected(self, message: JobSelected) -> None:
        message.stop()
        self._current_log_job = message.job
        self._log_view.show_loading(message.job.name)
        self._load_job_trace(message.job)

    @work(exclusive=True, group="job-trace")
    async def _load_job_trace(self, job: PipelineJob) -> None:
        project = LazyLabContext.current_project
        if not project:
            return
        try:
            trace = await pipeline_api.get_job_trace(project.id, job.id)
            if not self.is_mounted:
                return
            self._log_view.show_log(job, trace)
        except Exception:
            ll.exception("Failed to load trace for job %s", job.name)
            if self.is_mounted:
                self._log_view.show_log(job, f"Error loading log for {job.name}")

    def load_pipeline(self, detail: PipelineDetail) -> None:
        self._pipeline_detail = detail
        pipeline = detail.pipeline
        icon = _job_status_icon(pipeline.status)
        self._header.update(
            Content.from_markup(
                f"Pipeline [bold]#{pipeline.id}[/] {icon}  "
                f"[dim]ref:[/] {escape(pipeline.ref)}  "
                f"[dim]sha:[/] {escape(pipeline.sha[:8])}"
            )
        )
        stages = _group_jobs_by_stage(detail.jobs)
        self._stages_view.set_stages(stages)

    def show_loading(self) -> None:
        self._header.update(Content.from_markup("[dim]Loading pipeline...[/]"))

    def show_error(self, msg: str) -> None:
        self._header.update(Content.from_markup(f"[red]{escape(msg)}[/]"))

    def show_empty(self) -> None:
        self._header.update(Content.from_markup("[dim]No pipeline found for this merge request[/]"))

    def _build_job_grid(self) -> list[list[PipelineJobWidget]]:
        """Build a grid of job widgets indexed by [stage][job]."""
        return [
            list(card.query(PipelineJobWidget))
            for card in self._stages_view.query(PipelineStageCard)
        ]

    def _update_focused_pos(self, widget: PipelineJobWidget) -> None:
        """Update cached position when a job widget receives focus."""
        grid = self._build_job_grid()
        for si, jobs in enumerate(grid):
            for ji, job_widget in enumerate(jobs):
                if job_widget is widget:
                    self._focused_job_pos = (si, ji)
                    return

    def on_descendant_focus(self, event) -> None:
        if isinstance(event.widget, PipelineJobWidget):
            self._last_focused_job = event.widget
            self._update_focused_pos(event.widget)

    def _get_focused_job_position(self) -> tuple[int, int] | None:
        """Return cached (stage_index, job_index) of the focused PipelineJobWidget."""
        focused = self.screen.focused
        if not isinstance(focused, PipelineJobWidget):
            return None
        return self._focused_job_pos

    def _focus_job_at(self, stage_idx: int, job_idx: int) -> None:
        """Focus the job widget at the given stage and job indices (clamped)."""
        grid = self._build_job_grid()
        if not grid:
            return
        stage_idx = max(0, min(stage_idx, len(grid) - 1))
        jobs = grid[stage_idx]
        if not jobs:
            return
        job_idx = max(0, min(job_idx, len(jobs) - 1))
        jobs[job_idx].focus()

    def action_prev_stage(self) -> None:
        pos = self._get_focused_job_position()
        if pos is None:
            self._focus_job_at(0, 0)
        else:
            self._focus_job_at(pos[0] - 1, pos[1])

    def action_next_stage(self) -> None:
        pos = self._get_focused_job_position()
        if pos is None:
            self._focus_job_at(0, 0)
        else:
            self._focus_job_at(pos[0] + 1, pos[1])

    def action_prev_job(self) -> None:
        pos = self._get_focused_job_position()
        if pos is None:
            self._focus_job_at(0, 0)
        else:
            self._focus_job_at(pos[0], pos[1] - 1)

    def action_next_job(self) -> None:
        pos = self._get_focused_job_position()
        if pos is None:
            self._focus_job_at(0, 0)
        else:
            self._focus_job_at(pos[0], pos[1] + 1)

    def action_open_in_browser(self) -> None:
        # Focused PipelineJobWidget handles its own binding; this is the fallback
        if self._pipeline_detail:
            webbrowser.open(self._pipeline_detail.pipeline.web_url)
