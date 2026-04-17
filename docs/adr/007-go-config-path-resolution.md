# 007 — Go config path: manual XDG resolution over `adrg/xdg`

## What

`internal/config.DefaultConfigPath()` resolves the config file location using manual env-var lookup
(`LAZYLAB_CONFIG`, then `XDG_CONFIG_HOME`, then `~/.config`) instead of the `adrg/xdg` library that
the migration stack rationale (`docs/migration/001-stack-rationale.md`) originally called for.

Order of precedence:

1. `LAZYLAB_CONFIG` — explicit override for tests and edge cases.
2. `$XDG_CONFIG_HOME/lazylab/config.yaml` — XDG-compliant override.
3. `$HOME/.config/lazylab/config.yaml` — fallback on every platform.

Cache and log paths follow the same derivation via `defaultAppDir()`.

## Why

Python's `lazylab/lib/constants.py` hard-codes `Path.home() / ".config" / "lazylab"` regardless of
OS, and `CLAUDE.md` states that the Go rewrite keeps the config path unchanged. `adrg/xdg` applies
platform-specific conventions (on macOS: `~/Library/Application Support`), which would split the
existing Python users' configs from the Go binary at cut-over time.

A thin manual resolver is cheaper than introducing a dependency whose default behavior we have to
fight on every call site, and it keeps the `XDG_CONFIG_HOME` override working for users who do set
it.

## Tradeoffs

- We lose automatic support for `XDG_STATE_HOME` / `XDG_RUNTIME_DIR` and other XDG dirs, but we
  don't use those today.
- If we later want full XDG compliance on macOS, re-introducing `adrg/xdg` plus a mac-override
  shim is ~20 lines and cleanly replaces `defaultAppDir()`.

## Alternatives considered

- **`adrg/xdg` with macOS override** — still requires platform-specific code, and the lib
  initializes its paths at import time which complicates testing.
- **Full XDG on all platforms** — breaks parity with the Python build mid-migration.
