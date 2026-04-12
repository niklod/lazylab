"""Help screen showing keybinding cheatsheet."""

from textual.app import ComposeResult
from textual.containers import Vertical, VerticalScroll
from textual.screen import ModalScreen
from textual.widgets import Markdown

HELP_TEXT = """\
# LazyLab Keybindings

## Navigation
| Key | Action |
|-----|--------|
| `h` / `l` | Previous / next section |
| `j` / `k` | Move cursor down / up |
| `J` / `K` | Page down / up |
| `g` / `G` | Go to top / bottom |
| `Enter` / `Space` | Select entry |
| `/` | Search |

## Tabs
| Key | Action |
|-----|--------|
| `[` / `]` | Previous / next tab |

## MR Actions
| Key | Action |
|-----|--------|
| `x` | Close MR |
| `M` | Merge MR |

## Diff
| Key | Action |
|-----|--------|
| `Ctrl+d` / `Ctrl+u` | Scroll diff down / up |

## Pipeline
| Key | Action |
|-----|--------|
| `h` / `l` | Previous / next stage |
| `j` / `k` | Previous / next job |
| `Enter` | View job log |
| `Escape` | Close job log |
| `o` | Open in browser |

## Global
| Key | Action |
|-----|--------|
| `?` | This help screen |
| `q` | Quit |
| `Ctrl+p` | Command palette |

*Press `Escape` or `?` to close.*
"""


class HelpScreen(ModalScreen[None]):
    DEFAULT_CSS = """
    HelpScreen {
        align: center middle;
    }
    HelpScreen > Vertical {
        width: 64;
        height: 80%;
        padding: 1 2;
        border: solid $primary;
        background: $surface;
    }
    """

    BINDINGS = [
        ("escape", "close", "Close"),
        ("question_mark", "close", "Close"),
    ]

    def compose(self) -> ComposeResult:
        with Vertical():
            with VerticalScroll():
                yield Markdown(HELP_TEXT)

    def action_close(self) -> None:
        self.dismiss(None)
