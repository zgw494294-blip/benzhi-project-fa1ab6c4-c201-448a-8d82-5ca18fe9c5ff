package certificate_verify_cache_test

import (
	"testing"
	"time"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/calibration"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/certificate"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/domain"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/measurement"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/review"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
)

func TestCertificateVerificationCacheInvalidatedAfterVoid(t *testing.T) {
	repository, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	calibrations := calibration.New(repository)
	measurements := measurement.New(repository, calibrations)
	reviews := review.New(repository, calibrations, measurements)
	certificates := certificate.New(repository, calibrations, measurements, reviews)
	task, err := calibrations.Create(calibration.CreateInput{
		StationCode:    "EQ-CACHE",
		InstrumentID:   "ACC-CACHE",
		InstrumentType: "强震加速度计",
		ReferenceStandards: []domain.ReferenceStandard{{
			ID: "STD-CACHE", Name: "标准振动台", CertificateNo: "REF-CACHE", ValidUntil: time.Now().AddDate(1, 0, 0),
		}},
		RangeMin: 0, RangeMax: 100, CreatedBy: "技术员",
	}, "cache-create")
	if err != nil {
		t.Fatal(err)
	}
	observed := 50.01
	point, err := measurements.AddPoint(task.ID, task.Revision, measurement.PointInput{
		PointLabel: "10Hz", ReferenceValue: 50, ObservedValue: &observed, Unit: "m/s2", Uncertainty: 0.01, Tolerance: 0.2, EnteredBy: "技术员",
	}, "cache-measure")
	if err != nil {
		t.Fatal(err)
	}
	task, err = reviews.Submit(task.ID, point.Revision, "技术员", "cache-submit")
	if err != nil {
		t.Fatal(err)
	}
	task, err = reviews.Decide(task.ID, task.Revision, review.DecisionInput{Decision: "通过", Comment: "数据完整", Reviewer: "复核员"}, "cache-review")
	if err != nil {
		t.Fatal(err)
	}
	issued, err := certificates.Issue(task.ID, task.Revision, "质量负责人", "cache-issue", 12)
	if err != nil {
		t.Fatal(err)
	}
	first, err := certificates.Verify(issued.CertificateNo, issued.VerificationCode)
	if err != nil || !first.Valid {
		t.Fatalf("签发后校验应通过: %+v, %v", first, err)
	}
	if _, err = certificates.Void(issued.CertificateNo, issued.Revision, "发现复核记录错误", "质量负责人", "cache-void"); err != nil {
		t.Fatal(err)
	}
	second, err := certificates.Verify(issued.CertificateNo, issued.VerificationCode)
	if err != nil {
		t.Fatal(err)
	}
	if second.Valid || !second.Voided || second.Certificate.Status != "已作废" {
		t.Fatalf("作废后应返回失效证书，实际得到: %+v", second)
	}
}
