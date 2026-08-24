package certificate_test

import (
	"errors"
	"testing"
	"time"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/calibration"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/certificate"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/domain"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/measurement"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/review"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
)

func services(t *testing.T, dir string) (*calibration.Service, *measurement.Service, *review.Service, *certificate.Service) {
	t.Helper()
	repository, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := calibration.New(repository)
	m := measurement.New(repository, c)
	r := review.New(repository, c, m)
	return c, m, r, certificate.New(repository, c, m, r)
}
func input() calibration.CreateInput {
	return calibration.CreateInput{StationCode: "SC-01", InstrumentID: "INS-01", InstrumentType: "地震计", ReferenceStandards: []domain.ReferenceStandard{{ID: "STD-1", Name: "振动台", CertificateNo: "REF-1", ValidUntil: time.Now().AddDate(1, 0, 0)}}, RangeMin: 0, RangeMax: 100, CreatedBy: "技术员"}
}

func TestReturnRevisionAndCertificatePersistence(t *testing.T) {
	dir := t.TempDir()
	c, m, r, cert := services(t, dir)
	task, err := c.Create(input(), "create")
	if err != nil {
		t.Fatal(err)
	}
	bad := 50.4
	point, err := m.AddPoint(task.ID, task.Revision, measurement.PointInput{PointLabel: "P1", ReferenceValue: 50, ObservedValue: &bad, Unit: "m/s2", Uncertainty: .1, Tolerance: .2, EnteredBy: "技术员"}, "measure-bad")
	if err != nil {
		t.Fatal(err)
	}
	task.Revision = point.Revision
	deviations := r.Deviations(task.ID)
	if len(deviations) != 1 || deviations[0].Status != "待整改" {
		t.Fatalf("unexpected deviations: %+v", deviations)
	}
	if _, err = r.Submit(task.ID, task.Revision, "技术员", "too-early"); !errors.Is(err, review.ErrDeviationOpen) {
		t.Fatalf("want open deviation, got %v", err)
	}
	_, err = r.Remediate(task.ID, task.Revision, review.RemediationInput{MeasurementID: point.ID, Cause: "安装偏移", Correction: "重新安装", Evidence: "复测照片摘要", Actor: "技术员"}, "remediate")
	if err != nil {
		t.Fatal(err)
	}
	task.Revision++
	task, err = r.Submit(task.ID, task.Revision, "技术员", "submit-1")
	if err != nil {
		t.Fatal(err)
	}
	task, err = r.Decide(task.ID, task.Revision, review.DecisionInput{Decision: "退回", Comment: "请重新测量", Reviewer: "复核员"}, "return")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != domain.StatusReturned {
		t.Fatalf("want returned, got %s", task.Status)
	}
	good := 50.01
	fixed, err := m.AddPoint(task.ID, task.Revision, measurement.PointInput{PointLabel: "P1-R2", ReferenceValue: 50, ObservedValue: &good, Unit: "m/s2", Uncertainty: .05, Tolerance: .2, EnteredBy: "技术员"}, "measure-fixed")
	if err != nil {
		t.Fatal(err)
	}
	task.Revision = fixed.Revision
	if len(m.List(task.ID)) != 2 || len(m.CurrentList(task.ID)) != 1 {
		t.Fatal("old readings were not preserved or current revision is wrong")
	}
	task, err = r.Submit(task.ID, task.Revision, "技术员", "submit-2")
	if err != nil {
		t.Fatal(err)
	}
	task, err = r.Decide(task.ID, task.Revision, review.DecisionInput{Decision: "通过", Comment: "复测合格", Reviewer: "复核员"}, "approve")
	if err != nil {
		t.Fatal(err)
	}
	issued, err := cert.Issue(task.ID, task.Revision, "质量负责人", "issue", 24)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := cert.Verify(issued.CertificateNo, issued.VerificationCode)
	if err != nil || !valid.Valid {
		t.Fatalf("verify: %+v %v", valid, err)
	}
	invalid, err := cert.Verify(issued.CertificateNo, "WRONG")
	if err != nil || invalid.Valid {
		t.Fatal("wrong verification code should be invalid")
	}
	_, _, _, reopened := services(t, dir)
	persisted, err := reopened.Verify(issued.CertificateNo, issued.VerificationCode)
	if err != nil || !persisted.Valid {
		t.Fatalf("restart verify: %+v %v", persisted, err)
	}
}

func TestExpiredReferenceStandardRejected(t *testing.T) {
	dir := t.TempDir()
	c, _, _, _ := services(t, dir)
	invalid := input()
	invalid.ReferenceStandards[0].ValidUntil = time.Now().Add(-time.Hour)
	if _, err := c.Create(invalid, "expired"); !errors.Is(err, calibration.ErrInvalid) {
		t.Fatalf("want invalid task, got %v", err)
	}
}

func TestBatchFindingsAndAtomicVersion(t *testing.T) {
	dir := t.TempDir()
	c, m, _, _ := services(t, dir)
	task, err := c.Create(input(), "create-batch")
	if err != nil {
		t.Fatal(err)
	}
	one := 10.01
	two := 20.5
	result, err := m.SubmitBatch(task.ID, task.Revision, "技术员", "batch", []measurement.PointInput{{PointLabel: "P1", ReferenceValue: 10, ObservedValue: &one, Unit: "m/s2", Uncertainty: .01, Tolerance: .1}, {PointLabel: "P2", ReferenceValue: 20, ObservedValue: &two, Unit: "m/s2", Uncertainty: .1, Tolerance: .2}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != task.Revision+1 || len(result.Points) != 2 || len(result.Findings) != 1 {
		t.Fatalf("unexpected batch: %+v", result)
	}
	loaded, err := c.Get(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != result.Version {
		t.Fatalf("want version %d got %d", result.Version, loaded.Revision)
	}
}
