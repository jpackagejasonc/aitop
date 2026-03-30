package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jpackagejasonc/aitop/internal/aggregator"
	"github.com/jpackagejasonc/aitop/internal/bus"
	"github.com/jpackagejasonc/aitop/internal/logger"
	"github.com/jpackagejasonc/aitop/internal/provider/claudecode"
	"github.com/jpackagejasonc/aitop/internal/tui"
)

func main() {
	// Set up file-based logging before anything else so all subcommands log to
	// ~/.aitop/aitop.log. Errors here are non-fatal; slog falls back to stderr.
	if err := logger.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "aitop: warning: could not init log file: %v\n", err)
	}

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			runVersion()
			return
		case "hook":
			runHook()
			return
		case "install":
			runInstall()
			return
		}
	}
	runTUI()
}

// runHook reads JSON from stdin and forwards it to the Claude Code socket.
// Errors are logged to file but never written to stderr or stdout — this
// subcommand must never block or produce output that disrupts Claude Code.
func runHook() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		slog.Error("hook: failed to read stdin", "err", err)
		return
	}

	socketPath := claudecode.SocketPath()
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		// aitop is not running — expected when the TUI isn't open.
		slog.Debug("hook: aitop not running, skipping", "socket", socketPath, "err", err)
		return
	}
	defer conn.Close()

	if err := conn.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		slog.Error("hook: failed to set write deadline", "err", err)
		return
	}
	if _, err := conn.Write(data); err != nil {
		slog.Error("hook: failed to write payload", "err", err)
	}
}

var aitopHooks = map[string][]map[string]any{
	"PreToolUse":         {{"matcher": "", "hooks": []any{map[string]any{"type": "command", "command": "aitop hook"}}}},
	"PostToolUse":        {{"matcher": "", "hooks": []any{map[string]any{"type": "command", "command": "aitop hook"}}}},
	"PostToolUseFailure": {{"matcher": "", "hooks": []any{map[string]any{"type": "command", "command": "aitop hook"}}}},
	"Stop":               {{"hooks": []any{map[string]any{"type": "command", "command": "aitop hook"}}}},
	"StopFailure":        {{"hooks": []any{map[string]any{"type": "command", "command": "aitop hook"}}}},
	"SubagentStop":       {{"hooks": []any{map[string]any{"type": "command", "command": "aitop hook"}}}},
	"SessionStart":       {{"hooks": []any{map[string]any{"type": "command", "command": "aitop hook"}}}},
	"SessionEnd":         {{"hooks": []any{map[string]any{"type": "command", "command": "aitop hook"}}}},
	"UserPromptSubmit":   {{"hooks": []any{map[string]any{"type": "command", "command": "aitop hook"}}}},
	"PreCompact":         {{"hooks": []any{map[string]any{"type": "command", "command": "aitop hook"}}}},
	"PostCompact":        {{"hooks": []any{map[string]any{"type": "command", "command": "aitop hook"}}}},
}

// runInstall reads ~/.claude/settings.json, merges the aitop hook entries,
// and prints the result. Pass --write to atomically write the file instead.
func runInstall() {
	write := len(os.Args) > 2 && os.Args[2] == "--write"

	// Check aitop is on PATH.
	if _, err := exec.LookPath("aitop"); err != nil {
		fmt.Fprintln(os.Stderr, "warning: 'aitop' not found in PATH — hooks will fail until it is installed")
		fmt.Fprintln(os.Stderr, "  hint: go install github.com/jpackagejasonc/aitop/cmd/aitop@latest")
		fmt.Fprintln(os.Stderr)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not determine home directory: %v\n", err)
		os.Exit(1)
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")

	// Warn if settings file permissions are too permissive.
	if info, err := os.Stat(settingsPath); err == nil {
		if mode := info.Mode().Perm(); mode&0077 != 0 {
			fmt.Fprintf(os.Stderr, "warning: %s has permissions %o — recommended: 600\n\n", settingsPath, mode)
		}
	}

	// Load existing settings if present.
	settings := map[string]any{}
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not parse %s: %v\n\n", settingsPath, err)
		}
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "warning: could not read %s: %v\n\n", settingsPath, err)
	}

	// Merge hooks without overwriting existing entries.
	existing, _ := settings["hooks"].(map[string]any)
	if existing == nil {
		existing = map[string]any{}
	}
	for event, newEntries := range aitopHooks {
		cur, _ := existing[event].([]any)
		if !hasAitopHook(cur) {
			for _, e := range newEntries {
				cur = append(cur, e)
			}
		}
		existing[event] = cur
	}
	settings["hooks"] = existing

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if !write {
		fmt.Printf("Merged config for %s (not written — copy manually, or re-run with --write):\n\n", settingsPath)
		fmt.Println(string(out))
		return
	}

	// Write atomically: write to a temp file in the same directory, then rename.
	// This prevents a partial write from corrupting the settings file.
	dir := filepath.Dir(settingsPath)
	tmp, err := os.CreateTemp(dir, ".settings-*.json.tmp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not create temp file: %v\n", err)
		os.Exit(1)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // clean up if rename fails

	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		fmt.Fprintf(os.Stderr, "error: could not write temp file: %v\n", err)
		os.Exit(1)
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		fmt.Fprintf(os.Stderr, "error: could not set temp file permissions: %v\n", err)
		os.Exit(1)
	}
	if err := tmp.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "error: could not close temp file: %v\n", err)
		os.Exit(1)
	}
	if err := os.Rename(tmpName, settingsPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: could not rename temp file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Written to %s\n", settingsPath)
}

// hasAitopHook returns true if any entry in the slice already contains an aitop hook command.
func hasAitopHook(entries []any) bool {
	for _, entry := range entries {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		hooks, _ := m["hooks"].([]any)
		for _, h := range hooks {
			hm, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if cmd, _ := hm["command"].(string); cmd == "aitop hook" {
				return true
			}
		}
	}
	return false
}

// runTUI starts the aggregator, connects the Claude Code provider, and launches
// the Bubble Tea TUI.
func runTUI() {
	agg := aggregator.New()
	eb := bus.New()
	ccProvider := claudecode.New()

	events, err := ccProvider.Connect()
	if err != nil {
		fmt.Fprintf(os.Stderr, "aitop: failed to connect claude code provider: %v\n", err)
		os.Exit(1)
	}

	// Fan provider events onto the bus.
	go func() {
		for e := range events {
			eb.Publish(e)
		}
	}()

	// Aggregator subscribes to the bus.
	aggSub := eb.Subscribe()
	go func() {
		for e := range aggSub {
			agg.Ingest(e)
		}
	}()

	// Periodically prune old records (keep at most 30 minutes of history).
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			agg.Prune(30 * time.Minute)
		}
	}()

	model := tui.New(agg, claudecode.PricingStale())
	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "aitop: %v\n", err)
	}

	_ = ccProvider.Disconnect()
}
