package calibration

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/domain"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
)

var (
	ErrNotFound  = errors.New("校准任务不存在")
	ErrInvalid   = errors.New("校准任务参数无效")
	ErrState     = errors.New("校准任务状态不允许该操作")
	ErrInTransit = errors.New("同一仪器已有在途校准任务")
)

func (e *TransitConflict) Error() string {
	return fmt.Sprintf("%s: 任务%s，状态%s，更新时间%s", ErrInTransit, e.TaskID, e.Status, e.UpdatedAt.Format(time.RFC3339))
}
func (e *TransitConflict) Unwrap() error { return ErrInTransit }

func New(s *store.Store) *Service { return &Service{store: s, now: time.Now} }

func (s *Service) Create(input CreateInput, key string) (domain.CalibrationTask, error) {
	if key != "" {
		for _, event := range s.store.EventsWithKey(key) {
			if event.Type != "TaskCreated" {
				return domain.CalibrationTask{}, store.ErrDuplicateKey
			}
			var task domain.CalibrationTask
			if store.DecodePayload(event, &task) != nil {
				return domain.CalibrationTask{}, store.ErrDuplicateKey
			}
			if !sameCreateInput(task, input) {
				return domain.CalibrationTask{}, store.ErrDuplicateKey
			}
			return task, nil
		}
	}
	if strings.TrimSpace(input.StationCode) == "" || strings.TrimSpace(input.InstrumentID) == "" || strings.TrimSpace(input.InstrumentType) == "" || input.RangeMax <= input.RangeMin || input.RangeMin < 0 || len(input.ReferenceStandards) == 0 || strings.TrimSpace(input.CreatedBy) == "" {
		return domain.CalibrationTask{}, fmt.Errorf("%w: 台站、仪器、标准器、范围和创建人均为必填", ErrInvalid)
	}
	for _, standard := range input.ReferenceStandards {
		if strings.TrimSpace(standard.ID) == "" || strings.TrimSpace(standard.Name) == "" || strings.TrimSpace(standard.CertificateNo) == "" {
			return domain.CalibrationTask{}, fmt.Errorf("%w: 标准器编号、名称和证书号不能为空", ErrInvalid)
		}
		if !standard.ValidUntil.After(s.now()) {
			return domain.CalibrationTask{}, fmt.Errorf("%w: 标准器 %s 已过有效期", ErrInvalid, standard.Name)
		}
	}
	station, instrument := normalize(input.StationCode), normalize(input.InstrumentID)
	var frozen *domain.CertificateSummary
	for _, existing := range s.List() {
		if normalize(existing.StationCode) != station || normalize(existing.InstrumentID) != instrument {
			continue
		}
		if existing.Status == domain.StatusPending || existing.Status == domain.StatusReview || existing.Status == domain.StatusReturned {
			return domain.CalibrationTask{}, &TransitConflict{TaskID: existing.ID, Status: existing.Status, UpdatedAt: existing.UpdatedAt}
		}
		if existing.Status == domain.StatusFrozen {
			for _, event := range s.store.EventsFor(existing.ID) {
				if event.Type != "CertificateIssued" {
					continue
				}
				var p struct {
					Certificate domain.CalibrationCertificate `json:"certificate"`
				}
				if store.DecodePayload(event, &p) == nil {
					summary := domain.CertificateSummary{CertificateNo: p.Certificate.CertificateNo, IssuedAt: p.Certificate.IssuedAt, ValidUntil: p.Certificate.ValidUntil}
					if frozen == nil || summary.IssuedAt.After(frozen.IssuedAt) {
						frozen = &summary
					}
				}
			}
		}
	}
	id := fmt.Sprintf("CAL-%d", s.now().UnixNano())
	task := domain.CalibrationTask{ID: id, StationCode: input.StationCode, InstrumentID: input.InstrumentID, InstrumentType: input.InstrumentType, ReferenceStandards: input.ReferenceStandards, RangeMin: input.RangeMin, RangeMax: input.RangeMax, Revision: 1, Status: domain.StatusPending, CreatedBy: input.CreatedBy, UpdatedAt: s.now().UTC()}
	if frozen != nil {
		task.LastFrozenCertificate = frozen
	}
	_, _, err := s.store.AppendBatch(id, 0, key, store.ProposedEvent{AggregateID: id, Type: "TaskCreated", Actor: input.CreatedBy, Payload: task})
	return task, err
}

