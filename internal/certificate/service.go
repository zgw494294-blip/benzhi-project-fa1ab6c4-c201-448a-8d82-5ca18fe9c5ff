package certificate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/calibration"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/domain"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/measurement"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/review"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
)

var (
	ErrNotFound = errors.New("证书不存在")
	ErrNotReady = errors.New("任务尚未满足签发条件")
)

func New(s *store.Store, c *calibration.Service, m *measurement.Service, r *review.Service) *Service {
	return &Service{store: s, calibration: c, measurement: m, review: r, now: time.Now, verifyCache: make(map[string]Verification)}
}

func (s *Service) Issue(taskID string, expected uint64, issuedBy, key string, validMonths int) (domain.CalibrationCertificate, error) {
	if strings.TrimSpace(issuedBy) == "" {
		return domain.CalibrationCertificate{}, errors.New("签发人不能为空")
	}
	if validMonths <= 0 || validMonths > 120 {
		return domain.CalibrationCertificate{}, errors.New("有效月数必须为 1 到 120")
	}
	task, err := s.calibration.Get(taskID)
	if err != nil {
		return domain.CalibrationCertificate{}, err
	}
	if task.Revision != expected {
		return domain.CalibrationCertificate{}, store.NewVersionConflict(task.Revision)
	}
	if task.Status != domain.StatusReview {
		return domain.CalibrationCertificate{}, ErrNotReady
	}
	if err = s.calibration.ValidateStandards(taskID, s.now().UTC()); err != nil {
		return domain.CalibrationCertificate{}, fmt.Errorf("%w: %v", ErrNotReady, err)
	}
	if err = s.review.EnsureReady(taskID, expected); err != nil {
		return domain.CalibrationCertificate{}, fmt.Errorf("%w: %v", ErrNotReady, err)
	}
	points := s.measurement.CurrentList(taskID)
	if len(points) == 0 {
		return domain.CalibrationCertificate{}, ErrNotReady
	}
	deviations := s.review.Deviations(taskID)
	now := s.now().UTC().Truncate(time.Second)
	number := fmt.Sprintf("CERT-%s-R%d", strings.TrimPrefix(taskID, "CAL-"), expected)
	frozen := task
	frozen.Status = domain.StatusFrozen
	frozen.UpdatedAt = now
	frozen.Revision = expected + 1
	coverage := s.measurement.Coverage(taskID)
	checklist, _ := s.review.Checklist(taskID, expected)
	mat := material{CertificateNo: number, TaskID: taskID, Revision: expected + 1, IssuedBy: issuedBy, IssuedAt: now, ValidUntil: now.AddDate(0, validMonths, 0), Task: frozen, Measurements: points, Deviations: deviations, PreviousCertificateHash: s.lastDigest(), Coverage: coverage, ReviewChecklist: checklist}
	digest := digestMaterial(mat)
	cert := domain.CalibrationCertificate{CertificateNo: number, TaskID: taskID, Revision: expected + 1, ResultDigest: digest, IssuedBy: issuedBy, IssuedAt: mat.IssuedAt, ValidUntil: mat.ValidUntil, VerificationCode: verificationCode(digest), Status: "已签发", Task: frozen, Measurements: points, Deviations: deviations, MeasurementDigest: s.measurement.DeterministicSummary(taskID).Digest, Coverage: coverage, ReviewChecklist: checklist}
	events := []store.ProposedEvent{{AggregateID: taskID, Type: "CertificateIssued", Actor: issuedBy, Payload: struct {
		Certificate             domain.CalibrationCertificate `json:"certificate"`
		PreviousCertificateHash string                        `json:"previousCertificateHash"`
	}{cert, mat.PreviousCertificateHash}}, {AggregateID: taskID, Type: "TaskFrozen", Actor: issuedBy, Payload: frozen}}
	_, _, err = s.store.AppendBatch(taskID, expected, key, events...)
	if err != nil {
		return domain.CalibrationCertificate{}, err
	}
	return cert, nil
}

