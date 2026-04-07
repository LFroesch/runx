## DevLog
### 2026-03-24: Running page (was overlay)
- Converted run panel from `modeRunPanel` overlay to dedicated `pageRunning` (key `4`)
- Removed `o` shortcut and `modeRunPanel` mode enum value
- Running script output, tab switching, close tab all work within the page layout
- `runScript(foreground=true)` and `viewOutputFile()` now switch to Running page

### 2026-03-24: Phase 4 — Pages, mode enum, cron scheduling, UI polish

**Architecture refactor:**
- Replaced 8+ boolean mode flags with single `appMode` enum — prevents impossible states
- Added `appPage` enum for top-level page navigation (Scripts, Schedules, History)
- Page switching via `1/2/3` keys, footer shows page-specific hints

**Page-based navigation:**
- **Scripts** (1): main table with category grouping, search, sort — all existing functionality
- **Schedules** (2): table of all scripts with schedule/status/last run/next run. `enter` toggles, `e` sets interval
- **History** (3): output files with separate Script/Date/Time columns (fixed timestamp parsing bug)

**Cron / scheduling:**
- `Schedule` + `ScheduleOn` fields on ScriptEntry, persisted in JSON
- In-app scheduler checks every 30s, auto-runs due scripts in background
- Min interval 1m, Go duration strings (5m, 1h, 30m, etc.)
- Schedule indicator `⏱5m` next to script names in table
- Status message on scheduled auto-start

**UI polish:**
- Header: title + page tabs + right-aligned stats (scripts, running, scheduled, sort, filter)
- Horizontal rule separators between header/content/footer
- `Runs` column added to scripts table
- Better empty state rendering (centered per-page messages)
- Schedule info in dry run preview
- Help overlay reorganized into sections

Files: all 6 — full rewrite

### 2026-03-23: Param defaults auto-run + category emojis
- Param defaults auto-run, category emoji icons, removed unused style.

### 2026-03-23: Phase 3 — Tags, env vars, dry run, sort, parameterized scripts
- Tags, env vars, dry run preview, sort toggle, parameterized scripts with `{{name}}` placeholders.

### 2026-03-23: Vim-style navigation + Category headers + Phase 1+2
- Full UI/UX overhaul: centered dialogs, help overlay, search/filter, streaming output, concurrent execution, tab bar, output history.

### 2026-03-20: Race condition fix + output display
- Fixed data race in runScript(), added scriptDoneMsg pattern.
