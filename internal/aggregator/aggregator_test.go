package aggregator

import (
	"math"
	"testing"
	"time"

	"github.com/jpackagejasonc/aitop/internal/provider"
)

// ---- helpers ----------------------------------------------------------------

func reqEnd(sessionID string, in, out, cacheRead int, cost float64) provider.Event {
	return provider.Event{
		Type:      provider.EventRequestEnd,
		SessionID: sessionID,
		Request: &provider.RequestEvent{
			InputTokens:  in,
			OutputTokens: out,
			CacheRead:    cacheRead,
			Cost:         cost,
		},
	}
}

func reqEndWithStop(sessionID, stopReason string) provider.Event {
	return provider.Event{
		Type:      provider.EventRequestEnd,
		SessionID: sessionID,
		Request:   &provider.RequestEvent{StopReason: stopReason},
	}
}

func toolCall(sessionID, name string) provider.Event {
	return provider.Event{
		Type:      provider.EventToolCall,
		SessionID: sessionID,
		Tool:      &provider.ToolEvent{Name: name},
	}
}

func errEvent(sessionID string) provider.Event {
	return provider.Event{
		Type:      provider.EventError,
		SessionID: sessionID,
		Error:     &provider.ErrorEvent{Code: 500, Message: "oops"},
	}
}

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// insertAt bypasses Ingest's time.Now() so tests can control record timestamps.
func insertAt(a *Aggregator, ts time.Time, e provider.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.records = append(a.records, record{ts: ts, event: e})
}

// ---- Snapshot on empty aggregator -------------------------------------------

func TestSnapshot_Empty(t *testing.T) {
	a := New()
	s := a.Snapshot()

	if s.ActiveRequests != 0 {
		t.Errorf("ActiveRequests: want 0, got %d", s.ActiveRequests)
	}
	if s.Session.Requests != 0 {
		t.Errorf("Session.Requests: want 0, got %d", s.Session.Requests)
	}
	if s.Window1m.Requests != 0 {
		t.Errorf("Window1m.Requests: want 0, got %d", s.Window1m.Requests)
	}
}

// ---- Session totals ---------------------------------------------------------

func TestSession_Totals(t *testing.T) {
	a := New()

	a.Ingest(reqEnd("s1", 100, 50, 20, 0.001))
	a.Ingest(reqEnd("s1", 200, 75, 30, 0.002))
	a.Ingest(toolCall("s1", "Read"))
	a.Ingest(toolCall("s1", "Write"))
	a.Ingest(errEvent("s1"))

	s := a.Snapshot().Session

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"Requests", s.Requests, 2},
		{"InputTokens", int(s.InputTokens), 300},
		{"OutputTokens", int(s.OutputTokens), 125},
		{"CacheHits", int(s.CacheHits), 50},
		{"ToolCalls", s.ToolCalls, 2},
		{"Errors", s.Errors, 1},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("Session.%s: want %v, got %v", tt.name, tt.want, tt.got)
		}
	}
	if !approxEqual(s.Cost, 0.003) {
		t.Errorf("Session.Cost: want 0.003, got %f", s.Cost)
	}
}

// ---- Window boundary --------------------------------------------------------

func TestWindow_ExcludesOldRecords(t *testing.T) {
	a := New()
	now := time.Now()

	// Outside the 1m window, inside the 5m window.
	insertAt(a, now.Add(-2*time.Minute), reqEnd("s1", 100, 0, 0, 0.001))
	// Inside both windows.
	insertAt(a, now.Add(-30*time.Second), reqEnd("s2", 200, 0, 0, 0.002))

	s := a.Snapshot()

	if s.Window1m.Requests != 1 {
		t.Errorf("Window1m.Requests: want 1, got %d", s.Window1m.Requests)
	}
	if s.Window1m.InputTokens != 200 {
		t.Errorf("Window1m.InputTokens: want 200, got %d", s.Window1m.InputTokens)
	}
	if s.Window5m.Requests != 2 {
		t.Errorf("Window5m.Requests: want 2, got %d", s.Window5m.Requests)
	}
	if s.Window5m.InputTokens != 300 {
		t.Errorf("Window5m.InputTokens: want 300, got %d", s.Window5m.InputTokens)
	}
}

func TestWindow_ToolCallsAndErrors(t *testing.T) {
	a := New()
	now := time.Now()

	insertAt(a, now.Add(-30*time.Second), toolCall("s1", "Read"))
	insertAt(a, now.Add(-30*time.Second), toolCall("s1", "Write"))
	insertAt(a, now.Add(-30*time.Second), errEvent("s1"))
	// Outside 1m window.
	insertAt(a, now.Add(-2*time.Minute), toolCall("s1", "Bash"))

	s := a.Snapshot()

	if s.Window1m.ToolCalls != 2 {
		t.Errorf("Window1m.ToolCalls: want 2, got %d", s.Window1m.ToolCalls)
	}
	if s.Window1m.Errors != 1 {
		t.Errorf("Window1m.Errors: want 1, got %d", s.Window1m.Errors)
	}
	if s.Window5m.ToolCalls != 3 {
		t.Errorf("Window5m.ToolCalls: want 3, got %d", s.Window5m.ToolCalls)
	}
}

