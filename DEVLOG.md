## DevLog

### 2026-04-07: Fixes + {{}} improvements

- **ctrl+c fix**: moved global quit check before mode dispatch — ctrl+c now always quits, even when editing/searching/in dialogs
- **","** key opens config file in $EDITOR (falls back to nano); reloads scripts on exit
- **Password layout**: always reserve stdin line space (height-9) — no more layout jump when password prompt appears
- **History size pruning**: clear dialog (c on history page) now has two modes — "age" (delete older than N days) and "size" (prune oldest until under NMB). Toggle with `tab`, adjust with `+/-`
- **{{}} description support**: extended syntax to `{{name:Description=default}}` — description shown inline in param dialog; detail panel shows params with descriptions; help screen documents all three forms

### 2026-04-06: Bug fixes — left panel scroll, stdin pipe, copy output, ctrl+o

- **Left panel scroll**: fixed `maxScroll` in `renderScriptsPage` to account for top/bottom indicator lines consuming visible rows — cursor no longer goes off-screen at bottom of long lists
- **stdin pipe bug**: `stdinPipe` was being discarded (`_ = stdinPipe`) in `startScript`; moved cmd setup before the goroutine so stdin is correctly passed through `scriptStartedMsg`
- **Password UX**: auto-detects "password"/"passphrase" in output lines, masks stdinInput (`EchoPassword`) and focuses it; clears mask on tab switch or enter
- **Copy output**: `y` in running page copies full script output to clipboard (tries clip.exe → wl-copy → xclip → xsel)
- **ctrl+o**: now works from any edit field — auto-navigates to Work Dir field (field 4) then opens file picker

### 2026-04-06: UI fixes — height calc, q nav, E key, running page, ctrl+o

- **Scripts page height**: dynamic contentH using mode + status visibility — panels now fill available space
- **q nav**: `q` on non-scripts page goes to scripts; `q` on scripts page quits
- **E key**: opens `~/SECOND_BRAIN/sb` via `tea.ExecProcess`
- **Running page**: removed rounded border bubble (was causing line-wrap UI corruption), truncate long lines with `xansi.Truncate`; corrected scroll height constant to `m.height - 8`
- **ctrl+o**: alternative to `ctrl+f` for file picker on Work Dir field (ctrl+f intercepted by Cursor terminal)
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