func (s *Service) Get(number string) (domain.CalibrationCertificate, string, error) {
	var found domain.CalibrationCertificate
	previous := ""
	ok := false
	for _, event := range s.store.Events() {
		if event.Type == "CertificateIssued" {
			var payload struct {
				Certificate             domain.CalibrationCertificate `json:"certificate"`
				PreviousCertificateHash string                        `json:"previousCertificateHash"`
			}
			if store.DecodePayload(event, &payload) == nil && payload.Certificate.CertificateNo == number {
				found = payload.Certificate
				previous = payload.PreviousCertificateHash
				ok = true
			}
		}
		if event.Type == "CertificateVoided" {
			var record VoidRecord
			if store.DecodePayload(event, &record) == nil && record.CertificateNo == number {
				found.Status = "已作废"
				found.VoidedReason = record.Reason
				found.VoidedBy = record.Actor
				t := record.VoidedAt
				found.VoidedAt = &t
			}
		}
	}
	if ok {
		return found, previous, nil
	}
	return domain.CalibrationCertificate{}, "", ErrNotFound
}
func (s *Service) Verify(number, code string) (Verification, error) {
	cacheKey := strings.ToUpper(strings.TrimSpace(number)) + "|" + strings.ToUpper(strings.TrimSpace(code))
	s.verifyMu.RLock()
	cached, found := s.verifyCache[cacheKey]
	s.verifyMu.RUnlock()
	if found {
		return cached, nil
	}
	s.verifyMu.Lock()
	defer s.verifyMu.Unlock()
	if cached, found := s.verifyCache[cacheKey]; found {
		return cached, nil
	}
	cert, previous, err := s.Get(number)
	if err != nil {
		return Verification{}, err
	}
	mat := material{CertificateNo: cert.CertificateNo, TaskID: cert.TaskID, Revision: cert.Revision, IssuedBy: cert.IssuedBy, IssuedAt: cert.IssuedAt, ValidUntil: cert.ValidUntil, Task: cert.Task, Measurements: cert.Measurements, Deviations: cert.Deviations, PreviousCertificateHash: previous, Coverage: cert.Coverage, ReviewChecklist: cert.ReviewChecklist}
	digestOK := digestMaterial(mat) == cert.ResultDigest
	codeOK := strings.EqualFold(code, cert.VerificationCode) && verificationCode(cert.ResultDigest) == cert.VerificationCode
	issued := cert.Status == "已签发" || cert.Status == "已作废"
	voided := cert.Status == "已作废"
	within := !s.now().UTC().After(cert.ValidUntil)
	valid := digestOK && codeOK && issued && within && !voided
	message := "证书校验通过"
	if !valid {
		message = "证书校验未通过"
	}
	if voided {
		message = "证书已作废"
	}
	result := Verification{Certificate: cert, DigestValid: digestOK, CodeValid: codeOK, Issued: issued, WithinValidity: within, Valid: valid, Message: message, Voided: voided}
	s.verifyCache[cacheKey] = result
	return result, nil
}

func (s *Service) invalidateVerifyCache(number string) {
	prefix := strings.ToUpper(strings.TrimSpace(number)) + "|"
	s.verifyMu.Lock()
	defer s.verifyMu.Unlock()
	for key := range s.verifyCache {
		if strings.HasPrefix(key, prefix) {
			delete(s.verifyCache, key)
		}
	}
}
func (s *Service) List() []domain.CalibrationCertificate {
	result := make([]domain.CalibrationCertificate, 0)
	for _, event := range s.store.Events() {
		if event.Type == "CertificateIssued" {
			var payload struct {
				Certificate domain.CalibrationCertificate `json:"certificate"`
			}
			if store.DecodePayload(event, &payload) == nil {
				if current, _, err := s.Get(payload.Certificate.CertificateNo); err == nil {
					result = append(result, current)
				}
			}
		}
	}
	return result
}

