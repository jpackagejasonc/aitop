package claudecode

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jpackagejasonc/aitop/internal/provider"
)

const (
	// maxPayloadBytes caps the size of a single hook payload to prevent memory exhaustion.
	maxPayloadBytes = 1 << 20 // 1MB

	// maxTokensPerField is the per-field ceiling on token counts. Current max
	// context windows are well under 2M tokens; anything above is implausible.
	maxTokensPerField = 2_000_000

	// pricingAsOf is the date the pricing table was last verified against Anthropic's
	// published rates. A warning is logged at startup if it is more than 90 days old.
	pricingAsOf = "2026-03-29"
)

// validHookEvents is the set of Claude Code hook event names we accept.
var validHookEvents = map[string]bool{
	"PreToolUse":         true,
	"PostToolUse":        true,
	"PostToolUseFailure": true,
	"Stop":               true,
	"StopFailure":        true,
	"SubagentStop":       true,
	"SessionStart":       true,
	"SessionEnd":         true,
	"UserPromptSubmit":   true,
	"PreCompact":         true,
	"PostCompact":        true,
}

// SocketPath returns the path of the Unix domain socket for this provider.
// The socket lives in ~/.aitop/ so it is only accessible by the current user.
func SocketPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Error("claudecode: cannot determine home directory, using /tmp fallback", "err", err)
		return "/tmp/aitop-claudecode.sock"
	}
	return filepath.Join(home, ".aitop", "claudecode.sock")
}

// hookPayload is the JSON structure sent by Claude Code hooks via stdin.
type hookPayload struct {
	SessionID           string          `json:"session_id"`
	TranscriptPath      string          `json:"transcript_path"`
	AgentTranscriptPath string          `json:"agent_transcript_path"` // SubagentStop only
	HookEventName       string          `json:"hook_event_name"`
	ToolName            string          `json:"tool_name"`
	ToolInput           json.RawMessage `json:"tool_input"`
	ToolResponse        json.RawMessage `json:"tool_response"`
}

// transcriptEntry is a minimal representation of a JSONL line in the Claude
// Code session transcript. Only the fields we need for metrics are decoded.
type transcriptEntry struct {
	Type    string          `json:"type"`
	Message *transcriptMsg  `json:"message"`
}

// transcriptMsg holds the fields we need from an assistant message.
// Usage field names match usagePayload so the same struct is reused.
type transcriptMsg struct {
	Model      string        `json:"model"`
	StopReason string        `json:"stop_reason"`
	Usage      *usagePayload `json:"usage"`
}

// usagePayload contains the token usage breakdown from the Stop hook.
type usagePayload struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// pricing holds per-million-token prices for a model family.
type pricing struct {
	in, out, cacheWrite, cacheRead float64
}

var pricingTable = []struct {
	prefix string
	p      pricing
}{
	{"claude-opus-4-6", pricing{in: 15, out: 75, cacheWrite: 18.75, cacheRead: 1.50}},
	{"claude-sonnet-4-6", pricing{in: 3, out: 15, cacheWrite: 3.75, cacheRead: 0.30}},
	{"claude-haiku-4-5", pricing{in: 0.80, out: 4, cacheWrite: 1.00, cacheRead: 0.08}},
}

var defaultPricing = pricing{in: 3, out: 15, cacheWrite: 3.75, cacheRead: 0.30}

// PricingStale returns true if the pricing table was last verified more than
// 90 days ago. Callers can use this to surface a warning in the UI.
func PricingStale() bool {
	asOf, err := time.Parse("2006-01-02", pricingAsOf)
	if err != nil {
		return false
	}
	return time.Since(asOf) > 90*24*time.Hour
}

// checkPricingStaleness logs a warning once if the pricing table is stale.
var warnStalePricing sync.Once

func checkPricingStaleness() {
	warnStalePricing.Do(func() {
		if PricingStale() {
			slog.Warn("claudecode: pricing table may be outdated",
				"pricing_as_of", pricingAsOf,
				"hint", "update pricingTable in internal/provider/claudecode/provider.go")
		}
	})
}

