package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/calibration"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/certificate"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/measurement"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/review"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
)

func NewServer(s *store.Store) *Server {
	c := calibration.New(s)
	m := measurement.New(s, c)
	r := review.New(s, c, m)
	cert := certificate.New(s, c, m, r)
	server := &Server{calibrations: c, measurements: m, reviews: r, certificates: cert, store: s, mux: http.NewServeMux()}
	server.routes()
	return server
}
func (s *Server) Handler() http.Handler { return requestLog(recoverer(s.mux)) }

func (s *Server) routes() {
	assets, _ := fs.Sub(webFiles, "web")
	s.mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assets))))
	s.mux.HandleFunc("GET /", s.HandleHome)
	s.mux.HandleFunc("GET /calibrations", s.HandleHome)
	s.mux.HandleFunc("GET /health", s.HandleHealth)
	s.mux.HandleFunc("GET /api/calibrations", s.HandleListCalibrations)
	s.mux.HandleFunc("GET /api/calibrations/search", s.HandleSearchCalibrations)
	s.mux.HandleFunc("POST /api/calibrations", s.HandleCreateCalibration)
	s.mux.HandleFunc("GET /api/calibrations/{id}", s.HandleGetCalibration)
	s.mux.HandleFunc("GET /api/calibrations/{id}/measurements", s.HandleListMeasurements)
	s.mux.HandleFunc("GET /api/calibrations/{id}/findings", s.HandleFindings)
	s.mux.HandleFunc("POST /api/calibrations/{id}/measurements", s.HandleAddMeasurement)
	s.mux.HandleFunc("POST /api/calibrations/{id}/measurements/batch", s.HandleAddMeasurementBatch)
	s.mux.HandleFunc("POST /api/calibrations/{id}/measurements/batch/preflight", s.HandlePreflightMeasurementBatch)
	s.mux.HandleFunc("PUT /api/calibrations/{id}/required-points", s.HandleSetRequiredPoints)
	s.mux.HandleFunc("GET /api/calibrations/{id}/coverage", s.HandleCoverage)
	s.mux.HandleFunc("GET /api/calibrations/{id}/deviations", s.HandleListDeviations)
	s.mux.HandleFunc("POST /api/calibrations/{id}/deviations", s.HandleRemediateDeviationBatch)
	s.mux.HandleFunc("PUT /api/calibrations/{id}/deviations/{measurementID}", s.HandleRemediateDeviation)
	s.mux.HandleFunc("POST /api/calibrations/{id}/submit", s.HandleSubmitReview)
	s.mux.HandleFunc("GET /api/calibrations/{id}/reviews", s.HandleListReviews)
	s.mux.HandleFunc("GET /api/calibrations/{id}/reviews/checklist", s.HandleReviewChecklist)
	s.mux.HandleFunc("GET /api/calibrations/{id}/readiness", s.HandleReadiness)
	s.mux.HandleFunc("GET /api/calibrations/{id}/history", s.HandleTaskHistory)
	s.mux.HandleFunc("POST /api/calibrations/{id}/reviews", s.HandleReviewDecision)
	s.mux.HandleFunc("POST /api/calibrations/{id}/certificate", s.HandleIssueCertificate)
	s.mux.HandleFunc("GET /api/certificates", s.HandleListCertificates)
	s.mux.HandleFunc("GET /api/certificates/{number}", s.HandleGetCertificate)
	s.mux.HandleFunc("POST /api/certificates/{number}", s.HandleVoidCertificate)
	s.mux.HandleFunc("POST /api/certificates/{number}/verify", s.HandleVerifyCertificate)
	s.mux.HandleFunc("POST /api/certificates/{number}/void", s.HandleVoidCertificate)
	s.mux.HandleFunc("GET /api/calibrations/{id}/audit", s.HandleAudit)
	s.mux.HandleFunc("GET /api/system/integrity", s.HandleIntegrity)
}

func (s *Server) HandleHome(w http.ResponseWriter, r *http.Request) {
	data, err := webFiles.ReadFile("web/index.html")
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}
func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "events": len(s.store.Events())})
}

type apiError struct {
	Error struct {
		Code           string `json:"code"`
		Message        string `json:"message"`
		Details        any    `json:"details,omitempty"`
		CurrentVersion uint64 `json:"currentVersion,omitempty"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	code := "invalid_request"
	switch {
	case errors.Is(err, calibration.ErrNotFound), errors.Is(err, certificate.ErrNotFound):
		status = http.StatusNotFound
		code = "not_found"
	case errors.Is(err, store.ErrVersionConflict):
		status = http.StatusConflict
		code = "version_conflict"
	case errors.Is(err, store.ErrDuplicateKey):
		status = http.StatusConflict
		code = "idempotency_conflict"
	case errors.Is(err, calibration.ErrInTransit):
		status = http.StatusConflict
		code = "instrument_in_transit"
	case errors.Is(err, store.ErrCorruptLog), errors.Is(err, store.ErrStoreUnavailable):
		status = http.StatusInternalServerError
		code = "storage_error"
	}
	var response apiError
	response.Error.Code = code
	response.Error.Message = err.Error()
	var conflict *calibration.TransitConflict
	if errors.As(err, &conflict) {
		response.Error.Details = conflict
	}
	var version interface{ CurrentVersion() uint64 }
	if errors.As(err, &version) {
		response.Error.CurrentVersion = version.CurrentVersion()
	}
	writeJSON(w, status, response)
}
func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("请求 JSON 无效: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("请求只能包含一个 JSON 对象")
	}
	return nil
}
func expected(value uint64) error {
	if value == 0 {
		return errors.New("expectedVersion 必须大于零")
	}
	return nil
}
func key(r *http.Request) (string, error) {
	value := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if value == "" {
		return "", errors.New("Idempotency-Key 请求头不能为空")
	}
	if len(value) > 128 {
		return "", errors.New("Idempotency-Key 过长")
	}
	return value, nil
}
func parseLimit(r *http.Request) int {
	value, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if value < 1 || value > 500 {
		return 100
	}
	return value
}
func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("http request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("request panic", "error", recovered)
				writeError(w, errors.New("服务内部错误"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
