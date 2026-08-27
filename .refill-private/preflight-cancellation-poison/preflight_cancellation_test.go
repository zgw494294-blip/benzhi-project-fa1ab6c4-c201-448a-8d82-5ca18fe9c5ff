package preflight_cancellation_poison_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/calibration"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/domain"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/transport"
)

type stagedCancelContext struct {
	context.Context
	checked chan struct{}
	release chan struct{}
	done    chan struct{}
	once    sync.Once
}

func (c *stagedCancelContext) Done() <-chan struct{} { return c.done }
func (c *stagedCancelContext) Err() error {
	c.once.Do(func() { close(c.checked) })
	<-c.release
	return context.Canceled
}

type observedWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *observedWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestPreflightCancellationDoesNotPoisonRetry(t *testing.T) {
	repository, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tasks := calibration.New(repository)
	task, err := tasks.Create(calibration.CreateInput{
		StationCode:        "STA-CANCEL",
		InstrumentID:       "INS-CANCEL",
		InstrumentType:     "地震计",
		ReferenceStandards: []domain.ReferenceStandard{{ID: "STD-1", Name: "振动台", CertificateNo: "REF-1", ValidUntil: time.Now().AddDate(1, 0, 0)}},
		RangeMin:           0,
		RangeMax:           100,
		CreatedBy:          "技术员",
	}, "create-cancel-test")
	if err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(map[string]any{
		"expectedVersion": task.Revision,
		"actor":           "技术员",
		"preflight":       true,
		"points": []map[string]any{{
			"pointLabel": "P1", "referenceValue": 10, "observedValue": 10.01,
			"unit": "m/s2", "uncertainty": 0.01, "tolerance": 0.1,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	handler := transport.NewServer(repository).Handler()
	path := "/api/calibrations/" + task.ID + "/measurements/batch/preflight"
	leaderContext := &stagedCancelContext{Context: context.Background(), checked: make(chan struct{}), release: make(chan struct{}), done: make(chan struct{})}
	first := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)).WithContext(leaderContext)
	firstFinished := make(chan struct{})
	go func() {
		handler.ServeHTTP(httptest.NewRecorder(), first)
		close(firstFinished)
	}()
	<-leaderContext.checked

	retryBase, cancelRetry := context.WithCancel(context.Background())
	retryContext := &observedWaitContext{Context: retryBase, waiting: make(chan struct{})}
	retry := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)).WithContext(retryContext)
	recorder := httptest.NewRecorder()
	retryFinished := make(chan struct{})
	go func() {
		handler.ServeHTTP(recorder, retry)
		close(retryFinished)
	}()
	<-retryContext.waiting

	close(leaderContext.done)
	close(leaderContext.release)
	<-firstFinished
	cancelRetry()
	<-retryFinished
	if recorder.Code != http.StatusOK {
		t.Fatalf("取消的预检污染了后续同参数请求：status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
