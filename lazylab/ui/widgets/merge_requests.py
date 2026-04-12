from textual import on, work
from textual.app import ComposeResult
from textual.containers import VerticalScroll
from textual.content import Content
from textual.coordinate import Coordinate
from textual.css.query import NoMatches
from textual.reactive import reactive
from textual.widgets import DataTable, Label, Static, TabPane

import lazylab.lib.gitlab.merge_requests as mr_api
import lazylab.lib.gitlab.pipelines as pipeline_api
from lazylab.lib.bindings import LazyLabBindings
from lazylab.lib.cache import api_cache
from lazylab.lib.constants import MROwnerFilter, MRState, PipelineStatus
from lazylab.lib.context import LazyLabContext
from lazylab.lib.logging import ll
from lazylab.lib.messages import MRActionCompleted, MRSelected
from lazylab.models.gitlab import ApprovalStatus, MergeRequest, Project
from lazylab.ui.screens.mr_actions import CloseMRScreen, MergeMRScreen, MergeResult
from lazylab.ui.widgets.common import LazyLabContainer, SearchableDataTable, TableRow
from lazylab.ui.widgets.mr_diff import MRDiffTabContent
from lazylab.ui.widgets.mr_pipeline import MRPipelineTabContent


def _mr_status_icon(state: MRState) -> str:
    match state:
        case MRState.OPENED:
            return "[green]O[/]"
        case MRState.MERGED:
            return "[magenta]M[/]"
        case MRState.CLOSED:
            return "[red]C[/]"
        case _:
            return "?"


def mr_to_row(mr: MergeRequest) -> TableRow:
    return (
        f"!{mr.iid}",
        _mr_status_icon(mr.state),
        mr.author.username,
        mr.title,
    )


