package review

import (
	"time"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/calibration"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/domain"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/measurement"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
)

type RemediationInput struct {
	MeasurementID string `json:"measurementId"`
	Cause         string `json:"cause"`
	Correction    string `json:"correction"`
	Evidence      string `json:"evidence"`
	Actor         string `json:"actor"`
}

type DecisionInput struct {
	Decision         string                       `json:"decision"`
	Comment          string                       `json:"comment"`
	Reviewer         string                       `json:"reviewer"`
	Checklist        []domain.ReviewChecklistItem `json:"checklist,omitempty"`
	ReturnItems      []domain.ReviewReturnItem    `json:"returnItems,omitempty"`
	ChecklistVersion uint64                       `json:"checklistVersion,omitempty"`
	ConfirmedItemIDs []string                     `json:"confirmedItemIds,omitempty"`
}

type BatchRemediationInput struct {
	MeasurementIDs []string `json:"measurementIds"`
	Cause          string   `json:"cause"`
	Correction     string   `json:"correction"`
	Evidence       string   `json:"evidence"`
	Actor          string   `json:"actor"`
}

type DeviationCount struct {
	Open   int `json:"open"`
	Closed int `json:"closed"`
}

type Service struct {
	store       *store.Store
	calibration *calibration.Service
	measurement *measurement.Service
	now         func() time.Time
}

type Readiness struct {
	Ready            bool                    `json:"ready"`
	MeasurementCount int                     `json:"measurementCount"`
	OpenDeviations   int                     `json:"openDeviations"`
	LastDecision     string                  `json:"lastDecision,omitempty"`
	Reasons          []string                `json:"reasons"`
	Coverage         domain.CoverageSnapshot `json:"coverage"`
	Checklist        domain.ReviewChecklist  `json:"checklist"`
}
