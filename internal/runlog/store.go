// Package runlog provides append-only audit logging for command runs.
package runlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/google/uuid"
)

// Event is a single append-only run log event.
type Event struct {
	EventID        string                 `json:"event_id"`
	RunID          string                 `json:"run_id"`
	TenantID       string                 `json:"tenant_id,omitempty"`
	AgentID        string                 `json:"agent_id,omitempty"`
	State          string                 `json:"state,omitempty"`
	PolicyDecision string                 `json:"policy_decision,omitempty"`
	Attempt        int                    `json:"attempt,omitempty"`
	EventType      string                 `json:"event_type"`
	Payload        map[string]interface{} `json:"payload,omitempty"`
	Timestamp      time.Time              `json:"timestamp"`
}

// Store appends and reads audit events from daemon/command-events.jsonl.
type Store struct {
	path     string
	lockPath string
}

// NewStore creates a runlog store for a town root.
func NewStore(townRoot string) *Store {
	path := filepath.Join(townRoot, "daemon", "command-events.jsonl")
	return &Store{
		path:     path,
		lockPath: path + ".lock",
	}
}

// Path returns the underlying file path.
func (s *Store) Path() string {
	return s.path
}

// NewRunID returns a unique run ID.
func NewRunID() string {
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	if len(id) > 12 {
		id = id[:12]
	}
	return "run-" + time.Now().UTC().Format("20060102t150405") + "-" + id
}

// Append appends an event to the log.
func (s *Store) Append(evt Event) error {
	if strings.TrimSpace(evt.RunID) == "" {
		return fmt.Errorf("run_id is required")
	}
	if strings.TrimSpace(evt.EventType) == "" {
		return fmt.Errorf("event_type is required")
	}
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}
	if evt.EventID == "" {
		evt.EventID = "evt-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	}
	evt.Payload = RedactPayload(evt.Payload)

	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}

	lock := flock.New(s.lockPath)
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("locking runlog: %w", err)
	}
	defer lock.Unlock() //nolint:errcheck // best effort

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600) //nolint:gosec // operational local log
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

// ReadRun returns all events for a specific run, ordered by timestamp.
func (s *Store) ReadRun(runID string) ([]Event, error) {
	if strings.TrimSpace(runID) == "" {
		return nil, fmt.Errorf("run id is required")
	}
	if _, err := os.Stat(s.path); err != nil {
		if os.IsNotExist(err) {
			return []Event{}, nil
		}
		return nil, err
	}

	lock := flock.New(s.lockPath)
	if err := lock.Lock(); err != nil {
		return nil, fmt.Errorf("locking runlog: %w", err)
	}
	defer lock.Unlock() //nolint:errcheck // best effort

	f, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var evt Event
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}
		if evt.RunID == runID {
			evt.Payload = RedactPayload(evt.Payload)
			out = append(out, evt)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	return out, nil
}

var secretKVPattern = regexp.MustCompile(`(?i)\b(token|secret|password|api[_-]?key|authorization)\b(\s*[:=]\s*)([^\s,;]+)`)
var bearerPattern = regexp.MustCompile(`(?i)\bbearer\s+([a-z0-9\._\-]+)`)
var ghTokenPattern = regexp.MustCompile(`\b(gh[pousr]_[A-Za-z0-9]{20,})\b`)

// RedactString redacts common secret patterns from strings.
func RedactString(input string) string {
	out := secretKVPattern.ReplaceAllString(input, "${1}${2}[REDACTED]")
	out = bearerPattern.ReplaceAllString(out, "Bearer [REDACTED]")
	out = ghTokenPattern.ReplaceAllString(out, "[REDACTED_GITHUB_TOKEN]")
	return out
}

// RedactPayload redacts string-like values in event payloads.
func RedactPayload(payload map[string]interface{}) map[string]interface{} {
	if payload == nil {
		return nil
	}
	out := make(map[string]interface{}, len(payload))
	for k, v := range payload {
		switch val := v.(type) {
		case string:
			out[k] = RedactString(val)
		case []string:
			cpy := make([]string, 0, len(val))
			for _, item := range val {
				cpy = append(cpy, RedactString(item))
			}
			out[k] = cpy
		case map[string]interface{}:
			out[k] = RedactPayload(val)
		default:
			out[k] = val
		}
	}
	return out
}
