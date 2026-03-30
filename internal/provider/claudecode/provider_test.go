package claudecode

import (
	"math"
	"testing"
)

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestComputeCost_KnownModels(t *testing.T) {
	tests := []struct {
		model    string
		usage    usagePayload
		wantCost float64
	}{
		{
			model:    "claude-sonnet-4-6",
			usage:    usagePayload{InputTokens: 1_000_000, OutputTokens: 1_000_000},
			wantCost: 3.0 + 15.0,
		},
		{
			model:    "claude-opus-4-6",
			usage:    usagePayload{InputTokens: 1_000_000, OutputTokens: 1_000_000},
			wantCost: 15.0 + 75.0,
		},
		{
			model:    "claude-haiku-4-5",
			usage:    usagePayload{InputTokens: 1_000_000, OutputTokens: 1_000_000},
			wantCost: 0.80 + 4.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := computeCost(tt.model, &tt.usage)
			if !approxEqual(got, tt.wantCost) {
				t.Errorf("computeCost(%q): want %f, got %f", tt.model, tt.wantCost, got)
			}
		})
	}
}

func TestComputeCost_UnknownModelUsesDefault(t *testing.T) {
	u := usagePayload{InputTokens: 1_000_000}
	got := computeCost("some-future-model", &u)
	// Default pricing: $3/M input
	if !approxEqual(got, 3.0) {
		t.Errorf("want default cost 3.0, got %f", got)
	}
}

func TestComputeCost_CacheTokens(t *testing.T) {
	u := usagePayload{
		CacheCreationInputTokens: 1_000_000,
		CacheReadInputTokens:     1_000_000,
	}
	got := computeCost("claude-sonnet-4-6", &u)
	want := 3.75 + 0.30 // cache write + cache read for sonnet
	if !approxEqual(got, want) {
		t.Errorf("want %f, got %f", want, got)
	}
}

func TestComputeCost_ZeroUsage(t *testing.T) {
	u := usagePayload{}
	got := computeCost("claude-sonnet-4-6", &u)
	if got != 0 {
		t.Errorf("want 0 cost for zero usage, got %f", got)
	}
}

func TestComputeCost_PrefixMatching(t *testing.T) {
	// A model name with a version suffix should still match the prefix.
	u := usagePayload{InputTokens: 1_000_000}
	got := computeCost("claude-sonnet-4-6-20251022", &u)
	if !approxEqual(got, 3.0) {
		t.Errorf("prefix match failed: want 3.0, got %f", got)
	}
}
