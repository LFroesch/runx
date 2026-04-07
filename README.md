# Runx

TUI script runner and manager. Register commands once, organize by category, run them instantly with real-time streaming output. Run multiple scripts concurrently and switch between outputs. Schedule scripts to run on intervals. Built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Install

```bash
go install github.com/LFroesch/runx@latest
```

Or build from source:

```bash
make install
```

## Usage

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
- **Parameterized scripts** — `{{name}}` placeholders prompt for values before running
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
| `x` | Close completed tab |

## Storage

| Location | Purpose |
|----------|---------|
| `~/.config/runx/runx-scripts.json` | Script registry (includes schedules) |
| `~/.local/share/runx/` | Output history (timestamped files) |

## License

[AGPL-3.0](LICENSE)