func TestWindow_CostAccumulation(t *testing.T) {
	a := New()
	now := time.Now()

	insertAt(a, now.Add(-30*time.Second), reqEnd("s1", 0, 0, 0, 0.005))
	insertAt(a, now.Add(-30*time.Second), reqEnd("s2", 0, 0, 0, 0.003))

	s := a.Snapshot()

	if !approxEqual(s.Window1m.Cost, 0.008) {
		t.Errorf("Window1m.Cost: want 0.008, got %f", s.Window1m.Cost)
	}
}

// ---- Active session tracking ------------------------------------------------

func TestActiveSession_AddedOnToolCall(t *testing.T) {
	a := New()
	a.Ingest(toolCall("sess-1", "Read"))

	if a.Snapshot().ActiveRequests != 1 {
		t.Errorf("ActiveRequests: want 1 after tool call")
	}
}

func TestActiveSession_RemovedOnStop(t *testing.T) {
	a := New()
	a.Ingest(toolCall("sess-1", "Read"))
	a.Ingest(reqEnd("sess-1", 0, 0, 0, 0))

	if a.Snapshot().ActiveRequests != 0 {
		t.Errorf("ActiveRequests: want 0 after stop")
	}
}

func TestActiveSession_MultipleSessions(t *testing.T) {
	a := New()
	a.Ingest(toolCall("s1", "Read"))
	a.Ingest(toolCall("s2", "Write"))
	a.Ingest(toolCall("s1", "Bash")) // second tool for s1 — should not inflate count

	if a.Snapshot().ActiveRequests != 2 {
		t.Errorf("ActiveRequests: want 2, got %d", a.Snapshot().ActiveRequests)
	}

	a.Ingest(reqEnd("s1", 0, 0, 0, 0))

	if a.Snapshot().ActiveRequests != 1 {
		t.Errorf("ActiveRequests after s1 stop: want 1, got %d", a.Snapshot().ActiveRequests)
	}
}

func TestActiveSession_EmptySessionIDIgnored(t *testing.T) {
	a := New()
	e := toolCall("", "Read") // no session ID
	a.Ingest(e)

	if a.Snapshot().ActiveRequests != 0 {
		t.Errorf("ActiveRequests: want 0 for empty session ID, got %d", a.Snapshot().ActiveRequests)
	}
}

// ---- Prune ------------------------------------------------------------------

func TestPrune_RemovesOldRecords(t *testing.T) {
	a := New()
	now := time.Now()

	a.records = []record{
		{ts: now.Add(-40 * time.Minute), event: reqEnd("s1", 0, 0, 0, 0)},
		{ts: now.Add(-10 * time.Minute), event: reqEnd("s2", 0, 0, 0, 0)},
		{ts: now.Add(-1 * time.Minute), event: reqEnd("s3", 0, 0, 0, 0)},
	}

	a.Prune(30 * time.Minute)

	if len(a.records) != 2 {
		t.Errorf("records after prune: want 2, got %d", len(a.records))
	}
}

func TestPrune_ExpiresIdleSessions(t *testing.T) {
	a := New()
	now := time.Now()

	a.activeSessions["stale"] = now.Add(-20 * time.Minute)
	a.activeSessions["recent"] = now.Add(-1 * time.Minute)

	a.Prune(30 * time.Minute)

	if _, ok := a.activeSessions["stale"]; ok {
		t.Error("stale session should have been expired")
	}
	if _, ok := a.activeSessions["recent"]; !ok {
		t.Error("recent session should still be active")
	}
	if a.Snapshot().ActiveRequests != 1 {
		t.Errorf("ActiveRequests after expiry: want 1, got %d", a.Snapshot().ActiveRequests)
	}
}

func TestPrune_EmptyAggregator(t *testing.T) {
	a := New()
	// Must not panic on empty state.
	a.Prune(30 * time.Minute)
}

// ---- Stop reason tracking ---------------------------------------------------

func TestStopReasons_CountedInSnapshot(t *testing.T) {
	a := New()
	a.Ingest(reqEndWithStop("s1", "end_turn"))
	a.Ingest(reqEndWithStop("s1", "end_turn"))
	a.Ingest(reqEndWithStop("s2", "max_tokens"))

	sr := a.Snapshot().StopReasons
	if sr["end_turn"] != 2 {
		t.Errorf("end_turn: want 2, got %d", sr["end_turn"])
	}
	if sr["max_tokens"] != 1 {
		t.Errorf("max_tokens: want 1, got %d", sr["max_tokens"])
	}
}

func TestStopReasons_EmptyReasonIgnored(t *testing.T) {
	a := New()
	a.Ingest(reqEnd("s1", 0, 0, 0, 0)) // no StopReason set

	sr := a.Snapshot().StopReasons
	if len(sr) != 0 {
		t.Errorf("StopReasons: want empty map, got %v", sr)
	}
}

func TestStopReasons_SnapshotIsCopy(t *testing.T) {
	a := New()
	a.Ingest(reqEndWithStop("s1", "end_turn"))

	s := a.Snapshot()
	// Mutating the snapshot copy must not affect the aggregator.
	s.StopReasons["end_turn"] = 999

	if a.Snapshot().StopReasons["end_turn"] != 1 {
		t.Error("mutating snapshot StopReasons should not affect aggregator")
	}
}