class MRContainer(LazyLabContainer):
    def __init__(self, *args, **kwargs) -> None:
        super().__init__(*args, **kwargs)
        self.iid_column_index = 0
        self._current_project: Project | None = None
        self._pending_action_mr: MergeRequest | None = None

        self._table: SearchableDataTable[MergeRequest] = SearchableDataTable(
            id="searchable_mrs_table",
            table_id="mrs_table",
            search_input_id="mr_search",
            sort_key="iid",
            reverse_sort=True,
            item_to_key=lambda mr: f"{mr.project_path}!{mr.iid}",
            item_to_row=mr_to_row,
            cache_name="merge_requests",
            project_based_cache=True,
        )

    def compose(self) -> ComposeResult:
        self.border_title = Content.from_markup("Merge Requests")
        yield self._table

    @property
    def searchable_table(self) -> SearchableDataTable[MergeRequest]:
        return self._table

    @property
    def table(self) -> DataTable:
        return self.searchable_table.table

    async def on_mount(self) -> None:
        self.table.cursor_type = "row"
        self.table.add_column("#", key="iid")
        self.table.add_column("St", key="status")
        self.table.add_column("Author", key="author")
        self.table.add_column("Title", key="title")

    async def get_selected_mr(self) -> MergeRequest | None:
        if self.table.row_count == 0 or self._current_project is None:
            return None
        current_row = self.table.cursor_row
        iid_cell = str(self.table.get_cell_at(Coordinate(current_row, self.iid_column_index)))
        iid = int(iid_cell.replace("!", ""))
        key = f"{self._current_project.path_with_namespace}!{iid}"
        return self.searchable_table.items.get(key)

    def set_project(self, project: Project) -> None:
        """Called by parent when a project is selected."""
        self._current_project = project
        self.searchable_table.loading = True
        self.load_merge_requests(project)

    @work
    async def load_merge_requests(self, project: Project) -> None:
        self.searchable_table.initialize_from_cache(project.path_with_namespace, MergeRequest)
        try:
            config = LazyLabContext.config.merge_requests
            state = config.state_filter.value

            author_id = None
            reviewer_id = None
            if config.owner_filter in (MROwnerFilter.MINE, MROwnerFilter.REVIEWER):
                user = await LazyLabContext.client.get_current_user()
                if config.owner_filter == MROwnerFilter.MINE:
                    author_id = user.id
                else:
                    reviewer_id = user.id

            mrs = await mr_api.list_merge_requests(
                project_id=project.id,
                project_path=project.path_with_namespace,
                state=state,
                author_id=author_id,
                reviewer_id=reviewer_id,
            )
        except Exception:
            ll.exception("Error fetching MRs from GitLab API")
            self.searchable_table.loading = False
            return

        self.searchable_table.clear_rows()
        self.searchable_table.add_items(mrs)
        self.searchable_table.apply_current_filter()
        self.searchable_table.loading = False

    @on(DataTable.RowSelected, "#mrs_table")
    async def mr_selected(self) -> None:
        mr = await self.get_selected_mr()
        if mr is not None:
            self.post_message(MRSelected(mr))

    async def action_close_mr(self) -> None:
        mr = await self.get_selected_mr()
        if mr is None:
            return
        if mr.state != MRState.OPENED:
            self.app.notify(f"Cannot close: MR !{mr.iid} is {mr.state.value}", severity="warning")
            return
        self._pending_action_mr = mr
        self.app.push_screen(CloseMRScreen(mr), callback=self._on_close_confirmed)

    def _on_close_confirmed(self, confirmed: bool | None) -> None:
        if not confirmed or self._pending_action_mr is None:
            self._pending_action_mr = None
            return
        self._execute_close(self._pending_action_mr)
        self._pending_action_mr = None

    @work
    async def _execute_close(self, mr: MergeRequest) -> None:
        project = self._current_project
        if project is None:
            return
        try:
            updated_mr = await mr_api.close_merge_request(project.id, mr.iid, project.path_with_namespace)
            self.app.notify(f"Closed MR !{mr.iid}: {mr.title}")
            self.post_message(MRActionCompleted(updated_mr))
            self.load_merge_requests(project)
        except Exception:
            ll.exception(f"Failed to close MR !{mr.iid}")
            self.app.notify(f"Failed to close MR !{mr.iid}", severity="error")

    async def action_merge_mr(self) -> None:
        mr = await self.get_selected_mr()
        if mr is None:
            return
        if mr.state != MRState.OPENED:
            self.app.notify(f"Cannot merge: MR !{mr.iid} is {mr.state.value}", severity="warning")
            return
        self._pending_action_mr = mr
        self.app.push_screen(MergeMRScreen(mr), callback=self._on_merge_confirmed)

    def _on_merge_confirmed(self, result: MergeResult | None) -> None:
        if result is None or self._pending_action_mr is None:
            self._pending_action_mr = None
            return
        self._execute_merge(self._pending_action_mr, result)
        self._pending_action_mr = None

    @work
    async def _execute_merge(self, mr: MergeRequest, result: MergeResult) -> None:
        project = self._current_project
        if project is None:
            return
        try:
            updated_mr = await mr_api.merge_merge_request(
                project.id,
                mr.iid,
                project.path_with_namespace,
                should_remove_source_branch=result.should_remove_source_branch,
                merge_when_pipeline_succeeds=result.merge_when_pipeline_succeeds,
            )
            self.app.notify(f"Merged MR !{mr.iid}: {mr.title}")
            self.post_message(MRActionCompleted(updated_mr))
            self.load_merge_requests(project)
        except Exception:
            ll.exception(f"Failed to merge MR !{mr.iid}")
            self.app.notify(f"Failed to merge MR !{mr.iid}", severity="error")


def _pipeline_status_icon(status: PipelineStatus) -> str:
    match status:
        case PipelineStatus.SUCCESS:
            return "[green]\u2713 passed[/]"
        case PipelineStatus.FAILED:
            return "[red]\u2718 failed[/]"
        case PipelineStatus.RUNNING:
            return "[yellow]\u25b6 running[/]"
        case PipelineStatus.PENDING | PipelineStatus.CREATED | PipelineStatus.WAITING_FOR_RESOURCE | PipelineStatus.PREPARING:
            return "[yellow]\u25cb pending[/]"
        case PipelineStatus.CANCELED:
            return "[dim]\u2718 canceled[/]"
        case PipelineStatus.SKIPPED:
            return "[dim]- skipped[/]"
        case PipelineStatus.MANUAL | PipelineStatus.SCHEDULED:
            return "[cyan]\u25a0 manual[/]"
        case _:
            return f"[dim]{status}[/]"


