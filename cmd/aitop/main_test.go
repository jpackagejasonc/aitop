package main

import "testing"

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
