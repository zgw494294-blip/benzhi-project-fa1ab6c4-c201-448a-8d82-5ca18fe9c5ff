package coverage_projection_stale_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/domain"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/transport"
)

func TestCoverageProjectionRefreshesAfterMeasurement(t *testing.T) {
	repository, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(transport.NewServer(repository).Handler())
	defer server.Close()

	var task struct {
		ID       string `json:"id"`
		Revision uint64 `json:"revision"`
	}
	requestJSON(t, server.URL, http.MethodPost, "/api/calibrations", "coverage-create", map[string]any{
		"stationCode":    "EQ-CACHE",
		"instrumentId":   "ACC-CACHE",
		"instrumentType": "强震加速度计",
		"referenceStandards": []map[string]any{{
			"id": "STD-CACHE", "name": "标准振动台", "certificateNo": "REF-CACHE",
			"validUntil": time.Now().AddDate(1, 0, 0),
		}},
		"rangeMin": 0, "rangeMax": 100, "createdBy": "技术员",
	}, http.StatusCreated, &task)

	var required struct {
		Task struct {
			Revision uint64 `json:"revision"`
		} `json:"task"`
		Coverage domain.CoverageSnapshot `json:"coverage"`
	}
	requestJSON(t, server.URL, http.MethodPut, "/api/calibrations/"+task.ID+"/required-points", "coverage-required", map[string]any{
		"expectedVersion": task.Revision,
		"actor":           "技术员",
		"points":          []map[string]any{{"label": "50Hz", "value": 50}},
	}, http.StatusOK, &required)
	if required.Coverage.Complete || required.Coverage.Completed != 0 {
		t.Fatalf("必测点刚设置时应尚未覆盖: %+v", required.Coverage)
	}

	observed := 50.01
	requestJSON(t, server.URL, http.MethodPost, "/api/calibrations/"+task.ID+"/measurements", "coverage-measurement", map[string]any{
		"expectedVersion": required.Task.Revision,
		"pointLabel":      "50Hz",
		"referenceValue":  50,
		"observedValue":   observed,
		"unit":            "m/s2",
		"uncertainty":     0.01,
		"tolerance":       0.1,
		"enteredBy":       "技术员",
	}, http.StatusCreated, nil)

	var coverage domain.CoverageSnapshot
	requestJSON(t, server.URL, http.MethodGet, "/api/calibrations/"+task.ID+"/coverage", "", nil, http.StatusOK, &coverage)
	if !coverage.Complete || coverage.Completed != 1 || coverage.Required != 1 {
		t.Fatalf("录入匹配测量点后覆盖率投影未刷新: %+v", coverage)
	}
}

func requestJSON(t *testing.T, baseURL, method, path, key string, body any, expectedStatus int, target any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequest(method, baseURL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("%s %s 状态码=%d，响应=%s", method, path, response.StatusCode, data)
	}
	if target != nil {
		if err := json.NewDecoder(response.Body).Decode(target); err != nil {
			t.Fatal(fmt.Errorf("解析 %s %s 响应: %w", method, path, err))
		}
	}
}
