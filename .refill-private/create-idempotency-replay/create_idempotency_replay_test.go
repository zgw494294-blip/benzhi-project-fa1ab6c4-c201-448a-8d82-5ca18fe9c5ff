package createidempotencyreplay

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/domain"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/transport"
)

func TestCreateIdempotencyReplayReturnsOriginalTask(t *testing.T) {
	repository, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(transport.NewServer(repository).Handler())
	defer server.Close()

	payload := map[string]any{
		"stationCode":    "STA-REPLAY",
		"instrumentId":   "INS-REPLAY",
		"instrumentType": "宽频带地震计",
		"referenceStandards": []map[string]any{{
			"id":            "STD-REPLAY",
			"name":          "振动标准器",
			"certificateNo": "REF-REPLAY",
			"validUntil":    time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
		}},
		"rangeMin":  0,
		"rangeMax":  100,
		"createdBy": "技术员甲",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	first := postCreate(t, server.URL, body, "create-replay-key")
	if first.StatusCode != http.StatusCreated {
		first.Body.Close()
		t.Fatalf("first create status = %d, want %d", first.StatusCode, http.StatusCreated)
	}
	var original domain.CalibrationTask
	if err := json.NewDecoder(first.Body).Decode(&original); err != nil {
		first.Body.Close()
		t.Fatal(err)
	}
	first.Body.Close()

	replayed := postCreate(t, server.URL, body, "create-replay-key")
	if replayed.StatusCode != http.StatusCreated {
		replayed.Body.Close()
		t.Fatalf("replay status = %d, want %d", replayed.StatusCode, http.StatusCreated)
	}
	var restored domain.CalibrationTask
	if err := json.NewDecoder(replayed.Body).Decode(&restored); err != nil {
		replayed.Body.Close()
		t.Fatal(err)
	}
	replayed.Body.Close()

	if restored.ID != original.ID {
		t.Fatalf("replay returned task %q, want original task %q", restored.ID, original.ID)
	}
	if got := len(repository.Events()); got != 1 {
		t.Fatalf("replay appended events: got %d, want 1", got)
	}
}

func postCreate(t *testing.T, baseURL string, body []byte, idempotencyKey string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/calibrations", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
