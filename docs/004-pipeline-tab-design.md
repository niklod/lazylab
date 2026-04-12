# 004: Pipeline Tab Design

## What

Pipeline tab displays stages as horizontal columns (cards) using `HorizontalScroll`, each containing jobs with status icons. Job logs open in the browser via `webbrowser.open()`.

## Why

### Horizontal stage layout

- Matches GitLab's web UI mental model (stages flow left-to-right)
- Textual's `HorizontalScroll` provides native scrolling for pipelines with many stages
- No custom layout engine needed — built entirely from `Vertical` and `HorizontalScroll` containers
- Alternative considered: vertical list of stages — rejected because it doesn't match the established GitLab UX

### Inline job logs with browser fallback

- Primary: Press Enter on a job → fetch trace via `project.jobs.get(id).trace()` → display inline in `JobLogView` (VerticalScroll)
- ANSI escape codes in CI logs converted to Rich styling via `Text.from_ansi()`
- Fallback: Press `o` to open job URL in browser (for full GitLab log viewing with section collapsing, search)
- `Escape` closes the log panel and returns to stages view
- Log panel uses `display: none` / `display: block` toggle via CSS class `visible`

### Composition over inheritance for PipelineDetail

`PipelineDetail` wraps `Pipeline` + jobs list rather than extending `Pipeline`. This keeps the minimal `Pipeline` model reusable for the Overview tab's quick status display without carrying job data.

## Widget Hierarchy

```
MRPipelineTabPane (TabPane)
└── MRPipelineTabContent (Vertical)
    ├── Static (pipeline header: id, status, ref, sha)
    ├── PipelineStagesView (HorizontalScroll, height: 1fr)
    │   ├── PipelineStageCard (Vertical, bordered)
    │   │   ├── PipelineJobWidget (Static, focusable) — Enter: show log, o: open browser
    │   │   └── ...
    │   └── ...
    └── JobLogView (VerticalScroll, height: 2fr, hidden by default)
        ├── Static (log header: job name, status, hints)
        └── Static (log content: ANSI → Rich Text)
```

## Keybindings (Pipeline tab)

- `Enter` on job → fetch + display log inline
- `o` on job → open job URL in browser
- `o` on pipeline (no job focused) → open pipeline URL
- `Escape` → close log panel
- `Ctrl+d` / `Ctrl+u` → scroll log half-page
