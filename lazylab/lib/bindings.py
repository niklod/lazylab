from textual.binding import Binding


class LazyLabBindings:
    # Global
    QUIT_APP = Binding("q", "quit", "Quit", id="app.quit")
    OPEN_COMMAND_PALETTE = Binding("ctrl+p", "command_palette", "Commands", id="app.command_palette", show=False)
    OPEN_HELP = Binding("question_mark", "open_help", "Help", id="app.help")

    # Focus sections (cycle with h/l)
    FOCUS_PREV = Binding("h", "focus_prev", "Prev Section", id="focus.prev", show=False)
    FOCUS_NEXT = Binding("l", "focus_next", "Next Section", id="focus.next", show=False)

    # Tab navigation
    PREV_TAB = Binding("[", "previous_tab", "Prev Tab", id="detail.tab.prev", show=False)
    NEXT_TAB = Binding("]", "next_tab", "Next Tab", id="detail.tab.next", show=False)

    # Table navigation (vim-like)
    SELECT_ENTRY = Binding("enter,space", "select_cursor", "Select", id="common.table.select", show=False)
    TABLE_DOWN = Binding("j", "cursor_down", show=False)
    TABLE_PAGE_DOWN = Binding("J", "scroll_down", show=False)
    TABLE_CURSOR_UP = Binding("k", "cursor_up", show=False)
    TABLE_PAGE_UP = Binding("K", "scroll_up", show=False)
    TABLE_PAGE_RIGHT = Binding("L", "page_right", show=False)
    TABLE_PAGE_LEFT = Binding("H", "page_left", show=False)
    TABLE_SCROLL_TOP = Binding("g", "scroll_home", show=False)
    TABLE_SCROLL_BOTTOM = Binding("G", "scroll_end", show=False)

    # Searchable table
    SEARCH_TABLE = Binding("/", "focus_search", "Search", id="common.table.search")

    # Repo actions
    TOGGLE_FAVORITE = Binding("f", "toggle_favorite", "Fav", id="repo.toggle_favorite")

    # Diff viewer
    DIFF_SCROLL_DOWN = Binding("ctrl+d", "diff_scroll_down", "Scroll Down", id="diff.scroll_down", show=False)
    DIFF_SCROLL_UP = Binding("ctrl+u", "diff_scroll_up", "Scroll Up", id="diff.scroll_up", show=False)

    # Open in browser
    OPEN_IN_BROWSER = Binding("o", "open_in_browser", "Open URL", id="common.open_browser")

    # Pipeline navigation
    PIPELINE_PREV_STAGE = Binding("h", "prev_stage", "Prev Stage", id="pipeline.prev_stage", show=False)
    PIPELINE_NEXT_STAGE = Binding("l", "next_stage", "Next Stage", id="pipeline.next_stage", show=False)
    PIPELINE_PREV_JOB = Binding("k", "prev_job", "Prev Job", id="pipeline.prev_job", show=False)
    PIPELINE_NEXT_JOB = Binding("j", "next_job", "Next Job", id="pipeline.next_job", show=False)

    # Pipeline job log
    CLOSE_LOG = Binding("escape", "close_log", "Close Log", id="pipeline.close_log", show=False)
    LOG_SCROLL_DOWN_LINE = Binding("j", "log_scroll_down_line", show=False)
    LOG_SCROLL_UP_LINE = Binding("k", "log_scroll_up_line", show=False)
    LOG_SCROLL_TOP = Binding("g", "log_scroll_top", "Top", id="pipeline.log.top", show=False)
    LOG_SCROLL_BOTTOM = Binding("G", "log_scroll_bottom", "Bottom", id="pipeline.log.bottom", show=False)

    # Force refresh
    FORCE_REFRESH = Binding("R", "force_refresh", "Refresh", id="common.force_refresh")

    # MR actions
    CLOSE_MR = Binding("x", "close_mr", "Close MR", id="mr.close")
    MERGE_MR = Binding("M", "merge_mr", "Merge MR", id="mr.merge")

    @classmethod
    def all(cls) -> list[Binding]:
        return [v for v in vars(cls).values() if isinstance(v, Binding)]