// Provider is the Claude Code provider. It listens on a Unix socket for hook
// events forwarded by the `aitop hook` subcommand.
type Provider struct {
	mu       sync.Mutex
	listener net.Listener
	events   chan provider.Event
	done     chan struct{}
}

// New returns a new Claude Code Provider.
func New() *Provider {
	return &Provider{
		events: make(chan provider.Event, 256),
		done:   make(chan struct{}),
	}
}

// ID implements provider.Provider.
func (p *Provider) ID() string { return "claudecode" }

// Name implements provider.Provider.
func (p *Provider) Name() string { return "Claude Code" }

// Connect starts the Unix socket listener and returns the event channel.
// The socket is created in ~/.aitop/ with 0600 permissions.
func (p *Provider) Connect() (<-chan provider.Event, error) {
	checkPricingStaleness()

	socketPath := SocketPath()
	dir := filepath.Dir(socketPath)

	// Ensure ~/.aitop/ exists and is only accessible by the current user.
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	// Remove stale socket file if present.
	_ = os.Remove(socketPath)

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}

	// Restrict socket to owner-only access.
	if err := os.Chmod(socketPath, 0600); err != nil {
		_ = ln.Close()
		return nil, err
	}

	p.mu.Lock()
	p.listener = ln
	p.mu.Unlock()

	slog.Info("claudecode: listening", "socket", socketPath)
	go p.accept(ln)
	return p.events, nil
}

// Disconnect closes the listener and signals the provider to stop.
func (p *Provider) Disconnect() error {
	p.mu.Lock()
	ln := p.listener
	p.mu.Unlock()

	close(p.done)
	if ln != nil {
		return ln.Close()
	}
	return nil
}

// accept loops accepting new connections until the listener is closed.
func (p *Provider) accept(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-p.done:
				return
			default:
				slog.Error("claudecode: accept error", "err", err)
				continue
			}
		}
		go p.handle(conn.(*net.UnixConn))
	}
}

