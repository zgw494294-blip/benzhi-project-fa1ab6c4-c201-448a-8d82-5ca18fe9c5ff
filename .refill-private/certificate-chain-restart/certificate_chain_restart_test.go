package certificate_chain_restart_test

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

func TestCertificateChainTipSurvivesRestart(t *testing.T) {
	dir := t.TempDir()

	firstStore, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	issueReadyCertificate(t, firstStore, "first", "STA-01", "INS-01")

	reopened, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	secondService := issueReadyCertificate(t, reopened, "second", "STA-02", "INS-02")
	report := secondService.VerifyChain()
	if !report.Continuous {
		t.Fatalf("certificate chain must survive restart: anomaly=%s sequence=%d", report.AnomalyType, report.EventSequence)
	}
}

func issueReadyCertificate(t *testing.T, repository *store.Store, prefix, station, instrument string) *certificate.Service {
	t.Helper()
	calibrations := calibration.New(repository)
	measurements := measurement.New(repository, calibrations)
	reviews := review.New(repository, calibrations, measurements)
	certificates := certificate.New(repository, calibrations, measurements, reviews)

	task, err := calibrations.Create(calibration.CreateInput{
		StationCode:    station,
		InstrumentID:   instrument,
		InstrumentType: "地震计",
		ReferenceStandards: []domain.ReferenceStandard{{
			ID:            prefix + "-standard",
			Name:          "标准振动台",
			CertificateNo: prefix + "-reference",
			ValidUntil:    time.Now().AddDate(1, 0, 0),
		}},
		RangeMin:  0,
		RangeMax:  100,
		CreatedBy: "技术员-" + prefix,
	}, prefix+"-create")
	if err != nil {
		t.Fatal(err)
	}

	observed := 20.01
	point, err := measurements.AddPoint(task.ID, task.Revision, measurement.PointInput{
		PointLabel:     "20Hz",
		ReferenceValue: 20,
		ObservedValue:  &observed,
		Unit:           "m/s2",
		Uncertainty:    0.01,
		Tolerance:      0.1,
		EnteredBy:      "技术员-" + prefix,
	}, prefix+"-measure")
	if err != nil {
		t.Fatal(err)
	}

	task, err = reviews.Submit(task.ID, point.Revision, "技术员-"+prefix, prefix+"-submit")
	if err != nil {
		t.Fatal(err)
	}
	task, err = reviews.Decide(task.ID, task.Revision, review.DecisionInput{
		Decision: "通过",
		Comment:  "测量依据完整",
		Reviewer: "复核员-" + prefix,
	}, prefix+"-review")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = certificates.Issue(task.ID, task.Revision, "质量负责人-"+prefix, prefix+"-issue", 12); err != nil {
		t.Fatal(err)
	}
	return certificates
}
