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
