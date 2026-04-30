package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- shellSplit ---

func TestShellSplitBasic(t *testing.T) {
	got := shellSplit("echo hello world")
	want := []string{"echo", "hello", "world"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestShellSplitQuoted(t *testing.T) {
	got := shellSplit(`echo "hello world" 'foo bar'`)
	want := []string{"echo", "hello world", "foo bar"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestShellSplitBackslashEscape(t *testing.T) {
	got := shellSplit(`echo hello\ world`)
	want := []string{"echo", "hello world"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestShellSplitEscapedQuoteInDouble(t *testing.T) {
	got := shellSplit(`echo "hello \"world\""`)
	want := []string{"echo", `hello "world"`}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestShellSplitBackslashLiteralInSingleQuotes(t *testing.T) {
	got := shellSplit(`'hello\nworld'`)
	want := []string{`hello\nworld`}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got[0] != want[0] {
		t.Fatalf("got %q, want %q", got[0], want[0])
	}
}

func TestShellSplitEmpty(t *testing.T) {
	got := shellSplit("")
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestShellSplitPlaceholderWithSpaces(t *testing.T) {
	// Spaces inside {{ }} must not split the token
	got := shellSplit(`--only={{location title:|remote|sf=}} --hours=72`)
	want := []string{"--only={{location title:|remote|sf=}}", "--hours=72"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

// --- detectStdinPrompt ---

func TestDetectStdinPromptPassword(t *testing.T) {
	kind, label, ok := detectStdinPrompt("Enter password: ")
	if !ok || kind != "password" {
		t.Fatalf("expected password prompt, got kind=%q ok=%v", kind, ok)
	}
	if label == "" {
		t.Fatal("expected non-empty label")
	}
}

func TestDetectStdinPromptConfirm(t *testing.T) {
	tests := []string{
		"Continue? [y/n]",
		"Are you sure? (yes/no)",
		"Trust this key? (yes/no/[fingerprint])",
	}
	for _, input := range tests {
		kind, _, ok := detectStdinPrompt(input)
		if !ok || kind != "confirm" {
			t.Fatalf("input %q: expected confirm, got kind=%q ok=%v", input, kind, ok)
		}
	}
}

func TestDetectStdinPromptInput(t *testing.T) {
	kind, _, ok := detectStdinPrompt("What is your name?")
	if !ok || kind != "input" {
		t.Fatalf("expected input prompt, got kind=%q ok=%v", kind, ok)
	}
}

func TestDetectStdinPromptInputColon(t *testing.T) {
	kind, _, ok := detectStdinPrompt("Enter token:")
	if !ok || kind != "input" {
		t.Fatalf("expected input prompt, got kind=%q ok=%v", kind, ok)
	}
}

func TestDetectStdinPromptNoMatch(t *testing.T) {
	_, _, ok := detectStdinPrompt("just some output line")
	if ok {
		t.Fatal("expected no match")
	}
}

func TestDetectStdinPromptEmpty(t *testing.T) {
	_, _, ok := detectStdinPrompt("")
	if ok {
		t.Fatal("expected no match for empty string")
	}
}

// --- humanDuration ---

func TestHumanDurationSubMinute(t *testing.T) {
	if got := humanDuration(30e9); got != "<1m" {
		t.Fatalf("got %q", got)
	}
}

func TestHumanDurationMinutes(t *testing.T) {
	if got := humanDuration(5 * 60e9); got != "5m" {
		t.Fatalf("got %q", got)
	}
}

func TestHumanDurationHours(t *testing.T) {
	if got := humanDuration(2 * 3600e9); got != "2h" {
		t.Fatalf("got %q", got)
	}
}

func TestHumanDurationHoursMinutes(t *testing.T) {
	if got := humanDuration(90 * 60e9); got != "1h30m" {
		t.Fatalf("got %q", got)
	}
}

// --- loadScripts ---

func TestLoadScriptsCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "runx", "scripts.json")
	os.MkdirAll(filepath.Dir(configFile), 0755)
	os.WriteFile(configFile, []byte("{invalid json}"), 0644)

	// No backup — should return empty
	scripts := loadScripts(configFile)
	if len(scripts) != 0 {
		t.Fatalf("expected empty scripts for corrupt JSON, got %d", len(scripts))
	}
}

func TestLoadScriptsCorruptWithBackup(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "runx", "scripts.json")
	os.MkdirAll(filepath.Dir(configFile), 0755)
	os.WriteFile(configFile, []byte("{invalid}"), 0644)

	backup := ScriptManager{Scripts: []ScriptEntry{{Name: "backup-script", Command: "echo"}}}
	data, _ := json.Marshal(backup)
	os.WriteFile(configFile+".bak", data, 0644)

	scripts := loadScripts(configFile)
	if len(scripts) != 1 || scripts[0].Name != "backup-script" {
		t.Fatalf("expected backup recovery, got %v", scripts)
	}
}

func TestLoadScriptsMissing(t *testing.T) {
	scripts := loadScripts("/nonexistent/path/scripts.json")
	if len(scripts) != 1 || scripts[0].Name != "Git Status All" {
		t.Fatal("expected default script for missing config")
	}
}

// --- ensureScriptIDs ---

func TestEnsureScriptIDs(t *testing.T) {
	scripts := []ScriptEntry{
		{Name: "a", Command: "echo"},
		{Name: "b", Command: "echo", ID: "existing"},
	}
	changed := ensureScriptIDs(scripts)
	if !changed {
		t.Fatal("expected changed=true")
	}
	if scripts[0].ID == "" {
		t.Fatal("expected ID assigned to script 0")
	}
	if scripts[1].ID != "existing" {
		t.Fatalf("expected existing ID preserved, got %q", scripts[1].ID)
	}
}

func TestEnsureScriptIDsNoop(t *testing.T) {
	scripts := []ScriptEntry{
		{Name: "a", ID: "id1"},
		{Name: "b", ID: "id2"},
	}
	changed := ensureScriptIDs(scripts)
	if changed {
		t.Fatal("expected changed=false when all have IDs")
	}
}

// --- Parameterized scripts ---

func TestExtractPlaceholdersIncludesWorkdirAndEnv(t *testing.T) {
	s := ScriptEntry{
		Command: "bash",
		Args:    []string{"-c", "echo {{name=world}}"},
		WorkDir: "{{path=~/projects}}",
		EnvVars: map[string]string{
			"TOKEN": "{{token:API token}}",
		},
	}
	fields := extractPlaceholders(s)
	joined := make([]string, 0, len(fields))
	for _, f := range fields {
		joined = append(joined, f.Name)
	}
	got := strings.Join(joined, ",")
	for _, want := range []string{"name", "path", "token"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing placeholder %q in %q", want, got)
		}
	}
}

func TestSubstitutePlaceholdersAppliesWorkdirAndEnv(t *testing.T) {
	s := ScriptEntry{
		Command: "echo",
		Args:    []string{"{{who=world}}"},
		WorkDir: "{{dir=~/tmp}}",
		EnvVars: map[string]string{
			"FOO": "{{foo=bar}}",
		},
	}
	out := substitutePlaceholders(s, map[string]string{"who": "team", "foo": "baz"})
	if out.Args[0] != "team" {
		t.Fatalf("expected args placeholder substitution, got %q", out.Args[0])
	}
	if !strings.Contains(out.WorkDir, "tmp") || strings.Contains(out.WorkDir, "{{") {
		t.Fatalf("expected workdir substitution, got %q", out.WorkDir)
	}
	if out.EnvVars["FOO"] != "baz" {
		t.Fatalf("expected env placeholder substitution, got %q", out.EnvVars["FOO"])
	}
}

func TestUnresolvedPlaceholders(t *testing.T) {
	s := ScriptEntry{
		Command: "echo {{name}}",
		Args:    []string{"{{path=~/}}"},
		WorkDir: "{{dir}}",
	}
	out := substitutePlaceholders(s, map[string]string{"name": "ok"})
	unresolved := unresolvedPlaceholders(out)
	got := strings.Join(unresolved, ",")
	if strings.Contains(got, "name") {
		t.Fatalf("name should be resolved, got %q", got)
	}
	for _, want := range []string{"dir"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected unresolved %q in %q", want, got)
		}
	}
}

func TestExtractPlaceholdersEnum(t *testing.T) {
	s := ScriptEntry{
		Command: "deploy",
		Args:    []string{"{{env:staging|prod|dev=staging}}"},
	}
	fields := extractPlaceholders(s)
	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}
	f := fields[0]
	if f.Name != "env" {
		t.Fatalf("expected name=env, got %q", f.Name)
	}
	if len(f.Options) != 3 {
		t.Fatalf("expected 3 options, got %v", f.Options)
	}
	if f.Default != "staging" {
		t.Fatalf("expected default=staging, got %q", f.Default)
	}
}

// --- Sort ---

func TestSortByName(t *testing.T) {
	m := model{
		scripts: []ScriptEntry{
			{Name: "Zebra", Category: "B"},
			{Name: "Alpha", Category: "A"},
			{Name: "Beta", Category: "A"},
		},
		sortMode: sortByName,
	}
	sorted := m.getSortedScripts()
	if sorted[0].Name != "Alpha" || sorted[1].Name != "Beta" || sorted[2].Name != "Zebra" {
		t.Fatalf("unexpected sort order: %v", []string{sorted[0].Name, sorted[1].Name, sorted[2].Name})
	}
}

func TestSortByRunCount(t *testing.T) {
	m := model{
		scripts: []ScriptEntry{
			{Name: "a", RunCount: 1},
			{Name: "b", RunCount: 10},
			{Name: "c", RunCount: 5},
		},
		sortMode: sortByRunCount,
	}
	sorted := m.getSortedScripts()
	if sorted[0].Name != "b" || sorted[1].Name != "c" || sorted[2].Name != "a" {
		t.Fatalf("unexpected sort order: %v", []string{sorted[0].Name, sorted[1].Name, sorted[2].Name})
	}
}

// --- generateID ---

func TestGenerateIDUnique(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		id := generateID()
		if seen[id] {
			t.Fatalf("duplicate ID: %s", id)
		}
		seen[id] = true
		if len(id) != 8 {
			t.Fatalf("expected 8-char hex ID, got %q", id)
		}
	}
}

// --- expandPath ---

func TestExpandPathTilde(t *testing.T) {
	result := expandPath("~/projects")
	if strings.HasPrefix(result, "~") {
		t.Fatalf("tilde not expanded: %q", result)
	}
	if !strings.HasSuffix(result, "/projects") {
		t.Fatalf("path suffix wrong: %q", result)
	}
}

func TestExpandPathAbsolute(t *testing.T) {
	result := expandPath("/usr/bin")
	if result != "/usr/bin" {
		t.Fatalf("absolute path changed: %q", result)
	}
}

// --- formatElapsed ---

func TestFormatElapsedSeconds(t *testing.T) {
	got := formatElapsed(45e9)
	if got != "45s" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatElapsedMinutes(t *testing.T) {
	got := formatElapsed(125e9)
	if got != "2m 5s" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatElapsedHours(t *testing.T) {
	got := formatElapsed(3700e9)
	if got != "1h 1m" {
		t.Fatalf("got %q", got)
	}
}
