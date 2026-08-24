package domain

import "time"

type CalibrationTask struct {
	ID                    string              `json:"id"`
	StationCode           string              `json:"stationCode"`
	InstrumentID          string              `json:"instrumentId"`
	InstrumentType        string              `json:"instrumentType"`
	ReferenceStandards    []ReferenceStandard `json:"referenceStandards"`
	RangeMin              float64             `json:"rangeMin"`
	RangeMax              float64             `json:"rangeMax"`
	Revision              uint64              `json:"revision"`
	Status                TaskStatus          `json:"status"`
	CreatedBy             string              `json:"createdBy"`
	UpdatedAt             time.Time           `json:"updatedAt"`
	ReviewComment         string              `json:"reviewComment,omitempty"`
	RequiredPoints        []RequiredPoint     `json:"requiredPoints,omitempty"`
	ReturnItems           []ReviewReturnItem  `json:"returnItems,omitempty"`
	LastFrozenCertificate *CertificateSummary `json:"lastFrozenCertificate,omitempty"`
}

type RequiredPoint struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

type CertificateSummary struct {
	CertificateNo string    `json:"certificateNo"`
	IssuedAt      time.Time `json:"issuedAt"`
	ValidUntil    time.Time `json:"validUntil"`
}

type ReferenceStandard struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	CertificateNo string    `json:"certificateNo"`
	ValidUntil    time.Time `json:"validUntil"`
}
