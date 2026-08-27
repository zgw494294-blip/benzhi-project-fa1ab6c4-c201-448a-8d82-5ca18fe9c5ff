package rejected_review_checklist_pollution_test

import (
	"errors"
	"testing"
	"time"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/calibration"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/domain"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/measurement"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/review"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
)

func TestRejectedReviewDoesNotPolluteChecklist(t *testing.T) {
	repository, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	calibrations := calibration.New(repository)
	measurements := measurement.New(repository, calibrations)
	reviews := review.New(repository, calibrations, measurements)

	task, err := calibrations.Create(calibration.CreateInput{
		StationCode:    "STA-CACHE",
		InstrumentID:   "INS-CACHE",
		InstrumentType: "地震计",
		RangeMin:       0,
		RangeMax:       100,
		CreatedBy:      "技术员甲",
		ReferenceStandards: []domain.ReferenceStandard{{
			ID:            "STD-CACHE",
			Name:          "振动台",
			CertificateNo: "REF-CACHE",
			ValidUntil:    time.Now().AddDate(1, 0, 0),
		}},
	}, "create-cache-task")
	if err != nil {
		t.Fatal(err)
	}
	observed := 20.01
	point, err := measurements.AddPoint(task.ID, task.Revision, measurement.PointInput{
		PointLabel:     "P1",
		ReferenceValue: 20,
		ObservedValue:  &observed,
		Unit:           "m/s2",
		Uncertainty:    0.01,
		Tolerance:      0.1,
		EnteredBy:      "技术员甲",
	}, "measure-cache-task")
	if err != nil {
		t.Fatal(err)
	}
	task, err = reviews.Submit(task.ID, point.Revision, "技术员甲", "submit-cache-task")
	if err != nil {
		t.Fatal(err)
	}

	before, err := reviews.Checklist(task.ID, task.Revision)
	if err != nil {
		t.Fatal(err)
	}
	assertReviewerSeparation(t, before, "待确认", "复核提交时校验")

	_, err = reviews.Decide(task.ID, task.Revision, review.DecisionInput{
		Decision:         "退回",
		Comment:          "请补充依据",
		Reviewer:         "复核员乙",
		ChecklistVersion: task.Revision - 1,
	}, "rejected-review")
	if !errors.Is(err, store.ErrVersionConflict) {
		t.Fatalf("预期复核决定因清单版本冲突被拒绝，实际错误为 %v", err)
	}
	if decisions := reviews.Decisions(task.ID); len(decisions) != 0 {
		t.Fatalf("被拒绝的复核决定不应持久化，实际记录为 %+v", decisions)
	}

	after, err := reviews.Checklist(task.ID, task.Revision)
	if err != nil {
		t.Fatal(err)
	}
	assertReviewerSeparation(t, after, "待确认", "复核提交时校验")
}

func assertReviewerSeparation(t *testing.T, checklist domain.ReviewChecklist, wantStatus, wantEvidence string) {
	t.Helper()
	for _, item := range checklist.Items {
		if item.ID != "reviewer_separation" {
			continue
		}
		if item.Status != wantStatus || item.Evidence != wantEvidence {
			t.Fatalf("被拒绝的复核请求污染了缓存清单：status=%q evidence=%q", item.Status, item.Evidence)
		}
		return
	}
	t.Fatal("复核清单缺少 reviewer_separation 项")
}
