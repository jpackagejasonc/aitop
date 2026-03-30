package provider

import "time"

// EventType identifies the kind of event emitted by a provider.
type EventType string

const (
	EventRequestStart EventType = "request_start"
	EventRequestEnd   EventType = "request_end"
	EventToolCall     EventType = "tool_call"
	EventToolResult   EventType = "tool_result"
	EventError        EventType = "error"
	EventSessionEnd   EventType = "session_end"
	EventCompact      EventType = "compact"
)

// RequestEvent carries token counts, timing and cost for a completed request.
type RequestEvent struct {
	InputTokens  int
	OutputTokens int
	CacheRead    int
	CacheWrite   int
	TTFT         time.Duration
	Duration     time.Duration
	Cost         float64
	StopReason   string
}

// ToolEvent carries information about a tool invocation.
type ToolEvent struct {
	Name     string
	Duration time.Duration
	Error    string
}

// ErrorEvent carries information about a provider-level error.
type ErrorEvent struct {
	Code    int
	Message string
}

// Event is the normalized event emitted by every provider.
type Event struct {
	Timestamp  time.Time
	ProviderID string
	Model      string
	SessionID  string
	Type       EventType
	Request    *RequestEvent
	Tool       *ToolEvent
	Error      *ErrorEvent
}

// Provider is implemented by every AI metrics source.
type Provider interface {
	// ID returns the unique machine-readable identifier of the provider.
	ID() string
	// Name returns the human-readable display name.
	Name() string
	// Connect starts the provider and returns a channel of events.
	Connect() (<-chan Event, error)
	// Disconnect stops the provider and cleans up resources.
	Disconnect() error
}
