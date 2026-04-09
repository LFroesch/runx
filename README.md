# Runx

TUI script runner and manager. Register commands once, organize by category, run them instantly with real-time streaming output. Run multiple scripts concurrently and switch between outputs. Schedule scripts to run on intervals. Built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Quick Install

Supported platforms: Linux and macOS. On Windows, use WSL.

Recommended (installs to `~/.local/bin`):

```bash
curl -fsSL https://raw.githubusercontent.com/LFroesch/runx/main/install.sh | bash
```

Or download a binary from [GitHub Releases](https://github.com/LFroesch/runx/releases).

Or install with Go:

```bash
go install github.com/LFroesch/runx@latest
```

Or build from source:

```bash
make install
```

Command:

```bash
runx
```

### Quick Start

1. Press `n` to add a script
2. Fill in fields (tab to cycle, enter to save)
3. Press `enter` on any script to run it — output streams in real-time
4. Run multiple scripts, switch between outputs with `tab`
5. Press `2` to manage schedules, `3` to browse output history

## Features

- **Page navigation** — Switch between Scripts, Schedules, History, and Running with `1/2/3/4`
- **Streaming output** — See script output in real-time as it runs
- **Concurrent execution** — Run multiple scripts simultaneously, tab between outputs
- **Cron scheduling** — Run scripts on intervals (5m, 1h, etc.) with in-app scheduler
- **Category grouping** — Organize scripts by category with emoji icons
- **Search / filter** — `/` to live-filter by name, category, command, description, or tags
- **Parameterized scripts** — `{{name}}`, `{{name=default}}`, `{{name:Desc=default}}` placeholders prompt before running
- **Interactive input support** — Password/input/confirm prompts handled in-app (`y/n` quick confirm)
- **Tags & env vars** — Per-script tags and environment variables
- **Dry run** — Preview resolved command before executing
- **Output capture** — All output saved with timestamps to `~/.local/share/runx/`
- **Run tracking** — Last run time, execution count, and sort by usage

## Keybindings

### Global

| Key | Action |
|-----|--------|
| `1/2/3/4` | Switch page (Scripts/Schedules/History/Running) |
| `?` | Help overlay |
| `q` | Quit |

### Scripts Page

| Key | Action |
|-----|--------|
| `j/k`, `↑/↓` | Navigate |
| `G/g` | Jump to bottom/top |
| `ctrl+d/u` | Page down/up |
| `enter`, `space` | Run script |
| `D` | Dry run preview |
| `e` | Edit script |
| `n/a` | Add new script |
| `d` | Delete script |
| `/` | Search / filter |
| `s` | Sort (name/runs/recent) |
| `←/→` | Scroll table columns |
| `v` | Jump to History page |

### Schedules Page

| Key | Action |
|-----|--------|
| `enter` | Toggle schedule on/off |
| `e` | Set schedule interval |

### History Page

| Key | Action |
|-----|--------|
| `enter` | View output |
| `c` | Clear old output files |

### Running Page

| Key | Action |
|-----|--------|
| `j/k`, `↑/↓` | Scroll line by line |
| `G/g` | Jump to end / top |
| `ctrl+d/u` | Page down / up |
| `tab` | Switch between running scripts |
| `y/n` | Quick reply for confirm prompts |
| `x` | Close completed tab |

## Storage

| Location | Purpose |
|----------|---------|
| `~/.config/runx/runx-scripts.json` | Script registry (includes schedules) |
| `~/.config/runx/runx-scripts.json.bak` | Auto-backup of previous config state |
| `~/.local/share/runx/` | Output history (timestamped files) |

## Script UX Notes (v1)

- `runx` handles standard prompt-driven scripts well (`read -rp`, password prompts, `[y/N]` confirms).
- Full-screen terminal UIs (`fzf`, `vim`, `less`, `top`, etc.) need a normal terminal, not runx at this point in time.
- If unresolved placeholders remain after prompting, `runx` will block execution and show which keys are missing.

## Best Script Patterns

- Prefer explicit args/flags over interactive selection where possible.
- Use clear prompts ending with `?` or `:` for smooth input detection.
- For optional flags, use defaults in placeholders, e.g. `{{dry=--dry-run}}`.
- Keep long-running scripts line-oriented (flush output regularly) for the cleanest streaming UX.

## License

[AGPL-3.0](LICENSE)
