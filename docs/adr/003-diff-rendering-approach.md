# 003: Diff Rendering Approach

## Decision

Render MR diffs using Textual's `Static` widget with Rich markup inside a `VerticalScroll` container. The file tree uses Textual's built-in `Tree` widget.

## Why

- **Static + Rich markup** is the simplest approach that provides colored unified diffs (green=added, red=removed, cyan=hunks). Matches the pattern already used by `MROverviewTabPane`. No custom rendering widget needed.
- **Tree widget** provides hierarchical display, expand/collapse, cursor navigation, and `NodeSelected` messages out of the box. Files are grouped by directory path.
- **Separate `mr_diff.py` file** keeps widget code isolated from `merge_requests.py`, which will grow in future phases. Both stay well under the 800-line limit.

## Tradeoffs

- Rich markup escaping (`rich.markup.escape()`) is required before wrapping diff content in color tags, since diff content can contain `[` and `]` characters.
- Large diffs may be slow to render in a single `Static` widget. Acceptable for Phase 5; a future optimization could virtualize rendering.
- No side-by-side diff mode — unified only for now.
- Diff parsing happens in the widget layer (presentation concern), while models store raw unified diff strings (reusable for future views).
