# Session Communication: Nudge and Peek

Gas Town agents communicate with Claude Code sessions through **two canonical commands**:

| Command | Direction | Purpose |
|---------|-----------|---------|
| `gt nudge` | You → Agent | Send a message reliably |
| `gt peek` | Agent → You | Read recent output |

## Why Not Raw tmux?

**tmux send-keys is unreliable for Claude Code sessions.**

The problem: When you send text followed by Enter using `tmux send-keys "message" Enter`,
the Enter key often arrives before the paste completes. Claude receives a truncated
message or the Enter appends to the previous line.

This is a race condition in tmux's input handling. It's not a bug - tmux wasn't
designed for pasting multi-line content to interactive AI sessions.

### The Reliable Pattern

`gt nudge` uses a tested, reliable pattern:

```go
// 1. Send text in literal mode (handles special characters)
tmux send-keys -t session -l "message"

// 2. Wait 500ms for paste to complete
time.Sleep(500ms)

// 3. Send Enter as separate command
tmux send-keys -t session Enter
```

**Never use raw tmux send-keys for agent sessions.** Always use `gt nudge`.

## Command Reference

### gt nudge

Send a message to a polecat's Claude session:

```bash
gt nudge <rig/polecat> <message>

# Examples
gt nudge gastown/furiosa "Check your mail and start working"
gt nudge gastown/alpha "What's your status?"
gt nudge gastown/beta "Stop what you're doing and read gt-xyz"
```

### gt peek

View recent output from a polecat's session:

```bash
gt peek <rig/polecat> [lines]

# Examples
gt peek gastown/furiosa         # Last 100 lines (default)
gt peek gastown/furiosa 50      # Last 50 lines
gt peek gastown/furiosa -n 200  # Last 200 lines
```

## Common Patterns

### Check on a polecat

```bash
# See what they're doing
gt peek gastown/furiosa

# Ask for status
gt nudge gastown/furiosa "What's your current status?"
```

### Redirect a polecat

```bash
# Stop current work and pivot
gt nudge gastown/furiosa "Stop current task. New priority: read and act on gt-xyz"
```

### Wake up a stuck polecat

```bash
# Sometimes agents get stuck waiting for input
gt nudge gastown/furiosa "Continue working"
```

### Batch check all polecats

```bash
# Quick status check
for p in furiosa nux slit; do
  echo "=== $p ==="
  gt peek gastown/$p 20
done
```

## For Template Authors

When writing agent templates (deacon.md.tmpl, polecat.md.tmpl, etc.), include this guidance:

```markdown
## Session Communication

To send messages to other agents, use gt commands:
- `gt nudge <rig/polecat> <message>` - Send reliably
- `gt peek <rig/polecat>` - Read output

⚠️ NEVER use raw tmux send-keys - it's unreliable for Claude sessions.
```

## Implementation Details

The nudge/peek commands wrap these underlying functions:

| Command | Wrapper | Underlying |
|---------|---------|------------|
| `gt nudge` | `tmux.NudgeSession()` | literal send-keys + delay + Enter |
| `gt peek` | `session.Capture()` | tmux capture-pane |

The 500ms delay in NudgeSession was determined empirically. Shorter delays fail
intermittently; longer delays work but slow down communication unnecessarily.