func (s *Service) Void(number string, expected uint64, reason, actor, key string) (domain.CalibrationCertificate, error) {
	if strings.TrimSpace(reason) == "" || strings.TrimSpace(actor) == "" {
		return domain.CalibrationCertificate{}, errors.New("作废原因和操作人不能为空")
	}
	for _, e := range s.store.EventsWithKey(key) {
		if e.Type == "CertificateVoided" {
			var old VoidRecord
			if store.DecodePayload(e, &old) == nil && old.CertificateNo == number && old.Reason == reason && old.Actor == actor {
				cert, _, err := s.Get(number)
				if err == nil {
					s.invalidateVerifyCache(number)
				}
				return cert, err
			}
			return domain.CalibrationCertificate{}, store.ErrDuplicateKey
		}
	}
	cert, _, err := s.Get(number)
	if err != nil {
		return cert, err
	}
	if cert.Status != "已签发" {
		return cert, errors.New("证书当前状态不可作废")
	}
	current, err := s.calibration.Get(cert.TaskID)
	if err != nil {
		return cert, err
	}
	if current.Revision != expected {
		return cert, store.NewVersionConflict(current.Revision)
	}
	record := VoidRecord{CertificateNo: number, Reason: strings.TrimSpace(reason), Actor: strings.TrimSpace(actor), VoidedAt: s.now().UTC()}
	if _, _, err = s.store.AppendBatch(cert.TaskID, expected, key, store.ProposedEvent{AggregateID: cert.TaskID, Type: "CertificateVoided", Actor: actor, Payload: record}); err != nil {
		return domain.CalibrationCertificate{}, err
	}
	s.invalidateVerifyCache(number)
	cert.Status = "已作废"
	cert.VoidedReason = record.Reason
	cert.VoidedBy = record.Actor
	cert.VoidedAt = &record.VoidedAt
	return cert, nil
}

func (s *Service) VerifyChain() ChainReport {
	events := s.store.Events()
	result := ChainReport{Continuous: true}
	for _, e := range events {
		if e.Type == "CertificateIssued" {
			result.Total++
		}
	}
	previous := ""
	seen := map[string]bool{}
	for _, e := range events {
		if e.Type != "CertificateIssued" {
			continue
		}
		result.Checked++
		var p struct {
			Certificate             domain.CalibrationCertificate `json:"certificate"`
			PreviousCertificateHash string                        `json:"previousCertificateHash"`
		}
		if store.DecodePayload(e, &p) != nil {
			result.Continuous = false
			result.EventSequence = e.Sequence
			result.AnomalyType = "签发事件载荷无法解析"
			return result
		}
		result.FirstAnomalyCertificate = p.Certificate.CertificateNo
		if seen[p.Certificate.CertificateNo] {
			result.Continuous = false
			result.EventSequence = e.Sequence
			result.AnomalyType = "重复证书编号"
			return result
		}
		seen[p.Certificate.CertificateNo] = true
		mat := material{CertificateNo: p.Certificate.CertificateNo, TaskID: p.Certificate.TaskID, Revision: p.Certificate.Revision, IssuedBy: p.Certificate.IssuedBy, IssuedAt: p.Certificate.IssuedAt, ValidUntil: p.Certificate.ValidUntil, Task: p.Certificate.Task, Measurements: p.Certificate.Measurements, Deviations: p.Certificate.Deviations, PreviousCertificateHash: p.PreviousCertificateHash, Coverage: p.Certificate.Coverage, ReviewChecklist: p.Certificate.ReviewChecklist}
		if digestMaterial(mat) != p.Certificate.ResultDigest {
			result.Continuous = false
			result.EventSequence = e.Sequence
			result.AnomalyType = "证书内容摘要错误"
			return result
		}
		if p.PreviousCertificateHash != previous {
			result.Continuous = false
			result.EventSequence = e.Sequence
			result.AnomalyType = "前序引用错误"
			return result
		}
		previous = p.Certificate.ResultDigest
		result.FirstAnomalyCertificate = ""
	}
	return result
}
func (s *Service) lastDigest() string {
	items := s.List()
	if len(items) == 0 {
		return ""
	}
	return items[len(items)-1].ResultDigest
}
func digestMaterial(value material) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func verificationCode(digest string) string {
	sum := sha256.Sum256([]byte("CERT-V1|" + digest))
	return strings.ToUpper(hex.EncodeToString(sum[:10]))
}
