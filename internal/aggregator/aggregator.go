package aggregator

import (
	"sync"
	"time"

	"github.com/jpackagejasonc/aitop/internal/provider"
)

// WindowStats holds aggregate metrics for a rolling time window.
type WindowStats struct {
	Requests     int
	Errors       int
	InputTokens  int64
	OutputTokens int64
	CacheHits    int64
	Cost         float64
	ToolCalls    int
}

// Snapshot is a point-in-time view returned by Aggregator.Snapshot().
type Snapshot struct {
	Window1m        WindowStats
	Window5m        WindowStats
	Window15m       WindowStats
	Session         WindowStats
	StopReasons     map[string]int // session-level stop reason counts
	ContextTokens   int64          // total input tokens in the last request (context size estimate)
	CompactionCount int            // number of context compactions since aitop started
	ActiveRequests  int
	UpdatedAt       time.Time
}

// record is a single stored event with its ingestion timestamp.
type record struct {
	ts    time.Time
	event provider.Event
}

// sessionTimeout is how long a session can be idle before it is considered
// finished. Guards against sessions that never receive a Stop event (e.g.
// Claude Code crash or force-kill).
const sessionTimeout = 10 * time.Minute

// Aggregator accumulates provider events and provides windowed statistics.
type Aggregator struct {
	mu                sync.Mutex
	records           []record
	session           WindowStats
	stopReasons       map[string]int
	lastContextTokens int64          // total input tokens from the last completed request
	compactions       int            // number of PostCompact events received
	activeSessions    map[string]time.Time // session ID → last seen
}

// New returns an initialised Aggregator.
func New() *Aggregator {
	return &Aggregator{
		stopReasons:    make(map[string]int),
		activeSessions: make(map[string]time.Time),
	}
}

// Ingest processes an event, updating internal state accordingly.
func (a *Aggregator) Ingest(e provider.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()

	switch e.Type {
	case provider.EventRequestStart:
		if e.SessionID != "" {
			a.activeSessions[e.SessionID] = now
		}

	case provider.EventRequestEnd:
		delete(a.activeSessions, e.SessionID)
		a.records = append(a.records, record{ts: now, event: e})
		if e.Request != nil {
			a.session.Requests++
			a.session.InputTokens += int64(e.Request.InputTokens)
			a.session.OutputTokens += int64(e.Request.OutputTokens)
			a.session.CacheHits += int64(e.Request.CacheRead)
			a.session.Cost += e.Request.Cost
			if e.Request.StopReason != "" {
				a.stopReasons[e.Request.StopReason]++
			}
			// Track total input tokens for the most recent request as a
			// context window size estimate (input + cache_read + cache_write).
			a.lastContextTokens = int64(e.Request.InputTokens + e.Request.CacheRead + e.Request.CacheWrite)
		}

	case provider.EventSessionEnd:
		delete(a.activeSessions, e.SessionID)

	case provider.EventCompact:
		a.compactions++

	case provider.EventToolCall:
		// A tool call implies the session is active. Update last-seen time so
		// the session doesn't expire while work is in progress.
		if e.SessionID != "" {
			a.activeSessions[e.SessionID] = now
		}
		a.records = append(a.records, record{ts: now, event: e})
		a.session.ToolCalls++

	case provider.EventError:
		a.records = append(a.records, record{ts: now, event: e})
		a.session.Errors++
	}
}

// Snapshot returns a consistent point-in-time view of the aggregator state.
func (a *Aggregator) Snapshot() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	stopReasons := make(map[string]int, len(a.stopReasons))
	for k, v := range a.stopReasons {
		stopReasons[k] = v
	}
	return Snapshot{
		Window1m:        a.windowStats(now, 1*time.Minute),
		Window5m:        a.windowStats(now, 5*time.Minute),
		Window15m:       a.windowStats(now, 15*time.Minute),
		Session:         a.session,
		StopReasons:     stopReasons,
		ContextTokens:   a.lastContextTokens,
		CompactionCount: a.compactions,
		ActiveRequests:  len(a.activeSessions),
		UpdatedAt:       now,
	}
}

// Prune removes records older than maxAge and expires sessions that have been
// idle longer than sessionTimeout. Call periodically to bound memory.
func (a *Aggregator) Prune(maxAge time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	i := 0
	for i < len(a.records) && a.records[i].ts.Before(cutoff) {
		i++
	}
	a.records = a.records[i:]

	// Expire sessions that never sent a Stop event (e.g. Claude Code crash).
	idleCutoff := time.Now().Add(-sessionTimeout)
	for id, lastSeen := range a.activeSessions {
		if lastSeen.Before(idleCutoff) {
			delete(a.activeSessions, id)
		}
	}
}

// windowStats computes aggregate stats for events within duration d of now.
// Must be called with a.mu held.
func (a *Aggregator) windowStats(now time.Time, d time.Duration) WindowStats {
	cutoff := now.Add(-d)
	var ws WindowStats
	for _, r := range a.records {
		if r.ts.Before(cutoff) {
			continue
		}
		switch r.event.Type {
		case provider.EventRequestEnd:
			ws.Requests++
			if r.event.Request != nil {
				ws.InputTokens += int64(r.event.Request.InputTokens)
				ws.OutputTokens += int64(r.event.Request.OutputTokens)
				ws.CacheHits += int64(r.event.Request.CacheRead)
				ws.Cost += r.event.Request.Cost
			}
		case provider.EventToolCall:
			ws.ToolCalls++
		case provider.EventError:
			ws.Errors++
		}
	}
	return ws
}
