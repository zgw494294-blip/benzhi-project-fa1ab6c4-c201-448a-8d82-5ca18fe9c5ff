package eventpayloadalias

import (
	"testing"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
)

func TestEventQueriesDoNotAliasPayload(t *testing.T) {
	repository, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = repository.AppendBatch("task-1", 0, "create-1", store.ProposedEvent{
		AggregateID: "task-1",
		Type:        "Created",
		Payload:     map[string]string{"name": "仪器"},
	})
	if err != nil {
		t.Fatal(err)
	}

	returned := repository.EventsFor("task-1")
	if len(returned) != 1 {
		t.Fatalf("want one event, got %d", len(returned))
	}
	returned[0].Payload[0] = '['

	var payload map[string]string
	if err := store.DecodePayload(repository.EventsFor("task-1")[0], &payload); err != nil {
		t.Fatalf("TestEventQueriesDoNotAliasPayload: mutating returned event corrupted the stored projection: %v", err)
	}
	if payload["name"] != "仪器" {
		t.Fatalf("stored payload changed through returned event: %#v", payload)
	}
}
