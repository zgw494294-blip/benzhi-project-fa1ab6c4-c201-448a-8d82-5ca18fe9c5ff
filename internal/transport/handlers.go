package transport

import (
	"errors"
	"net/http"
	"strings"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/calibration"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/domain"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/measurement"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/review"
)

func (s *Server) HandleListCalibrations(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": s.calibrations.List()})
}
func (s *Server) HandleSearchCalibrations(w http.ResponseWriter, r *http.Request) {
	items := s.calibrations.Search(calibration.Query{Status: domain.TaskStatus(r.URL.Query().Get("status")), StationCode: r.URL.Query().Get("station"), Instrument: r.URL.Query().Get("instrument")})
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (s *Server) HandleCreateCalibration(w http.ResponseWriter, r *http.Request) {
	var input calibration.CreateInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, err)
		return
	}
	k, err := key(r)
	if err != nil {
		writeError(w, err)
		return
	}
	task, err := s.calibrations.Create(input, k)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, task)
}
func (s *Server) HandleGetCalibration(w http.ResponseWriter, r *http.Request) {
	task, err := s.calibrations.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": task, "summary": s.measurements.Summary(task.ID), "coverage": s.measurements.Coverage(task.ID), "deviations": s.reviews.Deviations(task.ID), "reviews": s.reviews.Decisions(task.ID)})
}
func (s *Server) HandleListMeasurements(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": s.measurements.List(r.PathValue("id")), "current": s.measurements.CurrentList(r.PathValue("id")), "summary": s.measurements.Summary(r.PathValue("id")), "coverage": s.measurements.Coverage(r.PathValue("id"))})
}
func (s *Server) HandleFindings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": s.measurements.Findings(r.PathValue("id"))})
}
func (s *Server) HandleAddMeasurement(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpectedVersion uint64 `json:"expectedVersion"`
		measurement.PointInput
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if err := expected(body.ExpectedVersion); err != nil {
		writeError(w, err)
		return
	}
	k, err := key(r)
	if err != nil {
		writeError(w, err)
		return
	}
	point, err := s.measurements.AddPoint(r.PathValue("id"), body.ExpectedVersion, body.PointInput, k)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"measurement": point, "version": body.ExpectedVersion + 1, "summary": s.measurements.Summary(r.PathValue("id")), "coverage": s.measurements.Coverage(r.PathValue("id"))})
}
func (s *Server) HandleSetRequiredPoints(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpectedVersion uint64                 `json:"expectedVersion"`
		Actor           string                 `json:"actor"`
		Points          []domain.RequiredPoint `json:"points"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	k, err := key(r)
	if err != nil {
		writeError(w, err)
		return
	}
	task, err := s.calibrations.SetRequiredPoints(r.PathValue("id"), body.ExpectedVersion, body.Points, body.Actor, k)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": task, "coverage": s.measurements.Coverage(task.ID)})
}
func (s *Server) HandleCoverage(w http.ResponseWriter, r *http.Request) {
	if _, err := s.calibrations.Get(r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.measurements.Coverage(r.PathValue("id")))
}
func (s *Server) HandlePreflightMeasurementBatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpectedVersion uint64                   `json:"expectedVersion"`
		Actor           string                   `json:"actor"`
		Points          []measurement.PointInput `json:"points"`
		Preflight       bool                     `json:"preflight"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.Preflight {
		writeJSON(w, http.StatusOK, s.measurements.PrecheckBatch(r.PathValue("id"), body.ExpectedVersion, body.Actor, body.Points))
		return
	}
	writeJSON(w, http.StatusOK, s.measurements.PrecheckBatch(r.PathValue("id"), body.ExpectedVersion, body.Actor, body.Points))
}
func (s *Server) HandleAddMeasurementBatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpectedVersion uint64                   `json:"expectedVersion"`
		Actor           string                   `json:"actor"`
		Points          []measurement.PointInput `json:"points"`
		Preflight       bool                     `json:"preflight"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.Preflight {
		writeJSON(w, http.StatusOK, s.measurements.PrecheckBatch(r.PathValue("id"), body.ExpectedVersion, body.Actor, body.Points))
		return
	}
	k, err := key(r)
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := s.measurements.SubmitBatch(r.PathValue("id"), body.ExpectedVersion, body.Actor, k, body.Points)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
func (s *Server) HandleListDeviations(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": s.reviews.Deviations(r.PathValue("id")), "stats": s.reviews.DeviationStats(r.PathValue("id"))})
}
func (s *Server) HandleRemediateDeviationBatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpectedVersion uint64 `json:"expectedVersion"`
		review.BatchRemediationInput
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	k, err := key(r)
	if err != nil {
		writeError(w, err)
		return
	}
	items, err := s.reviews.RemediateBatch(r.PathValue("id"), body.ExpectedVersion, body.BatchRemediationInput, k)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "version": body.ExpectedVersion + 1, "deviations": s.reviews.Deviations(r.PathValue("id")), "stats": s.reviews.DeviationStats(r.PathValue("id"))})
}
func (s *Server) HandleRemediateDeviation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpectedVersion uint64 `json:"expectedVersion"`
		Cause           string `json:"cause"`
		Correction      string `json:"correction"`
		Evidence        string `json:"evidence"`
		Actor           string `json:"actor"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	k, err := key(r)
	if err != nil {
		writeError(w, err)
		return
	}
	item, err := s.reviews.Remediate(r.PathValue("id"), body.ExpectedVersion, review.RemediationInput{MeasurementID: r.PathValue("measurementID"), Cause: body.Cause, Correction: body.Correction, Evidence: body.Evidence, Actor: body.Actor}, k)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
