package domain

import "time"

type CalibrationCertificate struct {
	CertificateNo     string             `json:"certificateNo"`
	TaskID            string             `json:"taskId"`
	Revision          uint64             `json:"revision"`
	ResultDigest      string             `json:"resultDigest"`
	IssuedBy          string             `json:"issuedBy"`
	IssuedAt          time.Time          `json:"issuedAt"`
	ValidUntil        time.Time          `json:"validUntil"`
	VerificationCode  string             `json:"verificationCode"`
	MeasurementDigest string             `json:"measurementDigest"`
	Status            string             `json:"status"`
	Task              CalibrationTask    `json:"task"`
	Measurements      []MeasurementPoint `json:"measurements"`
	Deviations        []DeviationCase    `json:"deviations"`
	Coverage          CoverageSnapshot   `json:"coverage"`
	ReviewChecklist   ReviewChecklist    `json:"reviewChecklist"`
	VoidedReason      string             `json:"voidedReason,omitempty"`
	VoidedBy          string             `json:"voidedBy,omitempty"`
	VoidedAt          *time.Time         `json:"voidedAt,omitempty"`
}
