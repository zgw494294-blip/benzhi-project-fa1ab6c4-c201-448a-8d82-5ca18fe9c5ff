package returnedrevisionbatch

import (
	"testing"
	"time"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/calibration"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/domain"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/measurement"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/review"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
)

func TestReturnedRevisionBatchCommitsSingleVersion(t *testing.T) {
	repository, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	calibrations := calibration.New(repository)
	measurements := measurement.New(repository, calibrations)
	reviews := review.New(repository, calibrations, measurements)

	task, err := calibrations.Create(calibration.CreateInput{
		StationCode:    "STA-RETURN",
		InstrumentID:   "INS-RETURN",
		InstrumentType: "地震计",
		ReferenceStandards: []domain.ReferenceStandard{{
			ID:            "STD-RETURN",
			Name:          "振动台",
			CertificateNo: "REF-RETURN",
			ValidUntil:    time.Now().AddDate(1, 0, 0),
		}},
		RangeMin:  0,
		RangeMax:  100,
		CreatedBy: "技术员甲",
	}, "create-returned-batch")
	if err != nil {
		t.Fatal(err)
	}

	initialValue := 10.01
	initial, err := measurements.AddPoint(task.ID, task.Revision, measurement.PointInput{
		PointLabel:     "P0",
		ReferenceValue: 10,
		ObservedValue:  &initialValue,
		Unit:           "m/s2",
		Uncertainty:    0.01,
		Tolerance:      0.1,
		EnteredBy:      "技术员甲",
	}, "initial-measurement")
	if err != nil {
		t.Fatal(err)
	}
	task, err = reviews.Submit(task.ID, initial.Revision, "技术员甲", "initial-submit")
	if err != nil {
		t.Fatal(err)
	}
	task, err = reviews.Decide(task.ID, task.Revision, review.DecisionInput{
		Decision: "退回",
		Comment:  "需要补充两个复测点",
		Reviewer: "复核员乙",
	}, "return-for-batch")
	if err != nil {
		t.Fatal(err)
	}

	firstValue, secondValue := 20.01, 30.01
	batch, err := measurements.SubmitBatch(task.ID, task.Revision, "技术员甲", "returned-batch", []measurement.PointInput{
		{PointLabel: "P1", ReferenceValue: 20, ObservedValue: &firstValue, Unit: "m/s2", Uncertainty: 0.01, Tolerance: 0.1},
		{PointLabel: "P2", ReferenceValue: 30, ObservedValue: &secondValue, Unit: "m/s2", Uncertainty: 0.01, Tolerance: 0.1},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err = reviews.Submit(task.ID, batch.Version, "技术员甲", "resubmit-returned-batch"); err != nil {
		t.Fatalf("批量补录返回的版本无法继续提交复核: %v", err)
	}
}
