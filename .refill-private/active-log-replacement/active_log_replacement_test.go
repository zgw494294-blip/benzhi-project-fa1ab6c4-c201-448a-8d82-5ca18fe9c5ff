package active_log_replacement_test

import (
	"os"
	"path/filepath"
	"testing"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
)

func TestActiveLogReplacementKeepsAcknowledgedEvents(t *testing.T) {
	dir := t.TempDir()
	repository, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	first := store.ProposedEvent{AggregateID: "task-1", Type: "Created", Actor: "tester", Payload: map[string]int{"value": 1}}
	if _, _, err = repository.AppendBatch("task-1", 0, "first", first); err != nil {
		t.Fatalf("first append: %v", err)
	}

	logPath := filepath.Join(dir, "events.jsonl")
	contents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(logPath, filepath.Join(dir, "events.rotated.jsonl")); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(logPath, contents, 0o640); err != nil {
		t.Fatal(err)
	}

	second := store.ProposedEvent{AggregateID: "task-1", Type: "Updated", Actor: "tester", Payload: map[string]int{"value": 2}}
	if _, _, err = repository.AppendBatch("task-1", 1, "second", second); err != nil {
		t.Fatalf("second append was not acknowledged: %v", err)
	}

	reopened, err := store.Open(dir)
	if err != nil {
		t.Fatalf("restart failed: %v", err)
	}
	events := reopened.EventsFor("task-1")
	if len(events) != 2 || events[1].Type != "Updated" {
		t.Fatalf("acknowledged event was lost after active log replacement and restart: %+v", events)
	}
}
