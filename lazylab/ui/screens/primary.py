from textual import on
from textual.app import ComposeResult
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
from lazylab.lib.messages import MRSelected, RepoSelected
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
        LazyLabBindings.PREV_TAB,
        LazyLabBindings.NEXT_TAB,
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
        self.border_title = Content.from_markup("\\[3] Details")
        yield self.tabs

    def on_mount(self) -> None:
        self.tabs.add_pane(LazyLabInfoTabPane())

    def action_previous_tab(self) -> None:
        self.tabs.action_previous_tab()  # type: ignore[attr-defined]

    def action_next_tab(self) -> None:
        self.tabs.action_next_tab()  # type: ignore[attr-defined]


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
        LazyLabBindings.FOCUS_REPOS,
        LazyLabBindings.FOCUS_MRS,
        LazyLabBindings.FOCUS_DETAILS,
    ]

    def action_focus_section(self, selector: str) -> None:
        try:
            target = self.query_one(selector)
            if target.visible:
                target.focus()
        except NoMatches:
            pass

    def action_focus_tabs(self) -> None:
        tabbed_content = self.query_one("#selection_detail_tabs", TabbedContent)
        if tabbed_content.children and tabbed_content.tab_count > 0:
            tabbed_content.children[0].focus()

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
        self.details.border_title = Content.from_markup(f"\\[3] MR !{mr.iid} Details")
        if tabbed_content.children:
            tabbed_content.children[0].focus()


class LazyLabMainScreen(Screen):
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

    @on(MRSelected)
    async def handle_mr_selection(self, message: MRSelected) -> None:
        await self.main_view_pane.load_mr_details(message.mr)