func (s *Server) HandleSubmitReview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpectedVersion uint64 `json:"expectedVersion"`
		Actor           string `json:"actor"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	k, err := key(r)
	if err != nil {
		writeError(w, err)
		return
	}
	task, err := s.reviews.Submit(r.PathValue("id"), body.ExpectedVersion, body.Actor, k)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}
func (s *Server) HandleListReviews(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": s.reviews.Decisions(r.PathValue("id"))})
}
func (s *Server) HandleReviewChecklist(w http.ResponseWriter, r *http.Request) {
	task, err := s.calibrations.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	checklist, err := s.reviews.Checklist(task.ID, task.Revision)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, checklist)
}
func (s *Server) HandleReadiness(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.reviews.Readiness(r.PathValue("id")))
}
func (s *Server) HandleTaskHistory(w http.ResponseWriter, r *http.Request) {
	history, err := s.calibrations.History(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	diff, _ := s.measurements.Diff(r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]any{"items": history, "deviations": s.reviews.DeviationHistory(r.PathValue("id")), "revisions": s.measurements.RevisionHistory(r.PathValue("id")), "difference": diff})
}
func (s *Server) HandleReviewDecision(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpectedVersion uint64 `json:"expectedVersion"`
		review.DecisionInput
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if len(body.Checklist) == 0 && len(body.ConfirmedItemIDs) == 0 {
		writeError(w, errors.New("必须确认全部结构化复核清单项"))
		return
	}
	k, err := key(r)
	if err != nil {
		writeError(w, err)
		return
	}
	task, err := s.reviews.Decide(r.PathValue("id"), body.ExpectedVersion, body.DecisionInput, k)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}
func (s *Server) HandleIssueCertificate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpectedVersion uint64 `json:"expectedVersion"`
		IssuedBy        string `json:"issuedBy"`
		ValidMonths     int    `json:"validMonths"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	k, err := key(r)
	if err != nil {
		writeError(w, err)
		return
	}
	cert, err := s.certificates.Issue(r.PathValue("id"), body.ExpectedVersion, body.IssuedBy, k, body.ValidMonths)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, cert)
}
func (s *Server) HandleListCertificates(w http.ResponseWriter, r *http.Request) {
	items := s.certificates.List()
	limit := parseLimit(r)
	if len(items) > limit {
		items = items[len(items)-limit:]
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (s *Server) HandleGetCertificate(w http.ResponseWriter, r *http.Request) {
	cert, _, err := s.certificates.Get(r.PathValue("number"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cert)
}
func (s *Server) HandleVerifyCertificate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		VerificationCode string `json:"verificationCode"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(body.VerificationCode) == "" {
		writeError(w, errors.New("verificationCode 不能为空"))
		return
	}
	result, err := s.certificates.Verify(r.PathValue("number"), body.VerificationCode)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) HandleVoidCertificate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpectedVersion uint64 `json:"expectedVersion"`
		Reason          string `json:"reason"`
		Actor           string `json:"actor"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	k, err := key(r)
	if err != nil {
		writeError(w, err)
		return
	}
	cert, err := s.certificates.Void(r.PathValue("number"), body.ExpectedVersion, body.Reason, body.Actor, k)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cert)
}
func (s *Server) HandleAudit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	events := s.store.EventsFor(id)
	limit := parseLimit(r)
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": events})
}
func (s *Server) HandleIntegrity(w http.ResponseWriter, r *http.Request) {
	report, err := s.store.Inspect()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"store": report, "certificateChain": s.certificates.VerifyChain()})
}
