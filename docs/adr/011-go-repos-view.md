# 011: Repos View, Search Lifecycle, and Views Package Layout

## Decision

Phase G2-task-2 ships three new pieces:

1. **`internal/tui/views/` subpackage** owns per-pane widgets (repos today; mrs
   and detail tabs land in G3/G4). Views expose `Bindings() []keymap.Binding`
   and a `Render(*gocui.View)` method called from the layout tick. `tui/app.go`
   constructs a `*views.Views` and threads it through `tui.NewManager` +
   `tui.Bind`.

2. **`internal/tui/keymap/` leaf package** holds the shared `Binding` type,
   `HandlerFunc` alias, and pane-name constants (`ViewRepos`, `ViewMRs`,
   `ViewDetail`, `ViewReposSearch`). Both `tui` and `tui/views` import keymap;
   neither imports the other, so the subpackage split does not produce an
   import cycle.

3. **Search lifecycle via transient view**. Pressing `/` inside the repos pane
   flips `searchActive = true`. The next layout tick creates a single-line
   editable view `repos_search` overlaying the top of the repos rectangle
   (`gocui.DefaultEditor`). `Enter` submits — the view reads the buffer,
   applies the filter, deletes the search view, and refocuses `repos`. `Esc`
   clears the query and does the same. The search view's existence is
   controlled entirely by `ReposView.searchActive`; the layout tick reconciles
   it on every frame, so a missed `g.DeleteView` from a handler self-heals on
   the next render.

## Context

The Python widget (`lazylab/ui/widgets/repositories.py`) wraps Textual's
`DataTable` inside a `SearchableDataTable[T]` generic container. Textual offers
`display = False` to hide the search input; gocui does not — views either exist
with a non-zero rectangle or not at all. The port therefore picks one of:

- **Permanent 1-row pane with height toggled by layout.** Rejected: gocui's
  `SetView` errors on zero-width/zero-height rectangles.
- **Ephemeral view created per invocation.** Accepted: matches gocui idioms and
  mirrors how `jesseduffield/lazygit` drives its own confirmation prompts.

For package layout, the module-mapping doc (`docs/migration/002-module-mapping.md`)
prescribes `internal/tui/views/repos.go`. The naïve implementation would have
`tui` import `views` (to wire them at startup) and `views` import `tui` (for
the `Binding` type + pane-name constants) — a cycle. Three options were
weighed:

- **Inline in `tui`**: simplest but violates the module-mapping plan and lets
  a single package grow unboundedly.
- **Interface on the `tui` side that views satisfy**: abstract but loses the
  concrete `Binding` struct's ergonomics (handlers accessed via interface
  dispatch).
- **Leaf `keymap` package** shared by both directions: keeps `Binding` as a
  concrete struct, costs one small package, breaks the cycle. Chosen.

`AppContext` previously carried `(Config, GitLab, Cache, CurrentProject)`.
Favourite persistence needs `Config.Save(fs, path)` — both dependencies have
to reach the view. Stashing them as additional `AppContext` fields (`FS`,
`ConfigPath`) is a mild breaking change to the constructor and keeps the
"single injected handle" ergonomic. `cli.Run` already owns `fs` and
`configPath` via its option funcs, so the change is local.

## Consequences

**Positive:**
- `tui.NewManager(views)` is an exported test seam — integration tests install
  the production layout without re-implementing it. The e2e parity gate
  (`tests/e2e/repos_render_test.go`) uses it directly.
- `ReposView.LoadSync(ctx)` lets integration tests drive the fetch-and-apply
  path without running `g.MainLoop`, which would require background threads
  and signal handling in tests. Production uses `Load(ctx)` which wraps
  `LoadSync` in a goroutine + `g.Update`.
- Per-view handlers live on the view struct; `tui.Bind` is a variadic passthrough.
  Adding the next pane is `views.Something` + `v.Repos, v.Something`.
- Immutability: `toggleFavoriteList` returns a new `[]string` instead of
  mutating `Config.Repositories.Favorites` in place, honouring the project's
  immutability rule.

**Known follow-ups (not this task):**
- `ReposView.fetchProjects` bypasses the cache — Phase G6 wraps
  `Client.ListProjects` with `cache.Do[T]`, giving the view SWR for free.
- The search pane is one line tall; multiline search is not needed to match
  Python parity. If a future phase needs richer input (regex, completion),
  revisit `gocui.DefaultEditor`.
- `RepoSelected` event wiring is deferred to G3 when the MRs view needs it.
  For now `SelectedProject()` is exposed for in-process tests but unused by
  production code paths.
- A shared `SearchableTable[T]` generic is *not* extracted yet. G3's MRs view
  will reveal the true shared shape — premature abstraction here would likely
  need to be reworked when MRs introduce status icons and multi-column sort.

**Divergence logged:** the module-mapping doc said
`internal/tui/views/common.go` for the shared searchable-table helper (mirror
of Python's `ui/widgets/common.py`). That file is *not* created here, because
there is nothing common yet. The mapping row stays advisory until G3.

## Alternatives considered

- **Implement search as a modal popup over all three panes.** Matches how
  command palettes work but obscures the list behind the search, and breaks
  Python parity (which overlays only the repos pane).
- **Store favourites in a separate file (`favorites.yaml`)** instead of in the
  main config. Rejected — Python keeps everything in one YAML under
  `repositories.favorites`, and forking the storage layout for Go would create
  a config-migration burden during cut-over.
- **Expose `tui.Bind` as a method on a `*Controller`** instead of a free
  function. Cleaner in the abstract but would require rewriting
  `cli/run.go` + existing tests for no concrete gain today. Revisit if G7
  adds a command palette that needs to mutate the keymap dynamically.
