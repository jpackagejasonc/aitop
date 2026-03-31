package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	labelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	valueStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	accentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	borderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("237"))
)

const (
	labelColWidth   = 22 // wide enough for "output tokens" + padding
	minValColWidth  = 10
	defValColWidth  = 12 // used before first WindowSizeMsg
)

// layout holds pre-built column styles for the current terminal width.
// Recomputed on every View() call so resizes take effect immediately.
type layout struct {
	label    lipgloss.Style
	value    lipgloss.Style
	accent   lipgloss.Style
	errCol   lipgloss.Style
	errLabel lipgloss.Style
	sep      string
}

func newLayout(width int) layout {
	valWidth := defValColWidth
	if width > 0 {
		valWidth = (width - labelColWidth) / 3
		if valWidth < minValColWidth {
			valWidth = minValColWidth
		}
	}
	sepWidth := labelColWidth + 3*valWidth
	return layout{
		label:    labelStyle.Width(labelColWidth),
		value:    valueStyle.Width(valWidth),
		accent:   accentStyle.Width(valWidth),
		errCol:   errorStyle.Width(valWidth),
		errLabel: errorStyle.Width(labelColWidth),
		sep:      borderStyle.Render(strings.Repeat("─", sepWidth)),
	}
}

// View implements tea.Model.
func (m Model) View() tea.View {
	if m.width == 0 {
		return tea.NewView("loading...")
	}

	l := newLayout(m.width)
	s := m.snapshot
	var b strings.Builder

	// ── Header ──────────────────────────────────────────────────────────────
	ts := dimStyle.Render(s.UpdatedAt.Format("2006-01-02 15:04:05"))
	active := accentStyle.Render(fmt.Sprintf("active: %d", s.ActiveRequests))
	b.WriteString(fmt.Sprintf("%s  %s  %s\n", headerStyle.Render("aitop"), ts, active))
	b.WriteString("\n")

	// ── Rolling window table ─────────────────────────────────────────────────
	b.WriteString(l.label.Render("metric") +
		l.value.Render("1m") +
		l.value.Render("5m") +
		l.value.Render("15m") + "\n")
	b.WriteString(l.sep + "\n")

	b.WriteString(windowRow(l, "requests",
		s.Window1m.Requests, s.Window5m.Requests, s.Window15m.Requests))
	b.WriteString(windowRowRate(l, "req/min",
		float64(s.Window1m.Requests)/1,
		float64(s.Window5m.Requests)/5,
		float64(s.Window15m.Requests)/15))

	if s.Window1m.Errors > 0 || s.Window5m.Errors > 0 || s.Window15m.Errors > 0 {
		b.WriteString(l.errLabel.Render("errors") +
			l.errCol.Render(fmt.Sprintf("%d", s.Window1m.Errors)) +
			l.errCol.Render(fmt.Sprintf("%d", s.Window5m.Errors)) +
			l.errCol.Render(fmt.Sprintf("%d", s.Window15m.Errors)) + "\n")
	} else {
		b.WriteString(windowRow(l, "errors",
			s.Window1m.Errors, s.Window5m.Errors, s.Window15m.Errors))
	}
	b.WriteString(windowRowPct(l, "error rate",
		s.Window1m.Errors, s.Window1m.Requests,
		s.Window5m.Errors, s.Window5m.Requests,
		s.Window15m.Errors, s.Window15m.Requests))

	b.WriteString(windowRowInt64(l, "input tokens",
		s.Window1m.InputTokens, s.Window5m.InputTokens, s.Window15m.InputTokens))
	b.WriteString(windowRowInt64(l, "output tokens",
		s.Window1m.OutputTokens, s.Window5m.OutputTokens, s.Window15m.OutputTokens))
	b.WriteString(windowRowRate(l, "tok/min",
		float64(s.Window1m.InputTokens+s.Window1m.OutputTokens)/1,
		float64(s.Window5m.InputTokens+s.Window5m.OutputTokens)/5,
		float64(s.Window15m.InputTokens+s.Window15m.OutputTokens)/15))
	b.WriteString(windowRowInt64(l, "cache hits",
		s.Window1m.CacheHits, s.Window5m.CacheHits, s.Window15m.CacheHits))
	b.WriteString(windowRowCacheEff(l, "cache hit rate",
		s.Window1m.CacheHits, s.Window1m.InputTokens,
		s.Window5m.CacheHits, s.Window5m.InputTokens,
		s.Window15m.CacheHits, s.Window15m.InputTokens))
	b.WriteString(windowRowCost(l, "cost ($)",
		s.Window1m.Cost, s.Window5m.Cost, s.Window15m.Cost))
	b.WriteString(windowRowHourlyCost(l, "cost ($/hr)",
		s.Window1m.Cost*60, s.Window5m.Cost*12, s.Window15m.Cost*4))
	b.WriteString(windowRow(l, "tool calls",
		s.Window1m.ToolCalls, s.Window5m.ToolCalls, s.Window15m.ToolCalls))

	// ── Session summary ───────────────────────────────────────────────────────
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("session") + "\n")
	b.WriteString(l.sep + "\n")

	sess := s.Session
	b.WriteString(sessionRow(l, "total requests", valueStyle.Render(fmt.Sprintf("%d", sess.Requests))))
	b.WriteString(sessionRow(l, "total cost", accentStyle.Render(fmt.Sprintf("$%.4f", sess.Cost))))
	b.WriteString(sessionRow(l, "input tokens", valueStyle.Render(fmt.Sprintf("%d", sess.InputTokens))))
	b.WriteString(sessionRow(l, "output tokens", valueStyle.Render(fmt.Sprintf("%d", sess.OutputTokens))))
	b.WriteString(sessionRow(l, "cache hits", valueStyle.Render(fmt.Sprintf("%d", sess.CacheHits))))

	var hitRate float64
	if total := sess.InputTokens + sess.CacheHits; total > 0 {
		hitRate = float64(sess.CacheHits) / float64(total) * 100
	}
	b.WriteString(sessionRow(l, "cache hit rate", cacheEffStyle(l, hitRate).Render(fmt.Sprintf("%.1f%%", hitRate))))

	if len(s.StopReasons) > 0 {
		// Sort keys for deterministic output.
		keys := make([]string, 0, len(s.StopReasons))
		for k := range s.StopReasons {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, reason := range keys {
			count := s.StopReasons[reason]
			label := "stop: " + reason
			var val string
			if reason == "max_tokens" {
				val = accentStyle.Render(fmt.Sprintf("%d", count))
			} else {
				val = valueStyle.Render(fmt.Sprintf("%d", count))
			}
			b.WriteString(sessionRow(l, label, val))
		}
	}

	// Context window utilization — shown when we have at least one Stop event.
	// Uses total input tokens (non-cached + cache-read + cache-write) as a
	// proxy for current context size against the 200K limit.
	const contextLimit = 200_000
	if s.ContextTokens > 0 {
		pct := float64(s.ContextTokens) / float64(contextLimit) * 100
		ctxStr := fmt.Sprintf("%dK / %dK (%.0f%%)", s.ContextTokens/1000, contextLimit/1000, pct)
		var ctxStyle lipgloss.Style
		switch {
		case pct >= 85:
			ctxStyle = errorStyle
		case pct >= 60:
			ctxStyle = accentStyle
		default:
			ctxStyle = valueStyle
		}
		b.WriteString(sessionRow(l, "context window", ctxStyle.Render(ctxStr)))
	}
	if s.CompactionCount > 0 {
		b.WriteString(sessionRow(l, "compactions", valueStyle.Render(fmt.Sprintf("%d", s.CompactionCount))))
	}

	// ── Footer ────────────────────────────────────────────────────────────────
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("q: quit  •  providers: claude code"))
	b.WriteString("\n")
	if m.pricingStale {
		b.WriteString(accentStyle.Render("! cost estimates may be outdated — update pricingTable in internal/provider/claudecode/provider.go"))
		b.WriteString("\n")
	}

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func windowRow(l layout, label string, v1, v5, v15 int) string {
	return l.label.Render(label) +
		l.value.Render(fmt.Sprintf("%d", v1)) +
		l.value.Render(fmt.Sprintf("%d", v5)) +
		l.value.Render(fmt.Sprintf("%d", v15)) + "\n"
}

