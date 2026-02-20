package runlog

import (
	"testing"
	"time"
)

func TestAppendAndReadRun(t *testing.T) {
	t.Parallel()

	store := NewStore(t.TempDir())
	runID := NewRunID()
	if err := store.Append(Event{
		RunID:     runID,
		EventType: "command_started",
		Payload: map[string]interface{}{
			"command": "git push origin main",
		},
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Append(start): %v", err)
	}
	if err := store.Append(Event{
		RunID:     runID,
		EventType: "command_completed",
		Payload: map[string]interface{}{
			"output": "ok",
		},
		Timestamp: time.Now().UTC().Add(time.Second),
	}); err != nil {
		t.Fatalf("Append(done): %v", err)
	}

	events, err := store.ReadRun(runID)
	if err != nil {
		t.Fatalf("ReadRun: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}
	if events[0].EventType != "command_started" || events[1].EventType != "command_completed" {
		t.Fatalf("unexpected event ordering: %+v", events)
	}
}

func TestRedactString(t *testing.T) {
	t.Parallel()

	in := "Authorization: Bearer abcdefghijkl token=my-token ghp_abcdefghijklmnopqrstuvwxyz12"
	out := RedactString(in)
	if out == in {
		t.Fatalf("expected redaction to modify input")
	}
}
