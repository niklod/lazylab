from textual import on
from textual.app import ComposeResult
from textual.binding import Binding
from textual.containers import Container, Horizontal
from textual.content import Content
from textual.css.query import NoMatches
from textual.reactive import reactive
from textual.screen import Screen
from textual.widget import Widget
from textual.widgets import TabbedContent

from lazylab.lib.bindings import LazyLabBindings
from lazylab.lib.context import LazyLabContext
from lazylab.lib.logging import ll
from lazylab.lib.messages import MRActionCompleted, MRSelected, RepoSelected
from lazylab.models.gitlab import MergeRequest, Project
from lazylab.ui.widgets.common import LazyLabContainer, LazyLabFooter
from lazylab.ui.widgets.info import LazyLabInfoTabPane
from lazylab.ui.widgets.merge_requests import (
    MRContainer,
    MRConversationTabPane,
    MRDiffTabPane,
    MROverviewTabPane,
    MRPipelineTabPane,
)
from lazylab.ui.widgets.repositories import ReposContainer


class CurrentlySelectedProject(Widget):
    current_project_name: reactive[str | None] = reactive(None)

    DEFAULT_CSS = """
    CurrentlySelectedProject {
        max-width: 100%;
        height: 100%;
        content-align: left middle;
    }
    """

    def render(self):
        if self.current_project_name:
            return Content.from_markup(f"Current project: [greenyellow]{self.current_project_name}[/]")
        return "No project selected"


class LazyLabStatusBar(Container):
    DEFAULT_CSS = """
    LazyLabStatusBar {
        max-height: 3;
        width: 100%;
        layout: horizontal;
        border: solid $secondary;
    }
    """

    def compose(self) -> ComposeResult:
        with Horizontal():
            yield CurrentlySelectedProject(id="current_project")


class SelectionDetailsContainer(LazyLabContainer):
    BINDINGS = [
        Binding("[", "previous_tab", "Prev Tab", show=False, priority=True),
        Binding("]", "next_tab", "Next Tab", show=False, priority=True),
    ]

    DEFAULT_CSS = """
    SelectionDetailsContainer {
        height: 1fr;
    }
    """

    def __init__(self, *args, **kwargs) -> None:
        super().__init__(*args, **kwargs)
        self.tabs = TabbedContent(id="selection_detail_tabs")

    def compose(self) -> ComposeResult:
        self.border_title = Content.from_markup("Details")
        yield self.tabs

    def on_mount(self) -> None:
        self.tabs.add_pane(LazyLabInfoTabPane())

    def _get_pane_ids(self) -> list[str]:
        return [pane.id or "" for pane in self.tabs.query("TabPane") if pane.id]

    def _switch_tab(self, direction: int) -> None:
        pane_ids = self._get_pane_ids()
        if not pane_ids:
            return
        current = self.tabs.active
        if current in pane_ids:
            idx = (pane_ids.index(current) + direction) % len(pane_ids)
        else:
            idx = 0
        target = pane_ids[idx]
        self.tabs.active = target
        self._focus_active_pane(target)

    def _focus_active_pane(self, pane_id: str) -> None:
        try:
            pane = self.tabs.query_one(f"#{pane_id}")
            for widget in pane.query("*"):
                if widget.focusable:
                    widget.focus()
                    return
        except Exception:
            pass
        # Fallback: focus the tab strip so priority bindings keep working
        from textual.widgets import Tabs

        try:
            self.tabs.query_one(Tabs).focus()
        except Exception:
            pass

    def action_previous_tab(self) -> None:
        self._switch_tab(-1)

    def action_next_tab(self) -> None:
        self._switch_tab(1)


class SelectionsPane(Container):
    DEFAULT_CSS = """
    SelectionsPane {
        height: 100%;
        width: 40%;
        dock: left;
    }
    """

    def compose(self) -> ComposeResult:
        yield ReposContainer(id="repos")
        yield MRContainer(id="merge_requests")

    @property
    def repositories(self) -> ReposContainer:
        return self.query_one("#repos", ReposContainer)

    @property
    def merge_requests(self) -> MRContainer:
        return self.query_one("#merge_requests", MRContainer)


class SelectionDetailsPane(Container):
    def compose(self) -> ComposeResult:
        yield SelectionDetailsContainer(id="selection_details")