func windowRowInt64(l layout, label string, v1, v5, v15 int64) string {
	return l.label.Render(label) +
		l.value.Render(fmt.Sprintf("%d", v1)) +
		l.value.Render(fmt.Sprintf("%d", v5)) +
		l.value.Render(fmt.Sprintf("%d", v15)) + "\n"
}

func windowRowCost(l layout, label string, v1, v5, v15 float64) string {
	return l.label.Render(label) +
		l.accent.Render(fmt.Sprintf("$%.4f", v1)) +
		l.accent.Render(fmt.Sprintf("$%.4f", v5)) +
		l.accent.Render(fmt.Sprintf("$%.4f", v15)) + "\n"
}

// windowRowHourlyCost renders projected hourly cost extrapolated from each
// rolling window: 1m×60, 5m×12, 15m×4.
func windowRowHourlyCost(l layout, label string, v1, v5, v15 float64) string {
	f := func(v float64) string { return fmt.Sprintf("$%.2f/hr", v) }
	return l.label.Render(label) +
		l.accent.Render(f(v1)) +
		l.accent.Render(f(v5)) +
		l.accent.Render(f(v15)) + "\n"
}

// windowRowRate renders a row of per-minute rates (1 decimal place).
func windowRowRate(l layout, label string, v1, v5, v15 float64) string {
	f := func(v float64) string { return fmt.Sprintf("%.1f", v) }
	return l.label.Render(label) +
		l.value.Render(f(v1)) +
		l.value.Render(f(v5)) +
		l.value.Render(f(v15)) + "\n"
}

