# Runx

TUI script runner and manager. Register commands once, organize by category, run them instantly with output capture and history. Built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea).


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

1. Press `n` or `a` to add a script
2. Fill in name, command, args, working directory, category
3. Press `enter`/`space` on any script to run it
4. Output displays inline and is saved to history

## Features

- **Instant execution** — Run registered scripts with one keystroke
- **Category grouping** — Organize scripts by category (System, Docker, Git, etc.)
- **Output capture** — All output saved with timestamps to `~/.local/share/runx/`
- **Run tracking** — Last run time and execution count per script
- **Working directory** — Set per-script working directories
- **Scrollable output** — View long output with j/k and pgup/pgdn
- **Output history** — Browse past runs and their output

## Keybindings

### Main View
| Key | Action |
|-----|--------|
| `j/k`, `up/down` | Navigate |
| `enter`, `space` | Run script |
| `n/a` | Add new script |
| `e` | Edit script |
| `d` | Delete script |
| `o` | View output history |
| `r` | Refresh |
| `q` | Quit |

### Output View
| Key | Action |
|-----|--------|
| `j/k`, `up/down` | Scroll line by line |
| `pgup/pgdn` | Scroll page |
| `esc`, `q` | Close |

## Storage

| Location | Purpose |
|----------|---------|
| `~/.config/runx/runx-scripts.json` | Script registry |
| `~/.local/share/runx/` | Output history (timestamped files) |

## Example Config

```json
{
  "scripts": [
    {
      "name": "System Update",
      "command": "bash",
      "args": ["-c", "sudo apt update && sudo apt upgrade"],
      "workdir": "~/",
      "category": "System",
      "description": "Update system packages",
      "last_run": "2026-01-15 14:30",
      "run_count": 42
    }
  ]
}
```

## License

[AGPL-3.0](LICENSE)
