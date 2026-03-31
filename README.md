# aitop

A `top`-like terminal dashboard for AI metrics. Tracks token usage, cost, and tool calls across your AI coding sessions in real time.

Currently supports **Claude Code**. Designed to support additional providers (OpenAI, Ollama, LiteLLM, etc.) over time.

## What it shows

```
aitop  2026-03-29 14:22:01  active: 1

metric                 1m            5m            15m
──────────────────────────────────────────────────────────
requests               3             8             21
req/min                3.0           1.6           1.4
errors                 0             0             0
error rate             -             -             -
input tokens           12450         31200         87600
output tokens          3820          9100          24300
tok/min                16270         8060          5453
cache hits             8900          22400         61200
cache hit rate         41.7%         41.8%         41.1%
cost ($)               $0.0142       $0.0381       $0.1024
tool calls             9             24            63

session
──────────────────────────────────────────────────────────
  total requests        21
  total cost            $0.1024
  input tokens          87600
  output tokens         24300
  cache hits            61200
  cache hit rate        41.2%
  stop: end_turn        19
  stop: max_tokens      2
  context window        142K / 200K (71%)
  compactions           1

q: quit  •  providers: claude code
```

Rolling windows (1m / 5m / 15m) behave like load averages in `top` — a quick read on recent activity vs. longer trends. The session section accumulates totals from when `aitop` was started.

**Cache hit rate** shows `cache_read / (cache_read + input)` for each rolling window. It turns orange below 60% and red below 30%. A sudden drop usually means a compaction just reset the context.

**Context window** shows an estimate of current context size (`input + cache_read + cache_write` tokens from the last completed turn) against the 200K limit. It turns orange above 60% and red above 85%. After a compaction the figure drops as the conversation is summarized. **Compactions** counts how many times automatic context compaction has run.

## Install

```bash
go install github.com/jpackagejasonc/aitop/cmd/aitop@latest
```

Requires Go 1.25+.

## Setup

`aitop` receives data from Claude Code via hooks. To wire them up:

**1. Generate the hook config:**

```bash
aitop install
```

This reads your existing `~/.claude/settings.json`, merges the required hook entries, and prints the result.

To write the file directly instead:

```bash
aitop install --write
```

`--write` uses an atomic write (temp file + rename) so the settings file is never left in a partial state.

**2. Copy the printed output into `~/.claude/settings.json`, or use `--write` to do it automatically.**

The hooks added look like this:

```json
{
  "hooks": {
    "PreToolUse":         [{"matcher": "", "hooks": [{"type": "command", "command": "aitop hook"}]}],
    "PostToolUse":        [{"matcher": "", "hooks": [{"type": "command", "command": "aitop hook"}]}],
    "PostToolUseFailure": [{"matcher": "", "hooks": [{"type": "command", "command": "aitop hook"}]}],
    "Stop":               [{"hooks": [{"type": "command", "command": "aitop hook"}]}],
    "StopFailure":        [{"hooks": [{"type": "command", "command": "aitop hook"}]}],
    "SubagentStop":       [{"hooks": [{"type": "command", "command": "aitop hook"}]}],
    "SessionStart":       [{"hooks": [{"type": "command", "command": "aitop hook"}]}],
    "SessionEnd":         [{"hooks": [{"type": "command", "command": "aitop hook"}]}],
    "UserPromptSubmit":   [{"hooks": [{"type": "command", "command": "aitop hook"}]}],
    "PreCompact":         [{"hooks": [{"type": "command", "command": "aitop hook"}]}],
    "PostCompact":        [{"hooks": [{"type": "command", "command": "aitop hook"}]}]
  }
}
```

`aitop install` is safe to re-run — it won't add duplicate entries.

## Usage

Start the dashboard before or during a Claude Code session:

```bash
aitop
```

Press `q` or `Ctrl+C` to quit.

The dashboard listens on a Unix socket (`~/.aitop/claudecode.sock`). Claude Code's hooks forward events to it via `aitop hook` (a subcommand called automatically by the hook config — you don't invoke it directly).

If `aitop` is not running, hook calls fail silently and do not affect Claude Code.

### Subcommands

| Command | Description |
|---|---|
| `aitop` | Start the TUI dashboard (default) |
| `aitop version` | Print the version and exit |
| `aitop install` | Print merged hook config for `~/.claude/settings.json` |
| `aitop install --write` | Write hook config directly (atomic) |
| `aitop hook` | Forward a hook event to the socket (called by Claude Code, not directly) |

## How it works

```
Claude Code hooks
  └─ aitop hook          # reads stdin, forwards JSON to socket
       └─ Unix socket (~/.aitop/claudecode.sock)
            └─ Claude Code provider
                 └─ Aggregator (rolling windows + session totals)
                      └─ TUI (redraws every second)
```

Events are normalised into a provider-agnostic schema, so future providers plug in without changing the aggregator or TUI.

## Cost accuracy

Costs are computed client-side using approximate Anthropic list prices (per million tokens):

| Model family       | Input   | Output  | Cache write | Cache read |
|--------------------|---------|---------|-------------|------------|
| claude-opus-4-6    | $15.00  | $75.00  | $18.75      | $1.50      |
| claude-sonnet-4-6  | $3.00   | $15.00  | $3.75       | $0.30      |
| claude-haiku-4-5   | $0.80   | $4.00   | $1.00       | $0.08      |

These are estimates. Check your Anthropic console for authoritative billing.

## Future plans