// windowRowPct renders a row of error-rate percentages. Shows "-" when there
// are no requests in a window to avoid division-by-zero and misleading 0.0%.
func windowRowPct(l layout, label string, e1, r1, e2, r2, e3, r3 int) string {
	f := func(errors, requests int) string {
		if requests == 0 {
			return "-"
		}
		pct := float64(errors) / float64(requests) * 100
		if pct > 100 {
			pct = 100
		}
		return fmt.Sprintf("%.1f%%", pct)
	}
	hasErrors := e1 > 0 || e2 > 0 || e3 > 0
	col := l.value
	if hasErrors {
		col = l.errCol
	}
	return l.label.Render(label) +
		col.Render(f(e1, r1)) +
		col.Render(f(e2, r2)) +
		col.Render(f(e3, r3)) + "\n"
}

// cacheEffStyle returns the appropriate style for a cache hit rate percentage.
// >= 60%: normal; >= 30%: accent (orange); < 30%: error (red).
func cacheEffStyle(l layout, pct float64) lipgloss.Style {
	switch {
	case pct >= 60:
		return l.value
	case pct >= 30:
		return l.accent
	default:
		return l.errCol
	}
}

// windowRowCacheEff renders a cache hit rate row for the rolling window table.
// Each window is coloured independently based on its own rate.
// Shows "-" when there is no token data in a window.
func windowRowCacheEff(l layout, label string, hits1, in1, hits5, in5, hits15, in15 int64) string {
	cell := func(hits, input int64) string {
		total := hits + input
		if total == 0 {
			return l.value.Render("-")
		}
		pct := float64(hits) / float64(total) * 100
		return cacheEffStyle(l, pct).Render(fmt.Sprintf("%.1f%%", pct))
	}
	return l.label.Render(label) +
		cell(hits1, in1) +
		cell(hits5, in5) +
		cell(hits15, in15) + "\n"
}

func sessionRow(l layout, label, value string) string {
	return "  " + l.label.Render(label) + value + "\n"
}
