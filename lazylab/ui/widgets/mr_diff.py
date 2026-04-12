from rich.markup import escape
from rich.text import Text
from textual import on
from textual.app import ComposeResult
from textual.containers import Horizontal, VerticalScroll
from textual.content import Content
from textual.widgets import Static, Tree
from textual.widgets.tree import TreeNode

from lazylab.lib.bindings import LazyLabBindings
from lazylab.models.gitlab import MRDiffData, MRDiffFile


def _file_status_label(file: MRDiffFile) -> str:
    if file.new_file:
        return "[green]A[/]"
    if file.deleted_file:
        return "[red]D[/]"
    if file.renamed_file:
        return "[yellow]R[/]"
    return "[cyan]M[/]"


def render_diff_markup(raw_diff: str) -> str:
    if not raw_diff.strip():
        return "[dim]Binary file or no diff available[/]"

    lines: list[str] = []
    for line in raw_diff.splitlines():
        escaped = escape(line)
        if line.startswith("+++") or line.startswith("---"):
            lines.append(f"[bold]{escaped}[/]")
        elif line.startswith("+"):
            lines.append(f"[green]{escaped}[/]")
        elif line.startswith("-"):
            lines.append(f"[red]{escaped}[/]")
        elif line.startswith("@@"):
            lines.append(f"[cyan]{escaped}[/]")
        else:
            lines.append(escaped)

    return "\n".join(lines)


class DiffFileTree(Tree[MRDiffFile]):
    DEFAULT_CSS = """
    DiffFileTree {
        width: 30%;
        min-width: 20;
        height: 100%;
        border-right: solid $primary-lighten-3;
    }
    """

    show_root = False

    BINDINGS = [
        LazyLabBindings.TABLE_DOWN,
        LazyLabBindings.TABLE_CURSOR_UP,
        LazyLabBindings.TABLE_SCROLL_TOP,
        LazyLabBindings.TABLE_SCROLL_BOTTOM,
        LazyLabBindings.HALF_PAGE_DOWN,
        LazyLabBindings.HALF_PAGE_UP,
    ]

    def action_half_page_down(self) -> None:
        for _ in range(self.size.height // 2):
            self.action_cursor_down()

    def action_half_page_up(self) -> None:
        for _ in range(self.size.height // 2):
            self.action_cursor_up()

    def __init__(self) -> None:
        super().__init__("Files", id="diff-file-tree")

    def set_files(self, files: list[MRDiffFile]) -> None:
        self.clear()
        dir_nodes: dict[str, TreeNode[MRDiffFile]] = {}

        for file in files:
            parts = file.new_path.split("/")
            filename = parts[-1]
            dir_parts = parts[:-1]

            parent: TreeNode[MRDiffFile] = self.root
            for i, segment in enumerate(dir_parts):
                dir_key = "/".join(dir_parts[: i + 1])
                if dir_key not in dir_nodes:
                    label = Text.from_markup(f"[bold]{escape(segment)}/[/]")
                    node = parent.add(label, expand=True)
                    dir_nodes[dir_key] = node
                parent = dir_nodes[dir_key]

            status = _file_status_label(file)
            leaf_label = Text.from_markup(f"{status} {escape(filename)}")
            parent.add_leaf(leaf_label, data=file)


class DiffContentView(VerticalScroll):
    DEFAULT_CSS = """
    DiffContentView {
        width: 1fr;
        height: 100%;
    }
    DiffContentView Static {
        width: 100%;
    }
    DiffContentView .diff-message {
        padding: 1 2;
    }
    """

    def __init__(self) -> None:
        super().__init__(id="diff-content-view")
        self._diff_static = Static(id="diff-content-static")

    def compose(self) -> ComposeResult:
        yield self._diff_static

    def show_diff(self, diff_file: MRDiffFile) -> None:
        markup = render_diff_markup(diff_file.diff)
        self._diff_static.update(Content.from_markup(markup))
        self.scroll_home(animate=False)

    def show_loading(self) -> None:
        self._diff_static.update(Content.from_markup("[dim]Loading diff...[/]"))

    def show_empty(self) -> None:
        self._diff_static.update(Content.from_markup("[dim]Select a file to view diff[/]"))

    def show_error(self, msg: str) -> None:
        self._diff_static.update(Content.from_markup(f"[red]{escape(msg)}[/]"))


class MRDiffTabContent(Horizontal):
    DEFAULT_CSS = """
    MRDiffTabContent {
        width: 100%;
        height: 100%;
    }
    """

    BINDINGS = [
        LazyLabBindings.HALF_PAGE_DOWN,
        LazyLabBindings.HALF_PAGE_UP,
    ]

    def __init__(self, **kwargs) -> None:
        super().__init__(**kwargs)
        self._file_tree = DiffFileTree()
        self._diff_content = DiffContentView()

    def compose(self) -> ComposeResult:
        yield self._file_tree
        yield self._diff_content

    def action_half_page_down(self) -> None:
        self._diff_content.scroll_relative(y=self._diff_content.size.height // 2)

    def action_half_page_up(self) -> None:
        self._diff_content.scroll_relative(y=-(self._diff_content.size.height // 2))

    @on(Tree.NodeSelected)
    def on_file_selected(self, event: Tree.NodeSelected[MRDiffFile]) -> None:
        node = event.node
        if node.data is not None:
            self._diff_content.show_diff(node.data)

    def load_diff(self, diff_data: MRDiffData) -> None:
        self._file_tree.set_files(diff_data.files)
        if diff_data.files:
            self._diff_content.show_diff(diff_data.files[0])
        else:
            self._diff_content.show_empty()

    def show_loading(self) -> None:
        self._diff_content.show_loading()

    def show_error(self, msg: str) -> None:
        self._diff_content.show_error(msg)
