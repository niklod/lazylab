"""Modal confirmation screens for MR close and merge actions."""

from dataclasses import dataclass

from textual import on
from textual.app import ComposeResult
from textual.containers import Vertical
from textual.content import Content
from textual.screen import ModalScreen
from textual.widgets import Button, Checkbox, Label, Static

from lazylab.models.gitlab import MergeRequest
from lazylab.ui.widgets.common import ModalDialogButtons


@dataclass
class MergeResult:
    should_remove_source_branch: bool
    merge_when_pipeline_succeeds: bool


class CloseMRScreen(ModalScreen[bool]):
    DEFAULT_CSS = """
    CloseMRScreen {
        align: center middle;
    }
    CloseMRScreen > Vertical {
        width: 60;
        height: auto;
        max-height: 80%;
        padding: 1 2;
        border: solid $primary;
        background: $surface;
    }
    CloseMRScreen .modal-title {
        text-style: bold;
        margin-bottom: 1;
    }
    CloseMRScreen .modal-subtitle {
        margin-bottom: 1;
        color: $text-muted;
    }
    """

    BINDINGS = [("escape", "cancel", "Cancel")]

    def __init__(self, mr: MergeRequest) -> None:
        super().__init__()
        self.mr = mr

    def compose(self) -> ComposeResult:
        with Vertical():
            yield Static(Content.from_markup(f"[bold]Close MR !{self.mr.iid}?[/]"), classes="modal-title")
            yield Label(self.mr.title, classes="modal-subtitle")
            yield ModalDialogButtons(submit_text="Close", cancel_text="Cancel")

    @on(Button.Pressed, "#submit")
    def on_submit(self) -> None:
        self.dismiss(True)

    @on(Button.Pressed, "#cancel")
    def on_cancel_button(self) -> None:
        self.dismiss(False)

    def action_cancel(self) -> None:
        self.dismiss(False)


class MergeMRScreen(ModalScreen[MergeResult | None]):
    DEFAULT_CSS = """
    MergeMRScreen {
        align: center middle;
    }
    MergeMRScreen > Vertical {
        width: 60;
        height: auto;
        max-height: 80%;
        padding: 1 2;
        border: solid $primary;
        background: $surface;
    }
    MergeMRScreen .modal-title {
        text-style: bold;
        margin-bottom: 1;
    }
    MergeMRScreen .modal-subtitle {
        margin-bottom: 1;
        color: $text-muted;
    }
    MergeMRScreen Checkbox {
        margin-bottom: 1;
    }
    """

    BINDINGS = [("escape", "cancel", "Cancel")]

    def __init__(self, mr: MergeRequest) -> None:
        super().__init__()
        self.mr = mr

    def compose(self) -> ComposeResult:
        with Vertical():
            yield Static(Content.from_markup(f"[bold]Merge MR !{self.mr.iid}?[/]"), classes="modal-title")
            yield Label(self.mr.title, classes="modal-subtitle")
            yield Label(f"{self.mr.source_branch} \u2192 {self.mr.target_branch}")
            yield Checkbox("Delete source branch", id="delete-branch")
            yield Checkbox("Merge when pipeline succeeds", id="merge-when-succeeds")
            yield ModalDialogButtons(submit_text="Merge", cancel_text="Cancel")

    @on(Button.Pressed, "#submit")
    def on_submit(self) -> None:
        delete_branch = self.query_one("#delete-branch", Checkbox).value
        merge_when_succeeds = self.query_one("#merge-when-succeeds", Checkbox).value
        self.dismiss(MergeResult(
            should_remove_source_branch=delete_branch,
            merge_when_pipeline_succeeds=merge_when_succeeds,
        ))

    @on(Button.Pressed, "#cancel")
    def on_cancel_button(self) -> None:
        self.dismiss(None)

    def action_cancel(self) -> None:
        self.dismiss(None)
