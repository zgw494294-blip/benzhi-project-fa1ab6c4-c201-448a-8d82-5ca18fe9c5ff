package certificate

import (
	"sync"
	"time"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/calibration"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/domain"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/measurement"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/review"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
)

type Verification struct {
	Certificate    domain.CalibrationCertificate `json:"certificate"`
	DigestValid    bool                          `json:"digestValid"`
	CodeValid      bool                          `json:"codeValid"`
	Issued         bool                          `json:"issued"`
	WithinValidity bool                          `json:"withinValidity"`
	Valid          bool                          `json:"valid"`
	Message        string                        `json:"message"`
	Voided         bool                          `json:"voided"`
}

type material struct {
	CertificateNo           string                    `json:"certificateNo"`
	TaskID                  string                    `json:"taskId"`
	Revision                uint64                    `json:"revision"`
	IssuedBy                string                    `json:"issuedBy"`
	IssuedAt                time.Time                 `json:"issuedAt"`
	ValidUntil              time.Time                 `json:"validUntil"`
	Task                    domain.CalibrationTask    `json:"task"`
	Measurements            []domain.MeasurementPoint `json:"measurements"`
	Deviations              []domain.DeviationCase    `json:"deviations"`
	PreviousCertificateHash string                    `json:"previousCertificateHash"`
	Coverage                domain.CoverageSnapshot   `json:"coverage"`
	ReviewChecklist         domain.ReviewChecklist    `json:"reviewChecklist"`
}

type VoidRecord struct {
	CertificateNo string    `json:"certificateNo"`
	Reason        string    `json:"reason"`
	Actor         string    `json:"actor"`
	VoidedAt      time.Time `json:"voidedAt"`
}

type ChainReport struct {
	Total                   int    `json:"total"`
	Checked                 int    `json:"checked"`
	Continuous              bool   `json:"continuous"`
	FirstAnomalyCertificate string `json:"firstAnomalyCertificate,omitempty"`
	EventSequence           uint64 `json:"eventSequence,omitempty"`
	AnomalyType             string `json:"anomalyType,omitempty"`
}

type Service struct {
	store       *store.Store
	calibration *calibration.Service
	measurement *measurement.Service
	review      *review.Service
	now         func() time.Time
	verifyMu    sync.RWMutex
	verifyCache map[string]Verification
}