func normalize(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func (s *Service) SetRequiredPoints(id string, expected uint64, points []domain.RequiredPoint, actor, key string) (domain.CalibrationTask, error) {
	task, err := s.Get(id)
	if err != nil {
		return task, err
	}
	if task.Revision != expected {
		return task, store.NewVersionConflict(task.Revision)
	}
	if task.Status == domain.StatusFrozen {
		return task, fmt.Errorf("%w: 已冻结任务点位方案只读", ErrState)
	}
	if strings.TrimSpace(actor) == "" || len(points) == 0 {
		return task, fmt.Errorf("%w: 必测点位不能为空", ErrInvalid)
	}
	seen := map[string]bool{}
	for i := range points {
		points[i].Label = strings.TrimSpace(points[i].Label)
		if points[i].Label == "" || seen[normalize(points[i].Label)] || points[i].Value < task.RangeMin || points[i].Value > task.RangeMax {
			return task, fmt.Errorf("%w: 点位标签重复、为空或超出范围", ErrInvalid)
		}
		if math.IsNaN(points[i].Value) || math.IsInf(points[i].Value, 0) {
			return task, ErrInvalid
		}
		for j := 0; j < i; j++ {
			if nearlyEqual(points[i].Value, points[j].Value) {
				return task, fmt.Errorf("%w: 必测标准值重复", ErrInvalid)
			}
		}
		seen[normalize(points[i].Label)] = true
	}
	task.RequiredPoints = points
	task.Revision = expected + 1
	task.UpdatedAt = s.now().UTC()
	_, _, err = s.store.AppendBatch(id, expected, key, store.ProposedEvent{AggregateID: id, Type: "RequiredPointsSet", Actor: actor, Payload: task})
	return task, err
}

func nearlyEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

func sameCreateInput(task domain.CalibrationTask, input CreateInput) bool {
	if task.StationCode != input.StationCode || task.InstrumentID != input.InstrumentID || task.InstrumentType != input.InstrumentType || task.RangeMin != input.RangeMin || task.RangeMax != input.RangeMax || task.CreatedBy != input.CreatedBy || len(task.ReferenceStandards) != len(input.ReferenceStandards) {
		return false
	}
	for index := range task.ReferenceStandards {
		left, right := task.ReferenceStandards[index], input.ReferenceStandards[index]
		if left.ID != right.ID || left.Name != right.Name || left.CertificateNo != right.CertificateNo || !left.ValidUntil.Equal(right.ValidUntil) {
			return false
		}
	}
	return true
}

func (s *Service) Get(id string) (domain.CalibrationTask, error) {
	events := s.store.EventsFor(id)
	if len(events) == 0 {
		return domain.CalibrationTask{}, ErrNotFound
	}
	var task domain.CalibrationTask
	for _, event := range events {
		switch event.Type {
		case "TaskCreated", "TaskRevisionCreated", "TaskSubmitted", "TaskFrozen", "RequiredPointsSet":
			if err := store.DecodePayload(event, &task); err != nil {
				return task, err
			}
		}
		task.Revision = event.AggregateVersion
	}
	return task, nil
}

func (s *Service) List() []domain.CalibrationTask {
	ids := map[string]struct{}{}
	for _, e := range s.store.Events() {
		ids[e.AggregateID] = struct{}{}
	}
	result := make([]domain.CalibrationTask, 0, len(ids))
	for id := range ids {
		if t, err := s.Get(id); err == nil {
			result = append(result, t)
		}
	}
	return result
}

func (s *Service) Search(query Query) []domain.CalibrationTask {
	all := s.List()
	result := make([]domain.CalibrationTask, 0, len(all))
	for _, task := range all {
		if query.Status != "" && task.Status != query.Status {
			continue
		}
		if query.StationCode != "" && !strings.EqualFold(task.StationCode, query.StationCode) {
			continue
		}
		needle := strings.ToLower(strings.TrimSpace(query.Instrument))
		if needle != "" && !strings.Contains(strings.ToLower(task.InstrumentID+" "+task.InstrumentType), needle) {
			continue
		}
		result = append(result, task)
	}
	return result
}

func (s *Service) History(id string) ([]HistoryEntry, error) {
	if _, err := s.Get(id); err != nil {
		return nil, err
	}
	result := make([]HistoryEntry, 0)
	for _, event := range s.store.EventsFor(id) {
		var task domain.CalibrationTask
		switch event.Type {
		case "TaskCreated", "TaskSubmitted", "TaskRevisionCreated", "TaskFrozen":
			if store.DecodePayload(event, &task) != nil {
				continue
			}
			result = append(result, HistoryEntry{Sequence: event.Sequence, Version: event.AggregateVersion, EventType: event.Type, Actor: event.Actor, OccurredAt: event.OccurredAt, Status: task.Status, ReviewComment: task.ReviewComment})
		}
	}
	return result, nil
}

func (s *Service) ValidateStandards(id string, at time.Time) error {
	task, err := s.Get(id)
	if err != nil {
		return err
	}
	for _, standard := range task.ReferenceStandards {
		if !standard.ValidUntil.After(at) {
			return fmt.Errorf("%w: 标准器 %s 的证书 %s 已过期", ErrInvalid, standard.Name, standard.CertificateNo)
		}
	}
	return nil
}

func (s *Service) SubmitForReview(id string, expected uint64, actor, key string) (domain.CalibrationTask, error) {
	task, err := s.Get(id)
	if err != nil {
		return task, err
	}
	if task.Revision != expected {
		return task, store.NewVersionConflict(task.Revision)
	}
	if task.Status != domain.StatusPending && task.Status != domain.StatusReturned {
		return task, fmt.Errorf("%w: 当前状态为%s", ErrState, task.Status)
	}
	task.Status = domain.StatusReview
	task.UpdatedAt = s.now().UTC()
	_, _, err = s.store.AppendBatch(id, expected, key, store.ProposedEvent{AggregateID: id, Type: "TaskSubmitted", Actor: actor, Payload: task})
	if err != nil {
		return domain.CalibrationTask{}, err
	}
	task.Revision = expected + 1
	return task, nil
}

func (s *Service) Freeze(id string, expected uint64, actor, key string) (domain.CalibrationTask, error) {
	task, err := s.Get(id)
	if err != nil {
		return task, err
	}
	if task.Revision != expected {
		return task, store.NewVersionConflict(task.Revision)
	}
	if task.Status != domain.StatusReview {
		return task, fmt.Errorf("%w: 仅待复核任务可冻结", ErrState)
	}
	task.Status = domain.StatusFrozen
	task.UpdatedAt = s.now().UTC()
	_, _, err = s.store.AppendBatch(id, expected, key, store.ProposedEvent{AggregateID: id, Type: "TaskFrozen", Actor: actor, Payload: task})
	if err != nil {
		return domain.CalibrationTask{}, err
	}
	task.Revision = expected + 1
	return task, nil
}