def _conflict_text(has_conflicts: bool) -> str:
    if has_conflicts:
        return "[red]Has conflicts[/]"
    return "[green]No conflicts[/]"


def _approval_text(approval: ApprovalStatus | None) -> str:
    if approval is None:
        return "[dim]Loading...[/]"
    approved_names = ", ".join(u.username for u in approval.approved_by)
    if approval.approved:
        return f"[green]\u2713 Approved[/] ({approved_names})"
    remaining = approval.approvals_left
    total = approval.approvals_required
    approved_part = f" ({approved_names})" if approved_names else ""
    return f"[yellow]{total - remaining}/{total} approvals{approved_part}[/]"


class MROverviewTabPane(TabPane):
    DEFAULT_CSS = """
    MROverviewTabPane {
        padding: 1 2;
    }
    MROverviewTabPane .overview-label {
        margin-bottom: 1;
    }
    MROverviewTabPane .overview-section {
        margin-bottom: 1;
        padding: 0 1;
    }
    """

    approval_text: reactive[str] = reactive("[dim]Loading...[/]")
    pipeline_text: reactive[str] = reactive("[dim]Loading...[/]")

    def __init__(self, mr: MergeRequest) -> None:
        super().__init__("Overview", id="mr-overview-tab")
        self.mr = mr

    def compose(self) -> ComposeResult:
        with VerticalScroll():
            yield Static(Content.from_markup(f"[bold]{self.mr.title}[/]"), classes="overview-label")
            yield Label(f"Author: @{self.mr.author.username}")
            yield Label(f"Created: {self.mr.created_at.strftime('%Y-%m-%d %H:%M')}")
            yield Static(Content.from_markup(f"Status: {_mr_status_icon(self.mr.state)} {self.mr.state.value}"))
            yield Label(f"Branches: {self.mr.source_branch} \u2192 {self.mr.target_branch}")
            yield Label("")
            yield Static(Content.from_markup(f"Conflicts: {_conflict_text(self.mr.has_conflicts)}"))
            yield Label(f"Comments: {self.mr.user_notes_count}")
            yield Label("")
            yield Static(id="approval-status", classes="overview-section")
            yield Static(id="pipeline-status", classes="overview-section")

    def _update_status_label(self, element_id: str, label: str, value: str) -> None:
        try:
            self.query_one(f"#{element_id}", Static).update(Content.from_markup(f"{label}: {value}"))
        except NoMatches:
            pass

    def on_mount(self) -> None:
        self._update_status_label("approval-status", "Approvals", self.approval_text)
        self._update_status_label("pipeline-status", "Pipeline", self.pipeline_text)
        self._load_approval()
        self._load_pipeline()

    def watch_approval_text(self, value: str) -> None:
        self._update_status_label("approval-status", "Approvals", value)

    def watch_pipeline_text(self, value: str) -> None:
        self._update_status_label("pipeline-status", "Pipeline", value)

    def _matches_current_mr(self, key: str) -> bool:
        project = LazyLabContext.current_project
        if not project:
            return False
        return f"{project.id}:{self.mr.iid}" in key

    def handle_cache_refresh(self, namespace: str, key: str) -> None:
        if not self.is_mounted:
            return
        if namespace == "mr_approvals" and self._matches_current_mr(key):
            self._load_approval()
        elif namespace == "pipeline_latest" and self._matches_current_mr(key):
            self._load_pipeline()

    @work
    async def _load_approval(self) -> None:
        project = LazyLabContext.current_project
        if not project:
            return
        try:
            approval = await mr_api.get_mr_approvals(project.id, self.mr.iid)
            self.approval_text = _approval_text(approval)
        except Exception:
            ll.exception("Failed to load MR approvals")
            self.approval_text = "[red]Error loading approvals[/]"

    @work
    async def _load_pipeline(self) -> None:
        project = LazyLabContext.current_project
        if not project:
            return
        try:
            pipeline = await pipeline_api.get_latest_pipeline_for_mr(project.id, self.mr.iid)
            if pipeline is None:
                self.pipeline_text = "[dim]No pipeline[/]"
            else:
                self.pipeline_text = _pipeline_status_icon(pipeline.status)
        except Exception:
            ll.exception("Failed to load pipeline status")
            self.pipeline_text = "[red]Error loading pipeline[/]"


