package audit

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestFileSinkAppendsEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	sink, err := NewFileSink(path)
	if err != nil {
		t.Fatalf("expected file sink to initialize: %v", err)
	}
	defer func() {
		if err := sink.Close(); err != nil {
			t.Fatalf("expected file sink to close cleanly: %v", err)
		}
	}()

	firstOccurredAt := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	secondOccurredAt := firstOccurredAt.Add(time.Second)

	if err := sink.Append(context.Background(), Event{
		EventID:    "evt_1",
		EventType:  "session.bootstrapped",
		OccurredAt: firstOccurredAt,
		SessionID:  "sess_1",
	}); err != nil {
		t.Fatalf("expected first event append to succeed: %v", err)
	}

	if err := sink.Append(context.Background(), Event{
		EventID:    "evt_2",
		EventType:  "tool.completed",
		OccurredAt: secondOccurredAt,
		SessionID:  "sess_1",
		ToolName:   "tools.list",
	}); err != nil {
		t.Fatalf("expected second event append to succeed: %v", err)
	}

	events, err := LoadEvents(path)
	if err != nil {
		t.Fatalf("expected event log to load: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 appended events, got %d", len(events))
	}

	if events[0].EventID != "evt_1" || events[1].EventID != "evt_2" {
		t.Fatalf("unexpected event ordering: %+v", events)
	}
}