// handle reads a single JSON hook payload from conn, validates it, maps it to
// a normalized provider.Event, and publishes it to the event channel.
func (p *Provider) handle(conn *net.UnixConn) {
	defer conn.Close()

	// Fix 3: verify the connecting process belongs to the current user.
	if !verifyPeer(conn) {
		slog.Warn("claudecode: rejected connection from non-owner process")
		return
	}

	// Fix 4: cap payload size to prevent memory exhaustion.
	lr := io.LimitReader(conn, maxPayloadBytes)
	raw, err := io.ReadAll(lr)
	if err != nil {
		slog.Warn("claudecode: failed to read hook payload")
		return
	}
	slog.Debug("claudecode: raw payload", "bytes", string(raw))
	var payload hookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		slog.Warn("claudecode: failed to decode hook payload")
		slog.Debug("claudecode: decode error detail", "err", err)
		return
	}

	// Fix 4: whitelist hook event names.
	slog.Debug("claudecode: decoded payload",
		"event", payload.HookEventName,
		"session", payload.SessionID,
		"transcript", payload.TranscriptPath)
	if !validHookEvents[payload.HookEventName] {
		slog.Debug("claudecode: ignoring unknown hook event", "event", payload.HookEventName)
		return
	}

	// Fix 4: basic field validation.
	if len(payload.SessionID) > 128 {
		slog.Warn("claudecode: oversized session_id, rejecting payload")
		return
	}

	now := time.Now()
	e := provider.Event{
		Timestamp:  now,
		ProviderID: p.ID(),
		SessionID:  payload.SessionID,
	}

	switch payload.HookEventName {
	case "PreToolUse":
		// Validated and accepted but not emitted — counting happens on
		// PostToolUse so each tool call is counted exactly once.
		return

	case "SessionStart", "UserPromptSubmit":
		e.Type = provider.EventRequestStart

	case "SessionEnd":
		e.Type = provider.EventSessionEnd

	case "PostToolUse":
		e.Type = provider.EventToolCall
		e.Tool = &provider.ToolEvent{Name: payload.ToolName}

	case "PostToolUseFailure":
		e.Type = provider.EventError
		e.Error = &provider.ErrorEvent{Message: "tool execution failed: " + payload.ToolName}

	case "PreCompact":
		// PreCompact fires when compaction is imminent. No-op: context size
		// (tracked via Stop transcript data) already serves as the approaching
		// indicator; the count is incremented on PostCompact when it completes.
		return

	case "PostCompact":
		e.Type = provider.EventCompact

	case "StopFailure":
		e.Type = provider.EventError
		e.Error = &provider.ErrorEvent{Message: "turn ended with API error"}

	case "Stop", "SubagentStop":
		e.Type = provider.EventRequestEnd
		re := &provider.RequestEvent{}
		// SubagentStop carries the subagent's own transcript; Stop uses the
		// session transcript. Both contain usage in the last assistant message.
		transcriptPath := payload.TranscriptPath
		if payload.HookEventName == "SubagentStop" && payload.AgentTranscriptPath != "" {
			transcriptPath = payload.AgentTranscriptPath
		}
		if transcriptPath != "" {
			model, stopReason, usage := readTranscriptUsage(transcriptPath)
			e.Model = model
			re.StopReason = stopReason
			if usage != nil {
				if !validUsage(usage) {
					slog.Warn("claudecode: transcript usage fields out of bounds, dropping usage data",
						"session", payload.SessionID)
				} else {
					re.InputTokens = usage.InputTokens
					re.OutputTokens = usage.OutputTokens
					re.CacheWrite = usage.CacheCreationInputTokens
					re.CacheRead = usage.CacheReadInputTokens
					re.Cost = computeCost(model, usage)
				}
			}
		}
		e.Request = re
	}

	select {
	case p.events <- e:
	default:
		slog.Warn("claudecode: event channel full, dropping event",
			"type", string(e.Type), "session", e.SessionID)
	}
}

// readTranscriptUsage opens the Claude Code session transcript at path, scans
// it for the last assistant message, and returns the model, stop reason, and
// usage fields. Returns zero values if the file cannot be read or parsed.
// The transcript is capped at 32 MB to bound memory use on very long sessions.
func readTranscriptUsage(path string) (model, stopReason string, u *usagePayload) {
	if !filepath.IsAbs(path) {
		slog.Warn("claudecode: transcript path is not absolute, skipping", "path", path)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		slog.Debug("claudecode: could not open transcript", "path", path, "err", err)
		return
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, 32<<20))
	if err != nil {
		slog.Debug("claudecode: could not read transcript", "path", path, "err", err)
		return
	}

	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var entry transcriptEntry
		if json.Unmarshal(line, &entry) != nil || entry.Type != "assistant" || entry.Message == nil {
			continue
		}
		model = entry.Message.Model
		stopReason = entry.Message.StopReason
		u = entry.Message.Usage
	}
	return
}

// validUsage returns false if any token count is negative or exceeds the
// per-field ceiling, indicating a malformed or malicious payload.
func validUsage(u *usagePayload) bool {
	for _, v := range []int{
		u.InputTokens,
		u.OutputTokens,
		u.CacheCreationInputTokens,
		u.CacheReadInputTokens,
	} {
		if v < 0 || v > maxTokensPerField {
			return false
		}
	}
	return true
}

// computeCost calculates the USD cost for a usage payload given the model name.
func computeCost(model string, u *usagePayload) float64 {
	pr := defaultPricing
	for _, entry := range pricingTable {
		if strings.HasPrefix(model, entry.prefix) {
			pr = entry.p
			break
		}
	}
	return float64(u.InputTokens)/1e6*pr.in +
		float64(u.OutputTokens)/1e6*pr.out +
		float64(u.CacheCreationInputTokens)/1e6*pr.cacheWrite +
		float64(u.CacheReadInputTokens)/1e6*pr.cacheRead
}
