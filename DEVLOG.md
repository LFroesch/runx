## DevLog

### 2026-05-04: v1 blocker audit

- Audited `WORK.md` against the live codebase and removed items that are already shipped: enum selectors, interactive `y/n` replies, scheduler support, and running-page output streaming
- Reframed the work list around actual release blockers: manual smoke coverage, explicit privileged-command expectations, and final review of local unstaged UI/output changes
- Removed a stale in-app help entry for the old dry-run key and documented the `sudo -v` / `sudo -S` guidance in `README.md`

### 2026-04-30: Rerun re-prompts params
`r` on a finished tab in the Running page used to call `runScript` directly, so users couldn't toggle dry-run or change flags between runs. Extracted a `promptOrRun` helper from the script-list `enter` handler and reused it from rerun. Resolves the live `ScriptEntry` by ID first so edits since launch are honored. Files: helpers.go, update.go.

### 2026-04-30: Release helpers — optional enum omission + multi-repo sweep

- **Optional enum flags**: standalone `{{...:|--flag=}}` placeholders now drop empty selections from `args`/`flags` instead of passing empty strings to the child process
- **Legacy compatibility**: older `{{mode:--|--flag=--}}` config entries are treated as omitted when `--` is selected, which fixes release-tag style scripts without forcing immediate config edits
- **Sweep helper**: added `scripts/release-sweep.sh` to bump semver tags across sibling repos under a suite root; each repo computes its own next version and only tags when it has commits since its last semver tag
- **Docs/tests**: documented optional flag syntax and added regression coverage for empty/legacy enum omission

### 2026-04-30: Bug fixes — param spaces, stop script, help scroll

- **`shellSplit`**: now treats `{{...}}` blocks as atomic — spaces inside placeholders no longer split args/flags into separate tokens
- **Placeholder regex**: updated to allow spaces in placeholder names (e.g. `{{location title:opt1|opt2=default}}`)
- **Stop script**: added `s` key on Running page to kill the active running process
- **Help scroll**: help overlay now scrolls with `j/k/G/g/ctrl+d/ctrl+u` when content exceeds terminal height

### 2026-04-18: Drop dry run mode

- Removed `D` keybind + `modeDryRun` + `renderDryRunPanel` — redundant with the detail panel and didn't actually dry-execute anything
- Added "Full Cmd" line to `renderDetailPanel` (shown when flags/args present) to preserve the one unique bit of the dry run view

### 2026-04-13: Industry-standard hardening

- **CLI flags**: `--version` (injected via `-ldflags -X main.version`), `--config` for custom config path, `--help`
- **Graceful shutdown**: `*exec.Cmd` stored on `RunningScript`; `killRunningScripts()` called on quit/ctrl+c — no more orphaned child processes
- **Stable script IDs**: each `ScriptEntry` gets a random hex `id` field (persisted in JSON); `updateVisibleScripts` matches by ID instead of name+command+category — eliminates collision for duplicate names
- **Save error reporting**: `saveScripts()` now surfaces marshal/mkdir/write failures via status message instead of silently dropping errors
- **shellSplit backslash escapes**: handles `\"`, `\ `, etc. in double-quoted and unquoted contexts; single quotes treat backslash as literal (POSIX-correct)
- **Makefile**: added `test`, `vet`, `lint`, `clean` targets; `build` injects version from `git describe`
- **Release workflow**: ldflags now inject `GITHUB_REF_NAME` as version in release binaries
- **Version in header**: TUI header shows version string next to title
- **Test coverage**: expanded from 3 to 25+ tests — shellSplit (basic, quoted, escapes, empty), detectStdinPrompt (password, confirm variants, input, colon, empty, no-match), humanDuration, loadScripts (corrupt, backup recovery, missing), ensureScriptIDs, sort modes, generateID uniqueness, expandPath, formatElapsed
- **Cleanup**: removed unused styles (colorBg, panelHeaderStyle, detailValueStyle), cleaned .gitignore of stale tui-suite entries, ran `go mod tidy`

### 2026-04-10: Stdin overlay — full key isolation + yes/no fix

- **Key bleed fixed**: restructured `updateRunningPage` with a top-level stdin guard — when overlay is active, ALL keys are intercepted before normal page handlers; `r`/`x`/`tab`/`y`/`1-4` etc. no longer fire page-level actions while typing
- **yes/no quick reply**: confirm quick keys now send `yes\n`/`no\n` instead of `y\n`/`n\n` — works correctly with SSH host key prompts
- **Prompt trigger docs**: added table to README documenting exactly which output patterns trigger each overlay type (password, confirm, input) and the `printf` pattern for scripts

### 2026-04-10: SSH docs + stdin overlay fix

- **SSH note in README**: SSH reads passwords from `/dev/tty` not stdin — in-app password input won't work for plain `ssh`; documented `sshpass` workaround and key auth recommendation; host key confirm prompts are handled
- **Confirm regex**: extended to match `(yes/no/[fingerprint])` — SSH host key prompts now correctly detected as `"confirm"` (y/n quick keys work) instead of falling through to generic `"input"`
- **Test script**: `test-scripts/test-stdin.sh` covers all four overlay types: input, confirm, password, SSH-style host key confirm

### 2026-04-07: Running page fixes

- **Stdin input now visible**: stdin input was handled in update but never rendered — added always-visible sep+input line at bottom; shows labeled prompt when active (`password`/`confirm [y/n]`/`input`), dim "stdin ready" otherwise to avoid layout jump
- **Bottom line fix**: `update.go` scroll calcs used `m.height - 9` but view renders `m.height - 10` lines — last output line was always off-screen; unified to `m.height - 10`
- **Status line overflow**: running page `visibleHeight` now shrinks by 1 when global status message is active

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
