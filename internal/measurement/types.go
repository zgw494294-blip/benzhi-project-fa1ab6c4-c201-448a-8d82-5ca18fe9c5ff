package measurement

import (
	"sync"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/calibration"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/domain"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
)

type PointInput struct {
	PointLabel     string   `json:"pointLabel"`
	ReferenceValue float64  `json:"referenceValue"`
	ObservedValue  *float64 `json:"observedValue"`
	Unit           string   `json:"unit"`
	Uncertainty    float64  `json:"uncertainty"`
	Tolerance      float64  `json:"tolerance"`
	EnteredBy      string   `json:"enteredBy"`
}

type Summary struct {
	Count      int    `json:"count"`
	Qualified  int    `json:"qualified"`
	Deviations int    `json:"deviations"`
	Complete   bool   `json:"complete"`
	Overall    string `json:"overall"`
}

type Finding struct {
	Code          string `json:"code"`
	Severity      string `json:"severity"`
	MeasurementID string `json:"measurementId"`
	Message       string `json:"message"`
	Row           int    `json:"row,omitempty"`
}

type BatchResult struct {
	Points   []domain.MeasurementPoint `json:"points"`
	Findings []Finding                 `json:"findings"`
	Summary  Summary                   `json:"summary"`
	Version  uint64                    `json:"version"`
}

type DeterministicSummary struct {
	TaskID   string                    `json:"taskId"`
	Summary  Summary                   `json:"summary"`
	Points   []domain.MeasurementPoint `json:"points"`
	Findings []Finding                 `json:"findings"`
	Digest   string                    `json:"digest"`
}

type RevisionGroup struct {
	Revision uint64                    `json:"revision"`
	Points   []domain.MeasurementPoint `json:"points"`
	Summary  Summary                   `json:"summary"`
}

type DiffItem struct {
	Label            string                   `json:"label"`
	Kind             string                   `json:"kind"`
	Before           *domain.MeasurementPoint `json:"before,omitempty"`
	After            *domain.MeasurementPoint `json:"after,omitempty"`
	ReadingChanged   bool                     `json:"readingChanged"`
	JudgementChanged bool                     `json:"judgementChanged"`
}

type RevisionDiff struct {
	BaseRevision    uint64     `json:"baseRevision"`
	CurrentRevision uint64     `json:"currentRevision"`
	Items           []DiffItem `json:"items"`
	ReturnComment   string     `json:"returnComment,omitempty"`
	ReturnVersion   uint64     `json:"returnVersion,omitempty"`
	HasChange       bool       `json:"hasChange"`
	BaseSummary     Summary    `json:"baseSummary"`
	CurrentSummary  Summary    `json:"currentSummary"`
}

type Service struct {
	store       *store.Store
	calibration *calibration.Service
	preflightMu sync.Mutex
	preflights  map[string]*preflightCall
}

type preflightCall struct {
	done   chan struct{}
	result BatchPrecheck
}

type BatchPrecheck struct {
	Points   []domain.MeasurementPoint `json:"points"`
	Findings []Finding                 `json:"findings"`
	Summary  Summary                   `json:"summary"`
	Version  uint64                    `json:"version"`
	Valid    bool                      `json:"valid"`
}
