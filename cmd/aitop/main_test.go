package main

import (
	"bytes"
	"os"
	"regexp"
	"testing"
)

// semverRE is the canonical regex from semver.org.
var semverRE = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
	`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?` +
	`(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

func captureStdout(f func()) string {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	f()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

func TestVersion_DefaultIsSemver(t *testing.T) {
	if !semverRE.MatchString(Version) {
		t.Errorf("default Version %q is not valid semver", Version)
	}
}

func TestRunVersion_Output(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })

	Version = "1.2.3"
	got := captureStdout(runVersion)
	if want := "aitop version 1.2.3\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRunVersion_PreRelease(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })

	Version = "2.0.0-alpha.1"
	got := captureStdout(runVersion)
	if want := "aitop version 2.0.0-alpha.1\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHasAitopHook_EmptySlice(t *testing.T) {
	if hasAitopHook(nil) {
		t.Error("nil slice: want false")
	}
	if hasAitopHook([]any{}) {
		t.Error("empty slice: want false")
	}
}

func TestHasAitopHook_Present(t *testing.T) {
	entries := []any{
		map[string]any{
			"hooks": []any{
				map[string]any{"type": "command", "command": "aitop hook"},
			},
		},
	}
	if !hasAitopHook(entries) {
		t.Error("want true when aitop hook is present")
	}
}

func TestHasAitopHook_Absent(t *testing.T) {
	entries := []any{
		map[string]any{
			"hooks": []any{
				map[string]any{"type": "command", "command": "some-other-hook"},
			},
		},
	}
	if hasAitopHook(entries) {
		t.Error("want false when only other hooks are present")
	}
}

func TestHasAitopHook_MultipleHooksOnlyOneMatches(t *testing.T) {
	entries := []any{
		map[string]any{
			"hooks": []any{
				map[string]any{"type": "command", "command": "other-hook"},
				map[string]any{"type": "command", "command": "aitop hook"},
			},
		},
	}
	if !hasAitopHook(entries) {
		t.Error("want true when aitop hook is one of several hooks")
	}
}

func TestHasAitopHook_MultipleEntriesOnlyOneMatches(t *testing.T) {
	entries := []any{
		map[string]any{
			"hooks": []any{
				map[string]any{"type": "command", "command": "other-hook"},
			},
		},
		map[string]any{
			"hooks": []any{
				map[string]any{"type": "command", "command": "aitop hook"},
			},
		},
	}
	if !hasAitopHook(entries) {
		t.Error("want true when aitop hook is in second entry")
	}
}

func TestHasAitopHook_MalformedEntriesIgnored(t *testing.T) {
	entries := []any{
		"not a map",
		42,
		map[string]any{"hooks": "not a slice"},
		map[string]any{"hooks": []any{"not a map"}},
	}
	if hasAitopHook(entries) {
		t.Error("want false for malformed entries")
	}
}
