# bolt ⚡

A lightning-fast TUI script manager for developers. Organize and instantly run your most-used scripts and commands.

## What is bolt?

bolt is your personal script shortcut manager. Instead of remembering complex commands or navigating to script locations, just register them once and `bolt` to run them instantly.

## Features

- **Lightning Fast** - Execute any registered script with one keystroke
- **Category Organization** - Group scripts by category for easy browsing
- **Script Tracking** - Track last run time and execution count
- **Working Directory** - Set specific directories for script execution
- **Output History** - All script output is automatically saved with timestamps
- **Scrollable Output** - View long output with full scrolling support
- **Live Editing** - Edit script details directly in the TUI

## Installation

```bash
# Build
go build -o bolt main.go

# Install globally
cp bolt ~/.local/bin/

# Make sure ~/.local/bin is in PATH
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

## Usage

```bash
bolt
```

### Quick Commands

#### Main View
- `↑↓` - Navigate scripts
- `space/enter` - Run script
- `e` - Edit script details
- `n/a` - Add new script
- `o` - View output history
- `d` - Delete script
- `r` - Refresh
- `q` - Quit

#### Output View
- `↑↓/j/k` - Scroll line by line
- `pageup/pagedown` - Scroll page by page
- `esc/q` - Close output view

#### Output History
- `↑↓` - Navigate output files
- `space/enter` - View selected output
- `esc/q` - Back to main view

## Examples

Perfect for managing:
- System maintenance (`apt update`, `brew upgrade`)
- Development tasks (`npm run dev`, `docker-compose up`)
- Deployment scripts (`deploy.sh`, `backup.sh`)
- Git workflows (`git pull all repos`, `clean branches`)
- Custom automations (`resize images`, `convert videos`)

## Storage

- **Scripts**: Registered in `~/.config/bolt/bolt-scripts.json`
- **Output History**: Automatically saved to `~/.local/share/scriptgodx/` with timestamps

bolt doesn't move your scripts - it just creates shortcuts to run them from anywhere. All script output is preserved for later review.

## Example Configuration

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
      "last_run": "2024-01-15 14:30",
      "run_count": 42
    },
    {
      "name": "Docker Cleanup",
      "command": "bash",
      "args": ["-c", "docker system prune -a --volumes"],
      "workdir": "~/",
      "category": "Docker",
      "description": "Clean up unused Docker resources",
      "last_run": "2024-01-14 09:15",
      "run_count": 18
    }
  ]
}
```

## Why bolt?

Stop wasting time typing long commands or hunting for scripts. Register them once, run them instantly. ⚡