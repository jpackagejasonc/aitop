package tui

import (
	"strings"
	"testing"

	"github.com/jpackagejasonc/aitop/internal/aggregator"
)

func TestCheckAlerts_NoAlerts(t *testing.T) {
	s := aggregator.Snapshot{}
	if got := checkAlerts(s); len(got) != 0 {
		t.Errorf("expected no alerts for zero snapshot, got: %v", got)
	}
}

func TestCheckAlerts_HighBurnRate(t *testing.T) {
	s := aggregator.Snapshot{}
	// $0.10/min → $6/hr, above the $5/hr threshold.
	s.Window1m.Cost = 0.10
	alerts := checkAlerts(s)
	if !anyContains(alerts, "high burn rate") {
		t.Errorf("expected high burn rate alert, got: %v", alerts)
	}
}

func TestCheckAlerts_NoBurnRateAlertBelowThreshold(t *testing.T) {
	s := aggregator.Snapshot{}
	// $0.08/min → $4.80/hr, below the $5/hr threshold.
	s.Window1m.Cost = 0.08
	alerts := checkAlerts(s)
	if anyContains(alerts, "high burn rate") {
		t.Errorf("unexpected high burn rate alert at $4.80/hr, got: %v", alerts)
	}
}

func TestCheckAlerts_ContextNearFull(t *testing.T) {
	s := aggregator.Snapshot{}
	// 170K / 200K = 85%, above the 80% threshold.
	s.ContextTokens = 170_000
	alerts := checkAlerts(s)
	if !anyContains(alerts, "context window near full") {
		t.Errorf("expected context alert, got: %v", alerts)
	}
}

func TestCheckAlerts_NoContextAlertBelowThreshold(t *testing.T) {
	s := aggregator.Snapshot{}
	// 150K / 200K = 75%, below the 80% threshold.
	s.ContextTokens = 150_000
	alerts := checkAlerts(s)
	if anyContains(alerts, "context window near full") {
		t.Errorf("unexpected context alert at 75%%, got: %v", alerts)
	}
}

func TestCheckAlerts_Spinning(t *testing.T) {
	s := aggregator.Snapshot{}
	s.Window1m.Cost = 0.01
	s.Window1m.Requests = 1
	s.Window1m.OutputTokens = 0
	alerts := checkAlerts(s)
	if !anyContains(alerts, "tool loop") {
		t.Errorf("expected tool loop alert, got: %v", alerts)
	}
}

func TestCheckAlerts_NoSpinningAlertWithOutput(t *testing.T) {
	s := aggregator.Snapshot{}
	s.Window1m.Cost = 0.01
	s.Window1m.Requests = 1
	s.Window1m.OutputTokens = 500
	alerts := checkAlerts(s)
	if anyContains(alerts, "tool loop") {
		t.Errorf("unexpected tool loop alert when output tokens > 0, got: %v", alerts)
	}
}

func TestCheckAlerts_CacheEfficiencyLow(t *testing.T) {
	s := aggregator.Snapshot{}
	// 10 hits out of 100 total = 10%, below the 30% threshold.
	s.Window1m.CacheHits = 10
	s.Window1m.InputTokens = 90
	alerts := checkAlerts(s)
	if !anyContains(alerts, "cache efficiency low") {
		t.Errorf("expected cache efficiency alert, got: %v", alerts)
	}
}

func TestCheckAlerts_NoCacheAlertAboveThreshold(t *testing.T) {
	s := aggregator.Snapshot{}
	// 50 hits out of 100 total = 50%, above the 30% threshold.
	s.Window1m.CacheHits = 50
	s.Window1m.InputTokens = 50
	alerts := checkAlerts(s)
	if anyContains(alerts, "cache efficiency low") {
		t.Errorf("unexpected cache efficiency alert at 50%%, got: %v", alerts)
	}
}

func TestCheckAlerts_MultipleAlerts(t *testing.T) {
	s := aggregator.Snapshot{}
	s.Window1m.Cost = 0.10     // high burn rate
	s.ContextTokens = 170_000  // context near full
	alerts := checkAlerts(s)
	if len(alerts) < 2 {
		t.Errorf("expected at least 2 alerts, got %d: %v", len(alerts), alerts)
	}
}

// anyContains reports whether any string in ss contains substr.
func anyContains(ss []string, substr string) bool {
	for _, s := range ss {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}
