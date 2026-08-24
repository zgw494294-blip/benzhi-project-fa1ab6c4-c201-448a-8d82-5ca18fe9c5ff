package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendRestartIdempotencyAndIntegrity(t *testing.T) {
	dir := t.TempDir()
	repository, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	proposal := ProposedEvent{AggregateID: "task-1", Type: "Created", Actor: "tester", Payload: map[string]string{"name": "仪器"}}
	events, replayed, err := repository.AppendBatch("task-1", 0, "key-1", proposal)
	if err != nil || replayed || len(events) != 1 {
		t.Fatalf("first append: events=%d replay=%v err=%v", len(events), replayed, err)
	}
	if _, _, err = repository.AppendBatch("task-1", 0, "key-2", proposal); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("want version conflict, got %v", err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	retry, replayed, err := reopened.AppendBatch("task-1", 0, "key-1", proposal)
	if err != nil || !replayed || len(retry) != 1 {
		t.Fatalf("restart retry: replay=%v err=%v", replayed, err)
	}
	report, err := reopened.Inspect()
	if err != nil {
		t.Fatal(err)
	}
	if !report.LogValid || !report.SnapshotValid || report.EventCount != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestCorruptSnapshotIsRebuiltFromLog(t *testing.T) {
	dir := t.TempDir()
	repository, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = repository.AppendBatch("a", 0, "k", ProposedEvent{AggregateID: "a", Type: "Created", Payload: map[string]int{"v": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "snapshot.json"), []byte(`{"schemaVersion":1,"lastSequence":99}`), 0o640); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	report, err := reopened.Inspect()
	if err != nil {
		t.Fatal(err)
	}
	if report.EventCount != 1 || !report.SnapshotValid {
		t.Fatalf("snapshot was not rebuilt: %+v", report)
	}
}

func TestCorruptEventLogStopsRecovery(t *testing.T) {
	dir := t.TempDir()
	repository, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = repository.AppendBatch("a", 0, "k", ProposedEvent{AggregateID: "a", Type: "Created", Payload: map[string]int{"v": 1}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "events.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var event Event
	if err = json.Unmarshal([]byte(strings.TrimSpace(string(data))), &event); err != nil {
		t.Fatal(err)
	}
	event.Actor = "篡改者"
	tampered, _ := json.Marshal(event)
	if err = os.WriteFile(path, append(tampered, '\n'), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(dir); !errors.Is(err, ErrCorruptLog) {
		t.Fatalf("want corrupt log, got %v", err)
	}
}
