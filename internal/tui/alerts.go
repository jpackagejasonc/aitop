package tui

import (
	"fmt"

	"github.com/jpackagejasonc/aitop/internal/aggregator"
)

// Alert thresholds.
const (
	alertBurnRateThreshold   = 5.0 // $/hr — based on 1m window
	alertContextPctThreshold = 80.0
	alertCacheEffThreshold   = 30.0 // % — based on 1m window
)

// checkAlerts returns a list of human-readable alert messages for conditions
// that warrant immediate attention. Returns nil when everything looks normal.
func checkAlerts(s aggregator.Snapshot) []string {
	var alerts []string

	// High burn rate — extrapolate the most recent 1m window to $/hr.
	if burnRate := s.Window1m.Cost * 60; burnRate > alertBurnRateThreshold {
		alerts = append(alerts, fmt.Sprintf("high burn rate ($%.2f/hr) — velocity based on last 1m", burnRate))
	}

	// Context window near full.
	if s.ContextTokens > 0 {
		pct := float64(s.ContextTokens) / float64(contextLimit) * 100
		if pct >= alertContextPctThreshold {
			alerts = append(alerts, fmt.Sprintf("context window near full (%.0f%%)", pct))
		}
	}

	// Spending with no output tokens — likely a tool loop consuming input
	// tokens without producing any response text.
	if s.Window1m.Cost > 0 && s.Window1m.OutputTokens == 0 && s.Window1m.Requests > 0 {
		alerts = append(alerts, "spending with no output tokens — possible tool loop")
	}

	// Cache efficiency collapsed in the most recent window.
	if total := s.Window1m.CacheHits + s.Window1m.InputTokens; total > 0 {
		rate := float64(s.Window1m.CacheHits) / float64(total) * 100
		if rate < alertCacheEffThreshold {
			alerts = append(alerts, fmt.Sprintf("cache efficiency low (%.1f%%) — context may have been reset", rate))
		}
	}

	return alerts
}
