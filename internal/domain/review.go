package domain

import "time"

type DeviationCase struct {
	ID            string    `json:"id"`
	TaskID        string    `json:"taskId"`
	Revision      uint64    `json:"revision"`
	MeasurementID string    `json:"measurementId"`
	Category      string    `json:"category"`
	Cause         string    `json:"cause"`
	Correction    string    `json:"correction"`
	Evidence      string    `json:"evidence"`
	ReviewComment string    `json:"reviewComment"`
	Status        string    `json:"status"`
	BatchID       string    `json:"batchId,omitempty"`
	RemediatedBy  string    `json:"remediatedBy,omitempty"`
	RemediatedAt  time.Time `json:"remediatedAt,omitempty"`
}

type ReviewChecklistItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Blocking bool   `json:"blocking"`
	Evidence string `json:"evidence"`
}

type ReviewChecklist struct {
	Version            uint64                `json:"version"`
	Items              []ReviewChecklistItem `json:"items"`
	MeasurementSummary MeasurementSnapshot   `json:"measurementSummary"`
	Coverage           CoverageSnapshot      `json:"coverage"`
}

type ReviewReturnItem struct {
	ChecklistItemID string `json:"checklistItemId"`
	Comment         string `json:"comment"`
}

type ReviewDecision struct {
	TaskID            string             `json:"taskId"`
	Revision          uint64             `json:"revision"`
	Reviewer          string             `json:"reviewer"`
	Decision          string             `json:"decision"`
	Comment           string             `json:"comment"`
	MeasurementDigest string             `json:"measurementDigest"`
	CreatedAt         time.Time          `json:"createdAt"`
	Checklist         ReviewChecklist    `json:"checklist"`
	ReturnItems       []ReviewReturnItem `json:"returnItems,omitempty"`
}
