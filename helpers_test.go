package main

import (
	"strings"
	"testing"
)

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
