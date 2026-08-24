package calibration

import (
	"time"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/domain"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
)

type CreateInput struct {
	StationCode        string                     `json:"stationCode"`
	InstrumentID       string                     `json:"instrumentId"`
	InstrumentType     string                     `json:"instrumentType"`
	ReferenceStandards []domain.ReferenceStandard `json:"referenceStandards"`
	RangeMin           float64                    `json:"rangeMin"`
	RangeMax           float64                    `json:"rangeMax"`
	CreatedBy          string                     `json:"createdBy"`
}

type Service struct {
	store *store.Store
	now   func() time.Time
}

type HistoryEntry struct {
	Sequence      uint64            `json:"sequence"`
	Version       uint64            `json:"version"`
	EventType     string            `json:"eventType"`
	Actor         string            `json:"actor"`
	OccurredAt    time.Time         `json:"occurredAt"`
	Status        domain.TaskStatus `json:"status"`
	ReviewComment string            `json:"reviewComment,omitempty"`
}

type Query struct {
	Status      domain.TaskStatus
	StationCode string
	Instrument  string
}

type TransitConflict struct {
	TaskID    string            `json:"taskId"`
	Status    domain.TaskStatus `json:"status"`
	UpdatedAt time.Time         `json:"updatedAt"`
}
