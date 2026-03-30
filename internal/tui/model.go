package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jpackagejasonc/aitop/internal/aggregator"
)

// tickMsg is sent on each 1-second tick to refresh the display.
type tickMsg time.Time

// Model is the Bubble Tea application model for aitop.
type Model struct {
	agg          *aggregator.Aggregator
	snapshot     aggregator.Snapshot
	width        int
	height       int
	pricingStale bool
}

// New returns an initialised Model backed by the given Aggregator.
// pricingStale should be set to true when the cost pricing table may be outdated.
func New(agg *aggregator.Aggregator, pricingStale bool) Model {
	return Model{
		agg:          agg,
		snapshot:     agg.Snapshot(),
		pricingStale: pricingStale,
	}
}

// tick returns a command that fires a tickMsg after one second.
func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Init implements tea.Model. It starts the first tick.
func (m Model) Init() tea.Cmd {
	return tick()
}
