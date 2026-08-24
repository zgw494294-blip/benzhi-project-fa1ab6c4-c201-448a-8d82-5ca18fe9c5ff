package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/calibration"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/certificate"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/domain"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/measurement"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/review"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/transport"
)

func runSelfcheck(listen string) error {
	dir, err := os.MkdirTemp("", "calibration-selfcheck-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	st, err := store.Open(dir)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("绑定自检地址 %s: %w", listen, err)
	}
	httpServer := &http.Server{Handler: transport.NewServer(st).Handler(), ReadHeaderTimeout: 2 * time.Second}
	done := make(chan error, 1)
	go func() { done <- httpServer.Serve(listener) }()
	defer func() { _ = httpServer.Close(); <-done }()
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + "/health")
	if err != nil {
		return fmt.Errorf("HTTP 健康检查失败: %w", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP 健康检查状态为 %d", response.StatusCode)
	}
	c := calibration.New(st)
	m := measurement.New(st, c)
	r := review.New(st, c, m)
	cert := certificate.New(st, c, m, r)
	task, err := c.Create(calibration.CreateInput{StationCode: "EQ-001", InstrumentID: "ACC-001", InstrumentType: "强震加速度计", ReferenceStandards: []domain.ReferenceStandard{{ID: "STD-001", Name: "标准振动台", CertificateNo: "REF-2026-001", ValidUntil: time.Now().AddDate(2, 0, 0)}}, RangeMin: 0, RangeMax: 100, CreatedBy: "自检技术员"}, "self-create")
	if err != nil {
		return err
	}
	observed := 50.02
	point, err := m.AddPoint(task.ID, task.Revision, measurement.PointInput{PointLabel: "10Hz", ReferenceValue: 50, ObservedValue: &observed, Unit: "m/s²", Uncertainty: .01, Tolerance: .1, EnteredBy: "自检技术员"}, "self-measure")
	if err != nil {
		return err
	}
	task.Revision = point.Revision
	task, err = r.Submit(task.ID, task.Revision, "自检技术员", "self-submit")
	if err != nil {
		return err
	}
	task, err = r.Decide(task.ID, task.Revision, review.DecisionInput{Decision: "通过", Comment: "依据完整，数据合格", Reviewer: "自检复核员"}, "self-review")
	if err != nil {
		return err
	}
	issued, err := cert.Issue(task.ID, task.Revision, "自检质量负责人", "self-certificate", 12)
	if err != nil {
		return err
	}
	result, err := cert.Verify(issued.CertificateNo, issued.VerificationCode)
	if err != nil {
		return err
	}
	if !result.Valid {
		return fmt.Errorf("证书校验结果无效")
	}
	return nil
}
