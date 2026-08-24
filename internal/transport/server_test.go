package transport

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
)

func TestWebAndCalibrationAPI(t *testing.T) {
	repository, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(repository).Handler())
	defer server.Close()
	response, err := http.Get(server.URL + "/calibrations")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("web status %d", response.StatusCode)
	}
	response.Body.Close()
	payload := map[string]any{"stationCode": "STA-1", "instrumentId": "INS-1", "instrumentType": "速度计", "referenceStandards": []map[string]any{{"id": "STD-1", "name": "振动台", "certificateNo": "REF-1", "validUntil": time.Now().AddDate(1, 0, 0)}}, "rangeMin": 0, "rangeMax": 100, "createdBy": "技术员"}
	data, _ := json.Marshal(payload)
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/calibrations", bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "create-http")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create status %d", response.StatusCode)
	}
	var task map[string]any
	if err = json.NewDecoder(response.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if task["status"] != "待测" {
		t.Fatalf("unexpected task: %+v", task)
	}
	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/calibrations", bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing key status %d", response.StatusCode)
	}
	var failure map[string]any
	_ = json.NewDecoder(response.Body).Decode(&failure)
	response.Body.Close()
	if failure["error"] == nil {
		t.Fatalf("stable error missing: %+v", failure)
	}
}
