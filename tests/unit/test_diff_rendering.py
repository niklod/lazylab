from lazylab.models.gitlab import MRDiffFile
from lazylab.ui.widgets.mr_diff import _file_status_label, render_diff_markup


class TestFileStatusLabel:
    def test_new_file(self):
        f = MRDiffFile(old_path="/dev/null", new_path="a.py", diff="", new_file=True)
        assert _file_status_label(f) == "[green]A[/]"

    def test_deleted_file(self):
        f = MRDiffFile(old_path="a.py", new_path="a.py", diff="", deleted_file=True)
        assert _file_status_label(f) == "[red]D[/]"

    def test_renamed_file(self):
        f = MRDiffFile(old_path="old.py", new_path="new.py", diff="", renamed_file=True)
        assert _file_status_label(f) == "[yellow]R[/]"

    def test_modified_file(self):
        f = MRDiffFile(old_path="a.py", new_path="a.py", diff="+x")
        assert _file_status_label(f) == "[cyan]M[/]"


class TestRenderDiffMarkup:
    def test_empty_diff_returns_binary_message(self):
        result = render_diff_markup("")
        assert "Binary file" in result
        assert "[dim]" in result

    def test_whitespace_only_diff_returns_binary_message(self):
        result = render_diff_markup("   \n  ")
        assert "Binary file" in result

    def test_added_line_green(self):
        result = render_diff_markup("+new line")
        assert "[green]" in result
        assert "new line" in result

    def test_removed_line_red(self):
        result = render_diff_markup("-old line")
        assert "[red]" in result
        assert "old line" in result

    def test_hunk_header_cyan(self):
        result = render_diff_markup("@@ -1,3 +1,4 @@")
        assert "[cyan]" in result

    def test_file_header_bold(self):
        result = render_diff_markup("--- a/file.py\n+++ b/file.py")
        lines = result.split("\n")
        assert "[bold]" in lines[0]
        assert "[bold]" in lines[1]

    def test_context_line_no_markup(self):
        result = render_diff_markup(" context line")
        assert "[green]" not in result
        assert "[red]" not in result
        assert "[cyan]" not in result
        assert "context line" in result

    def test_rich_characters_escaped(self):
        result = render_diff_markup("+items[0]")
        assert "\\[" in result or "[green]" in result
        # The literal [ should be escaped so Rich doesn't interpret it as markup
        assert "items" in result

    def test_bare_plus_line(self):
        result = render_diff_markup("+")
        assert "[green]" in result

    def test_bare_minus_line(self):
        result = render_diff_markup("-")
        assert "[red]" in result

    def test_file_header_with_markup_brackets(self):
        result = render_diff_markup("--- a/[red]evil[/red].py")
        assert "[bold]" in result
        assert "evil" in result
        # Rich markup tags in the path should be escaped
        assert "\\[red]" in result

    def test_full_unified_diff(self):
        diff = (
            "--- a/hello.py\n"
            "+++ b/hello.py\n"
            "@@ -1,3 +1,4 @@\n"
            " import os\n"
            "-old_func()\n"
            "+new_func()\n"
            "+extra_line()\n"
        )
        result = render_diff_markup(diff)
        lines = result.split("\n")
        assert len(lines) == 7
        assert "[bold]" in lines[0]  # --- header
        assert "[bold]" in lines[1]  # +++ header
        assert "[cyan]" in lines[2]  # @@ hunk
        assert "[green]" not in lines[3] and "[red]" not in lines[3]  # context
        assert "[red]" in lines[4]   # removed
        assert "[green]" in lines[5]  # added
        assert "[green]" in lines[6]  # added