- **Tool call duration** — pair `PreToolUse` and `PostToolUse` events by tool call ID to track per-tool latency (e.g. how long a `Bash` or `Edit` call takes)
- **Projected hourly cost** — extrapolate a $/hr rate from the rolling window, similar to how cloud cost dashboards show spend rate
- **Cache token breakdown** — split the input token count into fresh context, cache writes, and cache reads so it is clear where tokens are coming from each window
- **Cross-model cost comparison** — using the actual token mix for the current session, show what the same usage would have cost on each model family; useful when deciding whether to switch models mid-session
- **Alerts** — contextual warnings surfaced in the TUI: configurable daily budget exceeded, high burn rate, spending with zero output tokens (likely a tool loop), context window near full, cache efficiency collapsed after a compaction
- **Session tagging** — label a session via an environment variable and track per-feature or per-initiative costs; useful for understanding where spend goes across a project
- **Historical summaries** — persist session totals on exit and expose daily/weekly/monthly aggregates so cost trends are visible across sessions, not just within the current one
- **Sparklines** — inline ASCII trend graphs alongside the rolling window numbers to make the direction of a metric visible at a glance
- **Live pricing** — fetch current model pricing from an upstream source (e.g. LiteLLM's pricing database) on startup and cache it locally for 24 hours, replacing the hardcoded table that requires a code change to update
- **Latency and TTFT** — both are currently unsupported for Claude Code. Hooks fire after a response completes with no request-start timestamp, so there is nothing to diff against. A local proxy provider that intercepts at the API level would unblock both metrics.
- **Output quality metrics** — groundedness/faithfulness, relevance, toxicity and safety, and LLM-as-a-judge scoring require access to the actual prompt and response content. Claude Code hooks only expose token counts and stop reasons; a local proxy provider that intercepts at the API level would make this possible. `max_tokens` stop reason frequency is the one available quality signal with the current hook architecture — a high rate suggests responses are being truncated.
- **Additional providers** — OpenAI, Ollama, LiteLLM; each would implement the `Provider` interface with a collection strategy suited to that stack (SDK wrapper, API polling, or callback integration)

## Similar projects

Several other tools take a similar approach to monitoring Claude Code sessions — worth knowing about if you want a different language runtime or feature set:

| Project | Language | Notes |
|---|---|---|
| [agtop](https://github.com/ldegio/agtop) | Node.js | Closest sibling to aitop; hook-based live dashboard |
| [claudetop](https://github.com/liorwn/claudetop) | Bash | Cost/burn-rate focus; no compiled binary required |
| [Claude-Code-Usage-Monitor](https://github.com/Maciek-roboblog/Claude-Code-Usage-Monitor) | Python | Predictive analytics, plan-limit tracking |
| [ccboard](https://github.com/FlorianBruniaux/ccboard) | Rust | Broader scope — multi-editor, web UI, budget forecasting |

## Security

aitop is a local tool — it only communicates over a Unix domain socket at `~/.aitop/claudecode.sock`.

**Access control**

The socket directory is created with `0700` and the socket file with `0600`. On Linux and macOS, incoming connections are verified against the current user's UID via `SO_PEERCRED` / `LOCAL_PEERCRED` and rejected if they come from a different user.

On other platforms (FreeBSD, WSL, etc.) peer credential verification is not implemented and the socket relies solely on filesystem permissions for isolation. Run aitop only on trusted systems if you are not on Linux or macOS.

**Known limitations**

*Socket permission race (TOCTTOU)* — there is a narrow window between `net.Listen()` creating the socket file and `os.Chmod()` restricting it to `0600`. During this window the socket has default permissions. This is a limitation of Go's standard `net` package on Unix and cannot be fully closed without dropping to raw syscalls. The window is typically under a millisecond and requires a concurrent attacker on the same machine to exploit.

*No event deduplication* — if the same hook event is somehow delivered twice (e.g. by an external process with socket access), it will be counted twice. `aitop hook` has no retry logic and Claude Code's hook runner does not replay events, so this has no realistic trigger in normal use.

*Session ID ownership not verified* — hook payloads are trusted to report the correct session ID. A same-user process could claim an arbitrary session ID and mix metrics between sessions. Same-user processes are trusted by design; different-user processes are blocked by peer credential verification. A proper fix would require Claude Code to sign or bind session IDs to the connecting user, which is outside aitop's control.

## Development

**Build from source**

```bash
git clone https://github.com/jpackagejasonc/aitop
cd aitop
go build -o aitop ./cmd/aitop
```

To embed a version at build time:

```bash
go build -ldflags "-X main.Version=1.2.3" -o aitop ./cmd/aitop
```

Untagged local builds report `0.0.0-dev`.

Move the binary somewhere on your `$PATH`, then run `aitop install --write` to wire up the Claude Code hooks.

**Tests and linting**

```bash
go test ./...
go vet ./...
govulncheck ./...   # go install golang.org/x/vuln/cmd/govulncheck@latest
```

Tests live alongside their packages. The aggregator and claudecode pricing tests cover the core logic; run them before submitting changes.

To enable debug logging:

```bash
AITOP_LOG_LEVEL=debug aitop
```

Logs are written to `~/.aitop/aitop.log` (rotated at 10 MB, one backup kept).

## Adding a provider

Implement the `Provider` interface in `internal/provider/provider.go`:

```go
type Provider interface {
    ID() string
    Name() string
    Connect() (<-chan Event, error)
    Disconnect() error
}
```

Emit normalised `Event` values on the returned channel. See `internal/provider/claudecode/` for a reference implementation.
