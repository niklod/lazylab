from datetime import datetime
from typing import Callable, Generic, TypeVar

from pydantic import BaseModel
from textual import on
from textual.app import ComposeResult
from textual.containers import Container, Horizontal, Vertical
from textual.events import Blur
from textual.widgets import Button, DataTable, Footer, Input
from textual.widgets.data_table import RowDoesNotExist

from lazylab.lib.bindings import LazyLabBindings
from lazylab.lib.cache import load_models_from_cache, save_models_to_cache
from lazylab.lib.context import LazyLabContext
from lazylab.lib.logging import ll

T = TypeVar("T", bound=BaseModel)
TableCellType = str | int | datetime
TableRow = tuple[TableCellType, ...]


class LazyLabFooter(Footer):
    def __init__(self) -> None:
        super().__init__(show_command_palette=False)


class VimLikeDataTable(DataTable[TableCellType]):
    """DataTable with vim-like keybindings."""

    BINDINGS = [
        LazyLabBindings.SELECT_ENTRY,
        LazyLabBindings.TABLE_DOWN,
        LazyLabBindings.TABLE_PAGE_DOWN,
        LazyLabBindings.TABLE_CURSOR_UP,
        LazyLabBindings.TABLE_PAGE_UP,
        LazyLabBindings.TABLE_PAGE_RIGHT,
        LazyLabBindings.TABLE_PAGE_LEFT,
        LazyLabBindings.TABLE_SCROLL_TOP,
        LazyLabBindings.TABLE_SCROLL_BOTTOM,
    ]


class ToggleableSearchInput(Input):
    def _on_blur(self, event: Blur) -> None:
        if not self.value.strip():
            self.can_focus = False
            self.display = False
        return super()._on_blur(event)


class SearchableDataTable(Vertical, Generic[T]):
    BINDINGS = [LazyLabBindings.SEARCH_TABLE]

    DEFAULT_CSS = """
    ToggleableSearchInput {
        margin-bottom: 1;
    }
    """

    def __init__(
        self,
        table_id: str,
        search_input_id: str,
        sort_key: str,
        item_to_row: Callable[[T], TableRow],
        item_to_key: Callable[[T], str],
        *args,
        reverse_sort: bool = False,
        cache_name: str | None = None,
        project_based_cache: bool = True,
        **kwargs,
    ) -> None:
        super().__init__(*args, **kwargs)
        self.table = VimLikeDataTable(id=table_id)
        self.search_input = ToggleableSearchInput(placeholder="Search...", id=search_input_id)
        self.search_input.display = False
        self.search_input.can_focus = False
        self.sort_key = sort_key
        self.reverse_sort = reverse_sort
        self.item_to_row = item_to_row
        self.item_to_key = item_to_key
        self.cache_name = cache_name
        self.project_based_cache = project_based_cache
        self.items: dict[str, T] = {}
        self._all_items: dict[str, T] = {}

    def compose(self) -> ComposeResult:
        yield self.search_input
        yield self.table

    def sort_table(self) -> None:
        self.table.sort(self.sort_key, reverse=self.reverse_sort)

    async def action_focus_search(self) -> None:
        self.search_input.can_focus = True
        self.search_input.display = True
        self.search_input.focus()

    def clear_rows(self) -> None:
        self.items = {}
        self._all_items = {}
        self.table.clear()

    def initialize_from_cache(self, project_path: str | None, expect_type: type[T]) -> None:
        self.clear_rows()
        if not self.cache_name:
            return

        cache_dir = LazyLabContext.config.cache.directory
        cache_project = project_path if self.project_based_cache else None
        ll.debug(f"Loading '{expect_type.__name__}' from '{self.cache_name}' cache")
        cached_models = load_models_from_cache(cache_dir, cache_project, self.cache_name, expect_type)
        self.add_items(cached_models, write_to_cache=False)

    def save_to_cache(self) -> None:
        if not self.cache_name:
            return

        cache_dir = LazyLabContext.config.cache.directory
        project_path = None
        if self.project_based_cache and LazyLabContext.current_project:
            project_path = LazyLabContext.current_project.path_with_namespace
        save_models_to_cache(cache_dir, project_path, self.cache_name, self._all_items.values())

    def _add_item_no_sort(self, item: T) -> None:
        item_key = self.item_to_key(item)
        try:
            if item_key in self.items:
                self.table.remove_row(item_key)
        except RowDoesNotExist:
            pass

        self.items[item_key] = item
        self._all_items[item_key] = item
        self.table.add_row(*self.item_to_row(item), key=item_key)

    def add_item(self, item: T, write_to_cache: bool = True) -> None:
        self._add_item_no_sort(item)
        self.sort_table()

        if write_to_cache and self.cache_name:
            self.save_to_cache()

    def add_items(self, new_items: list[T], write_to_cache: bool = True) -> None:
        for item in new_items:
            self._add_item_no_sort(item)
        self.sort_table()
        if write_to_cache:
            self.save_to_cache()

    def apply_current_filter(self) -> None:
        """Reapply the current search filter to the table rows."""
        search_query = self.search_input.value.strip().lower()
        self.table.clear()
        self.items = {}
        for key, item in self._all_items.items():
            if not search_query or search_query in str(self.item_to_row(item)).lower():
                self.items[key] = item
                self.table.add_row(*self.item_to_row(item), key=key)
        self.sort_table()

    @on(Input.Submitted)
    async def handle_submitted_search(self) -> None:
        self.apply_current_filter()
        self.table.focus()


class LazyLabContainer(Container):
    """Base container with focusable border highlight."""

    DEFAULT_CSS = """
    LazyLabContainer {
        display: block;
        border: solid $primary-lighten-3;
    }

    LazyLabContainer:focus-within {
        min-height: 40%;
        border: solid $success;
    }
    """


class ModalDialogButtons(Horizontal):
    DEFAULT_CSS = """
    ModalDialogButtons {
        align: center middle;
        height: auto;
        width: 100%;
    }
    Button {
        margin: 1;
    }
    """

    def __init__(self, submit_text: str = "Submit", cancel_text: str = "Cancel") -> None:
        super().__init__()
        self.submit_text = submit_text
        self.cancel_text = cancel_text

    def compose(self) -> ComposeResult:
        yield Button(self.submit_text, id="submit", variant="success")
        yield Button(self.cancel_text, id="cancel", variant="error")
