from textual.binding import Binding


class LazyLabBindings:
    # Global
    QUIT_APP = Binding("q", "quit", "Quit", id="app.quit")
    OPEN_COMMAND_PALETTE = Binding("ctrl+p", "command_palette", "Commands", id="app.command_palette", show=False)
    OPEN_HELP = Binding("question_mark", "open_help", "Help", id="app.help")

    # Focus sections
    FOCUS_REPOS = Binding("1", "focus_section('#repos_table')", "Repos", id="focus.repos", show=False)
    FOCUS_MRS = Binding("2", "focus_section('#mrs_table')", "MRs", id="focus.mrs", show=False)
    FOCUS_DETAILS = Binding("3", "focus_tabs", "Details", id="focus.details", show=False)

    # Tab navigation
    PREV_TAB = Binding("[", "previous_tab", "Prev Tab", id="detail.tab.prev", show=False)
    NEXT_TAB = Binding("]", "next_tab", "Next Tab", id="detail.tab.next", show=False)

    # Table navigation (vim-like)
    SELECT_ENTRY = Binding("enter,space", "select_cursor", "Select", id="common.table.select", show=False)
    TABLE_DOWN = Binding("j", "cursor_down", show=False)
    TABLE_PAGE_DOWN = Binding("J", "scroll_down", show=False)
    TABLE_CURSOR_UP = Binding("k", "cursor_up", show=False)
    TABLE_PAGE_UP = Binding("K", "scroll_up", show=False)
    TABLE_SCROLL_RIGHT = Binding("l", "scroll_right", show=False)
    TABLE_PAGE_RIGHT = Binding("L", "page_right", show=False)
    TABLE_SCROLL_LEFT = Binding("h", "scroll_left", show=False)
    TABLE_PAGE_LEFT = Binding("H", "page_left", show=False)
    TABLE_SCROLL_TOP = Binding("g", "scroll_home", show=False)
    TABLE_SCROLL_BOTTOM = Binding("G", "scroll_end", show=False)

    # Searchable table
    SEARCH_TABLE = Binding("/", "focus_search", "Search", id="common.table.search")

    # Repo actions
    TOGGLE_FAVORITE = Binding("f", "toggle_favorite", "Fav", id="repo.toggle_favorite")

    # MR actions
    # (Phase 8: CREATE_MR, CLOSE_MR, MERGE_MR)

    @classmethod
    def all(cls) -> list[Binding]:
        return [v for v in vars(cls).values() if isinstance(v, Binding)]
