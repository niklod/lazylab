from textual.widgets import Markdown, TabPane

INFO_TEXT = """\
# LazyLab

Welcome to **LazyLab** — a Terminal UI for GitLab.

## Navigation

- **1** / **2** / **3** — Focus repos / MRs / details
- **j** / **k** — Move down / up
- **/** — Search
- **[** / **]** — Previous / next tab
- **q** — Quit
- **?** — Help

Select a repository to get started.
"""


class LazyLabInfoTabPane(TabPane):
    def __init__(self) -> None:
        super().__init__("Info", id="info-tab")

    def compose(self):
        yield Markdown(INFO_TEXT)
