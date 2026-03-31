package tui

import (
	"strings"
	"testing"

	"github.com/jpackagejasonc/aitop/internal/aggregator"
)

func TestNewLayout_DefaultWhenZeroWidth(t *testing.T) {
	l := newLayout(0)
	if l.value.GetWidth() != defValColWidth {
		t.Errorf("zero width: value col want %d, got %d", defValColWidth, l.value.GetWidth())
	}
	if l.label.GetWidth() != labelColWidth {
		t.Errorf("zero width: label col want %d, got %d", labelColWidth, l.label.GetWidth())
	}
}

func TestNewLayout_ValueColumnsScaleWithWidth(t *testing.T) {
	narrow := newLayout(60)
	wide := newLayout(160)

	if wide.value.GetWidth() <= narrow.value.GetWidth() {
		t.Errorf("wide terminal should produce larger value columns: narrow=%d wide=%d",
			narrow.value.GetWidth(), wide.value.GetWidth())
	}
}

func TestNewLayout_FloorAtMinValColWidth(t *testing.T) {
	// A very narrow terminal should not go below the minimum.
	l := newLayout(10)
	if l.value.GetWidth() < minValColWidth {
		t.Errorf("value col below minimum: want >=%d, got %d", minValColWidth, l.value.GetWidth())
	}
}

func TestNewLayout_LabelWidthAlwaysFixed(t *testing.T) {
	for _, w := range []int{0, 40, 80, 200} {
		l := newLayout(w)
		if l.label.GetWidth() != labelColWidth {
			t.Errorf("width=%d: label col want %d, got %d", w, labelColWidth, l.label.GetWidth())
		}
	}
}

func TestNewLayout_SeparatorWidthMatchesTable(t *testing.T) {
	for _, termWidth := range []int{80, 120, 200} {
		l := newLayout(termWidth)
		wantSep := labelColWidth + 3*l.value.GetWidth()
		// Strip ANSI codes to get the visible rune count.
		// lipgloss.Width returns the visible width of a rendered string.
		gotSep := lipglossVisibleWidth(l.sep)
		if gotSep != wantSep {
			t.Errorf("termWidth=%d: sep width want %d, got %d", termWidth, wantSep, gotSep)
		}
	}
}

func TestView_PricingStaleWarningShown(t *testing.T) {
	m := Model{
		agg:          aggregator.New(),
		width:        80,
		pricingStale: true,
	}
	m.snapshot = m.agg.Snapshot()

	out := m.View().Content
	if !strings.Contains(out, "cost estimates may be outdated") {
		t.Error("expected pricing stale warning in view output")
	}
}

func TestView_PricingStaleWarningHidden(t *testing.T) {
	m := Model{
		agg:          aggregator.New(),
		width:        80,
		pricingStale: false,
	}
	m.snapshot = m.agg.Snapshot()

	out := m.View().Content
	if strings.Contains(out, "cost estimates may be outdated") {
		t.Error("expected no pricing stale warning when pricing is current")
	}
}

func TestWindowRowCacheEff_NoData(t *testing.T) {
	l := newLayout(80)
	row := windowRowCacheEff(l, "cache hit rate", 0, 0, 0, 0, 0, 0)
	if strings.Count(row, "-") < 3 {
		t.Errorf("expected three '-' placeholders when all windows are empty, got: %q", row)
	}
}

func TestWindowRowCacheEff_Values(t *testing.T) {
	l := newLayout(80)
	// 80 hits out of 100 total = 80%
	row := windowRowCacheEff(l, "cache hit rate", 80, 20, 80, 20, 80, 20)
	if !strings.Contains(row, "80.0%") {
		t.Errorf("expected 80.0%% in row, got: %q", row)
	}
}

func TestWindowRowCacheEff_MixedWindows(t *testing.T) {
	l := newLayout(80)
	// 1m: 80% (good), 5m: 40% (warning), 15m: 10% (bad)
	row := windowRowCacheEff(l, "cache hit rate", 80, 20, 40, 60, 10, 90)
	if !strings.Contains(row, "80.0%") {
		t.Errorf("expected 80.0%% for 1m window, got: %q", row)
	}
	if !strings.Contains(row, "40.0%") {
		t.Errorf("expected 40.0%% for 5m window, got: %q", row)
	}
	if !strings.Contains(row, "10.0%") {
		t.Errorf("expected 10.0%% for 15m window, got: %q", row)
	}
}

func TestWindowRowHourlyCost_Format(t *testing.T) {
	l := newLayout(80)
	// $0.01 per minute → $0.60/hr
	row := windowRowHourlyCost(l, "cost ($/hr)", 0.60, 0.60, 0.60)
	if !strings.Contains(row, "$0.60/hr") {
		t.Errorf("expected $0.60/hr in row, got: %q", row)
	}
}

func TestWindowRowHourlyCost_Extrapolation(t *testing.T) {
	l := newLayout(80)
	// Different rates per window.
	row := windowRowHourlyCost(l, "cost ($/hr)", 1.20, 0.60, 0.30)
	for _, want := range []string{"$1.20/hr", "$0.60/hr", "$0.30/hr"} {
		if !strings.Contains(row, want) {
			t.Errorf("expected %s in row, got: %q", want, row)
		}
	}
}

func TestView_CacheHitRateRowPresent(t *testing.T) {
	m := Model{
		agg:   aggregator.New(),
		width: 80,
	}
	m.snapshot = m.agg.Snapshot()
	out := m.View().Content
	if !strings.Contains(out, "cache hit rate") {
		t.Error("expected 'cache hit rate' row in view output")
	}
}

func TestView_HourlyCostRowPresent(t *testing.T) {
	m := Model{
		agg:   aggregator.New(),
		width: 80,
	}
	m.snapshot = m.agg.Snapshot()
	out := m.View().Content
	if !strings.Contains(out, "cost ($/hr)") {
		t.Error("expected 'cost ($/hr)' row in view output")
	}
}

// lipglossVisibleWidth counts the visible runes in a string by stripping ANSI
// escape sequences. We use a simple count here since lipgloss is already
// handling the actual width math; we just want to verify it matches expectations.
func lipglossVisibleWidth(s string) int {
	count := 0
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		count++
	}
	return count
}
