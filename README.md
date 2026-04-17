# LazyLab

> **Notice:** LazyLab is being rewritten from Python to Go 1.25. Active work lives on the `go-rewrite` branch. See [docs/migration/](docs/migration/) for the plan and status. The `master` branch stays on Python until cut-over.

A terminal UI for GitLab, inspired by [lazygit](https://github.com/jesseduffield/lazygit). Browse repositories, review merge requests, view diffs, and monitor pipelines — all from your terminal.

## Features

- Browse and search GitLab projects with favorites
- View merge requests with filtering by state (opened/closed/merged) and owner
- Side-by-side diff viewer
- Pipeline status with job logs
- Merge and close MRs directly from TUI
- Vim-style keybindings
- Configurable themes

## Installation

### From release binaries

Download the latest binary for your platform from [Releases](../../releases).

```bash
# macOS / Linux
chmod +x lazylab-*
mv lazylab-* /usr/local/bin/lazylab

# Windows — rename to lazylab.exe, add to PATH
```

### From source (requires Python 3.11+ and [uv](https://docs.astral.sh/uv/))

```bash
git clone https://github.com/niklod/lazylab.git
cd lazylab
uv sync
uv run lazylab
```

### With pip

```bash
pip install git+https://github.com/niklod/lazylab.git
lazylab
```

## Configuration

On first run LazyLab creates `~/.config/lazylab/config.yaml`. Add your GitLab token:

```yaml
gitlab:
  url: https://gitlab.com          # or your self-hosted instance
  token: glpat-xxxxxxxxxxxxxxxxxxxx  # personal access token (api scope)
```

### Create a personal access token

1. Go to **GitLab → Settings → Access Tokens**
2. Create a token with the **`api`** scope
3. Paste it into `config.yaml`

### Full config example

```yaml
gitlab:
  url: https://gitlab.com
  token: glpat-xxxxxxxxxxxxxxxxxxxx

appearance:
  theme: textual-dark        # any built-in Textual theme

repositories:
  favorites:
    - group/project-one
    - group/project-two
  sort_by: last_activity

merge_requests:
  state_filter: opened       # all | opened | closed | merged
  owner_filter: all          # all | mine | reviewer

cache:
  ttl: 600                   # seconds
```

## Usage

```bash
lazylab              # launch TUI
lazylab version      # show version
lazylab dump-config  # print current config (token redacted)
lazylab clear-cache  # clear API cache
```

## Keybindings

| Key | Action |
|-----|--------|
| `h` / `l` | Previous / next section |
| `j` / `k` | Move cursor down / up |
| `J` / `K` | Page down / up |
| `g` / `G` | Go to top / bottom |
| `Enter` | Select |
| `/` | Search |
| `[` / `]` | Previous / next tab |
| `f` | Toggle favorite |
| `x` | Close MR |
| `M` | Merge MR |
| `o` | Open in browser |
| `R` | Force refresh |
| `?` | Help |
| `q` | Quit |

## License

[MIT](LICENSE)