class MRDiffTabPane(TabPane):
    DEFAULT_CSS = """
    MRDiffTabPane {
        padding: 0;
        height: 1fr;
    }
    """

    def __init__(self, mr: MergeRequest) -> None:
        super().__init__("Diff", id="mr-diff-tab")
        self.mr = mr

    def compose(self) -> ComposeResult:
        yield MRDiffTabContent(id="mr-diff-content")

    @property
    def diff_content(self) -> MRDiffTabContent:
        return self.query_one("#mr-diff-content", MRDiffTabContent)

    def on_mount(self) -> None:
        self.diff_content.show_loading()
        self._load_diff()

    @work
    async def _load_diff(self) -> None:
        project = LazyLabContext.current_project
        if not project:
            return
        try:
            diff_data = await mr_api.get_mr_changes(project.id, self.mr.iid)
            self.diff_content.load_diff(diff_data)
        except Exception:
            ll.exception("Failed to load MR diff")
            self.diff_content.show_error("Error loading diff")


class MRConversationTabPane(TabPane):
    """Placeholder for Phase 6."""

    def __init__(self, mr: MergeRequest) -> None:
        super().__init__("Conversation", id="mr-conversation-tab")
        self.mr = mr

    def compose(self) -> ComposeResult:
        yield Label("Conversation will be implemented in Phase 6")


class MRPipelineTabPane(TabPane):
    DEFAULT_CSS = """
    MRPipelineTabPane {
        padding: 0;
        height: 1fr;
    }
    """

    BINDINGS = [LazyLabBindings.FORCE_REFRESH]

    def __init__(self, mr: MergeRequest) -> None:
        super().__init__("Pipeline", id="mr-pipeline-tab")
        self.mr = mr

    def compose(self) -> ComposeResult:
        yield MRPipelineTabContent(id="mr-pipeline-content")

    @property
    def pipeline_content(self) -> MRPipelineTabContent:
        return self.query_one("#mr-pipeline-content", MRPipelineTabContent)

    def on_mount(self) -> None:
        self.pipeline_content.show_loading()
        self._load_pipeline()

    def handle_cache_refresh(self, namespace: str, key: str) -> None:
        if not self.is_mounted:
            return
        project = LazyLabContext.current_project
        if not project:
            return
        mr_prefix = f"{project.id}:{self.mr.iid}"
        if namespace == "pipeline_detail" and mr_prefix in key:
            self._load_pipeline()

    def action_force_refresh(self) -> None:
        project = LazyLabContext.current_project
        if not project:
            return
        api_cache.invalidate(f"pipeline_detail:{project.id}:{self.mr.iid}")
        api_cache.invalidate(f"pipeline_latest:{project.id}:{self.mr.iid}")
        self.pipeline_content.show_loading()
        self._load_pipeline()
        self.app.notify("Pipeline refreshed")

    @work(exclusive=True)
    async def _load_pipeline(self) -> None:
        project = LazyLabContext.current_project
        if not project:
            return
        try:
            detail = await pipeline_api.get_pipeline_detail(project.id, self.mr.iid)
            if not self.is_mounted:
                return
            if detail is None:
                self.pipeline_content.show_empty()
            else:
                self.pipeline_content.load_pipeline(detail)
        except Exception:
            ll.exception("Failed to load pipeline details")
            if self.is_mounted:
                self.pipeline_content.show_error("Error loading pipeline")
