from typing import Callable

from textual import on, work
from textual.app import ComposeResult
from textual.content import Content
from textual.coordinate import Coordinate
from textual.widgets import DataTable

import lazylab.lib.gitlab.projects as projects_api
from lazylab.lib.bindings import LazyLabBindings
from lazylab.lib.constants import IS_FAVORITED, favorite_string
from lazylab.lib.context import LazyLabContext
from lazylab.lib.logging import ll
from lazylab.lib.messages import RepoSelected
from lazylab.models.gitlab import Project
from lazylab.ui.widgets.common import LazyLabContainer, SearchableDataTable, TableRow


def _make_project_to_row() -> Callable[[Project], TableRow]:
    """Build a project_to_row closure with a frozenset for O(1) favorites lookup."""
    favorites = frozenset(LazyLabContext.config.repositories.favorites)

    def project_to_row(project: Project) -> TableRow:
        favorited = favorite_string(project.path_with_namespace in favorites)
        return (favorited, project.path_with_namespace)

    return project_to_row


class ReposContainer(LazyLabContainer):
    BINDINGS = [LazyLabBindings.TOGGLE_FAVORITE]

    def __init__(self, *args, **kwargs) -> None:
        super().__init__(*args, **kwargs)
        self.favorite_column_index = 0
        self.name_column_index = 1

        self._table: SearchableDataTable[Project] = SearchableDataTable(
            id="searchable_repos_table",
            table_id="repos_table",
            search_input_id="repo_search",
            sort_key="favorite",
            item_to_key=lambda p: p.path_with_namespace,
            item_to_row=_make_project_to_row(),
            cache_name="projects",
            project_based_cache=False,
        )

    def compose(self) -> ComposeResult:
        self.border_title = Content.from_markup("Repositories")
        yield self._table

    @property
    def searchable_table(self) -> SearchableDataTable[Project]:
        return self._table

    @property
    def table(self) -> DataTable:
        return self.searchable_table.table

    async def on_mount(self) -> None:
        self.table.cursor_type = "row"
        self.table.add_column(IS_FAVORITED, key="favorite")
        self.table.add_column("Name", key="name")

        self.searchable_table.loading = True
        self.load_projects()

    async def get_selected_project(self) -> Project | None:
        if self.table.row_count == 0:
            return None
        current_row = self.table.cursor_row
        name = str(self.table.get_cell_at(Coordinate(current_row, self.name_column_index)))
        return self.searchable_table.items.get(name)

    def set_projects(self, projects: list[Project]) -> None:
        self.searchable_table.clear_rows()
        self.searchable_table.add_items(projects)

    @work
    async def load_projects(self) -> None:
        self.searchable_table.initialize_from_cache(None, Project)
        try:
            projects = await projects_api.list_projects()
        except Exception:
            ll.exception("Error fetching projects from GitLab API")
            self.searchable_table.loading = False
            return

        self.set_projects(projects)
        self.searchable_table.loading = False
        self.table.focus()

    async def action_toggle_favorite(self) -> None:
        project = await self.get_selected_project()
        if project is None:
            return
        path = project.path_with_namespace
        with LazyLabContext.config.to_edit() as config:
            if path in config.repositories.favorites:
                ll.info(f"Unfavoriting {path}")
                config.repositories.favorites.remove(path)
            else:
                ll.info(f"Favoriting {path}")
                config.repositories.favorites.append(path)

        updated = path in LazyLabContext.config.repositories.favorites
        self.table.update_cell(path, "favorite", favorite_string(updated))
        self._table.item_to_row = _make_project_to_row()
        self.searchable_table.sort_table()

    @on(DataTable.RowSelected, "#repos_table")
    async def repo_selected(self) -> None:
        project = await self.get_selected_project()
        if project is not None:
            self.post_message(RepoSelected(project))