class MainViewPane(Container):
    BINDINGS = [
        LazyLabBindings.FOCUS_PREV,
        LazyLabBindings.FOCUS_NEXT,
    ]

    SECTIONS = ("#repos_table", "#mrs_table", "#selection_detail_tabs")

    def _current_section_index(self) -> int:
        """Return index of the section that currently contains focus, or -1."""
        focused = self.screen.focused
        if focused is None:
            return -1
        for i, selector in enumerate(self.SECTIONS):
            try:
                section = self.query_one(selector)
                if focused is section or focused in section.query("*"):
                    return i
            except NoMatches:
                continue
        return -1

    def _focus_section(self, index: int) -> None:
        selector = self.SECTIONS[index]
        if selector == "#selection_detail_tabs":
            self._focus_tabs()
        else:
            try:
                target = self.query_one(selector)
                if target.visible:
                    target.focus()
            except NoMatches:
                pass

    def _focus_tabs(self) -> None:
        tabbed_content = self.query_one("#selection_detail_tabs", TabbedContent)
        if tabbed_content.tab_count == 0:
            return
        active_id = tabbed_content.active
        if active_id:
            try:
                pane = tabbed_content.query_one(f"#{active_id}")
                first_focusable = pane.query("*:focusable").first()
                first_focusable.focus()
                return
            except Exception:
                pass
        tabbed_content.focus()

    def action_focus_prev(self) -> None:
        idx = self._current_section_index()
        new_idx = (idx - 1) % len(self.SECTIONS)
        self._focus_section(new_idx)

    def action_focus_next(self) -> None:
        idx = self._current_section_index()
        new_idx = (idx + 1) % len(self.SECTIONS)
        self._focus_section(new_idx)

    def compose(self) -> ComposeResult:
        yield SelectionsPane(id="selections_pane")
        yield SelectionDetailsPane(id="details_pane")

    @property
    def selections(self) -> SelectionsPane:
        return self.query_one("#selections_pane", SelectionsPane)

    @property
    def details(self) -> SelectionDetailsContainer:
        return self.query_one("#selection_details", SelectionDetailsContainer)

    async def load_mr_details(self, mr: MergeRequest) -> None:
        """Load MR detail tabs with Overview, Diff, Conversation, Pipeline."""
        tabbed_content = self.query_one("#selection_detail_tabs", TabbedContent)
        await tabbed_content.clear_panes()
        await tabbed_content.add_pane(MROverviewTabPane(mr))
        await tabbed_content.add_pane(MRDiffTabPane(mr))
        await tabbed_content.add_pane(MRConversationTabPane(mr))
        await tabbed_content.add_pane(MRPipelineTabPane(mr))
        self.details.border_title = Content.from_markup(f"MR !{mr.iid} Details")
        if tabbed_content.children:
            tabbed_content.children[0].focus()


class LazyLabMainScreen(Screen):
    BINDINGS = [
        LazyLabBindings.CLOSE_MR,
        LazyLabBindings.MERGE_MR,
    ]

    def compose(self) -> ComposeResult:
        with Container():
            yield LazyLabStatusBar()
            yield MainViewPane(id="main-view-pane")
            yield LazyLabFooter()

    @property
    def main_view_pane(self) -> MainViewPane:
        return self.query_one("#main-view-pane", MainViewPane)

    def set_currently_loaded_project(self, project: Project) -> None:
        ll.info(f"Selected project {project.path_with_namespace}")
        LazyLabContext.current_project = project
        widget = self.query_one("#current_project", CurrentlySelectedProject)
        widget.current_project_name = project.path_with_namespace

    @on(RepoSelected)
    async def handle_repo_selection(self, message: RepoSelected) -> None:
        self.set_currently_loaded_project(message.project)
        self.main_view_pane.selections.merge_requests.set_project(message.project)
        try:
            mrs_table = self.query_one("#mrs_table")
            mrs_table.focus()
        except NoMatches:
            pass

    @on(MRSelected)
    async def handle_mr_selection(self, message: MRSelected) -> None:
        await self.main_view_pane.load_mr_details(message.mr)

    @on(MRActionCompleted)
    async def handle_mr_action_completed(self, message: MRActionCompleted) -> None:
        await self.main_view_pane.load_mr_details(message.mr)

    def handle_cache_refresh(self, namespace: str, key: str) -> None:
        """Dispatch a cache refresh notification to the relevant active widget."""
        try:
            overview = self.query_one(MROverviewTabPane)
            overview.handle_cache_refresh(namespace, key)
        except NoMatches:
            pass
        try:
            pipeline_tab = self.query_one(MRPipelineTabPane)
            pipeline_tab.handle_cache_refresh(namespace, key)
        except NoMatches:
            pass

    async def action_close_mr(self) -> None:
        await self.main_view_pane.selections.merge_requests.action_close_mr()

    async def action_merge_mr(self) -> None:
        await self.main_view_pane.selections.merge_requests.action_merge_mr()
