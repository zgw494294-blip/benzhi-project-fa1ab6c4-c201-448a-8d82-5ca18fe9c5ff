package review

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/calibration"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/domain"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/measurement"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
)

var (
	ErrDeviationOpen    = errors.New("仍有未完成的偏差处置")
	ErrReviewState      = errors.New("任务当前不可复核")
	ErrNoRevisionChange = errors.New("退回后没有实质测量或整改变化")
)

func New(s *store.Store, c *calibration.Service, m *measurement.Service) *Service {
	return &Service{store: s, calibration: c, measurement: m, now: time.Now}
}

func (s *Service) Deviations(taskID string) []domain.DeviationCase {
	task, err := s.calibration.Get(taskID)
	if err != nil {
		return nil
	}
	byID := map[string]domain.DeviationCase{}
	for _, point := range s.measurement.CurrentList(taskID) {
		if point.Judgement != "合格" {
			category := "超差"
			if point.ObservedValue == nil {
				category = "数据缺失"
			}
			id := "DEV-" + point.ID
			byID[point.ID] = domain.DeviationCase{ID: id, TaskID: taskID, Revision: task.Revision, MeasurementID: point.ID, Category: category, Status: "待整改"}
		}
	}
	for _, event := range s.store.EventsFor(taskID) {
		if event.Type != "DeviationRemediated" {
			continue
		}
		var item domain.DeviationCase
		if store.DecodePayload(event, &item) == nil {
			if _, ok := byID[item.MeasurementID]; ok {
				item.Revision = event.AggregateVersion
				byID[item.MeasurementID] = item
			}
		}
	}
	result := make([]domain.DeviationCase, 0, len(byID))
	for _, item := range byID {
		result = append(result, item)
	}
	return result
}
func (s *Service) DeviationStats(taskID string) map[string]DeviationCount {
	result := map[string]DeviationCount{"超差": {}, "数据缺失": {}}
	for _, item := range s.Deviations(taskID) {
		count := result[item.Category]
		if item.Status == "已整改" {
			count.Closed++
		} else {
			count.Open++
		}
		result[item.Category] = count
	}
	return result
}

func (s *Service) Remediate(taskID string, expected uint64, input RemediationInput, key string) (domain.DeviationCase, error) {
	task, err := s.calibration.Get(taskID)
	if err != nil {
		return domain.DeviationCase{}, err
	}
	if task.Revision != expected {
		return domain.DeviationCase{}, store.NewVersionConflict(task.Revision)
	}
	if task.Status != domain.StatusPending && task.Status != domain.StatusReturned {
		return domain.DeviationCase{}, ErrReviewState
	}
	if strings.TrimSpace(input.Cause) == "" || strings.TrimSpace(input.Correction) == "" || strings.TrimSpace(input.Evidence) == "" || strings.TrimSpace(input.Actor) == "" {
		return domain.DeviationCase{}, errors.New("偏差原因、纠正措施、证据摘要和操作人均为必填")
	}
	var target *domain.DeviationCase
	for _, item := range s.Deviations(taskID) {
		if item.MeasurementID == input.MeasurementID {
			copy := item
			target = &copy
			break
		}
	}
	if target == nil {
		return domain.DeviationCase{}, errors.New("指定测量点没有偏差")
	}
	target.Cause = input.Cause
	target.Correction = input.Correction
	target.Evidence = input.Evidence
	target.Status = "已整改"
	target.Revision = expected + 1
	_, _, err = s.store.AppendBatch(taskID, expected, key, store.ProposedEvent{AggregateID: taskID, Type: "DeviationRemediated", Actor: input.Actor, Payload: *target})
	return *target, err
}

func (s *Service) RemediateBatch(taskID string, expected uint64, input BatchRemediationInput, key string) ([]domain.DeviationCase, error) {
	if key != "" {
		events := s.store.EventsWithKey(key)
		if len(events) > 0 {
			items := make([]domain.DeviationCase, 0, len(events))
			wanted := map[string]bool{}
			for _, id := range input.MeasurementIDs {
				wanted[id] = true
			}
			for _, event := range events {
				if event.Type != "DeviationRemediated" {
					return nil, store.ErrDuplicateKey
				}
				var item domain.DeviationCase
				if store.DecodePayload(event, &item) != nil || !wanted[item.MeasurementID] || item.Cause != input.Cause || item.Correction != input.Correction || item.Evidence != input.Evidence || item.RemediatedBy != input.Actor {
					return nil, store.ErrDuplicateKey
				}
				items = append(items, item)
			}
			if len(items) != len(wanted) {
				return nil, store.ErrDuplicateKey
			}
			return items, nil
		}
	}
	task, err := s.calibration.Get(taskID)
	if err != nil {
		return nil, err
	}
	if task.Revision != expected {
		return nil, store.NewVersionConflict(task.Revision)
	}
	if task.Status != domain.StatusPending && task.Status != domain.StatusReturned {
		return nil, ErrReviewState
	}
	if len(input.MeasurementIDs) == 0 || strings.TrimSpace(input.Cause) == "" || strings.TrimSpace(input.Correction) == "" || strings.TrimSpace(input.Evidence) == "" || strings.TrimSpace(input.Actor) == "" {
		return nil, errors.New("批量整改必须选择偏差并填写原因、措施、证据和操作人")
	}
	selected := map[string]bool{}
	for _, id := range input.MeasurementIDs {
		selected[id] = true
	}
	items := s.Deviations(taskID)
	results := make([]domain.DeviationCase, 0, len(input.MeasurementIDs))
	batchID := fmt.Sprintf("DEV-BATCH-%d", s.now().UnixNano())
	seen := map[string]bool{}
	for _, item := range items {
		if !selected[item.MeasurementID] {
			continue
		}
		if item.Status != "待整改" || item.Revision > expected {
			return nil, errors.New("批量选择包含已整改或非当前修订偏差")
		}
		item.Cause = input.Cause
		item.Correction = input.Correction
		item.Evidence = input.Evidence
		item.Status = "已整改"
		item.Revision = expected + 1
		item.BatchID = batchID
		item.RemediatedBy = input.Actor
		item.RemediatedAt = s.now().UTC()
		results = append(results, item)
		seen[item.MeasurementID] = true
	}
	if len(results) != len(selected) {
		return nil, errors.New("批量选择包含不存在或已消失的偏差")
	}
	events := make([]store.ProposedEvent, 0, len(results))
	for _, item := range results {
		events = append(events, store.ProposedEvent{AggregateID: taskID, Type: "DeviationRemediated", Actor: input.Actor, Payload: item})
	}
	if _, _, err = s.store.AppendBatch(taskID, expected, key, events...); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *Service) Checklist(taskID string, expected uint64) (domain.ReviewChecklist, error) {
	task, err := s.calibration.Get(taskID)
	if err != nil {
		return domain.ReviewChecklist{}, err
	}
	if task.Revision != expected {
		return domain.ReviewChecklist{}, store.NewVersionConflict(task.Revision)
	}
	summary := s.measurement.Summary(taskID)
	coverage := s.measurement.Coverage(taskID)
	open := 0
	for _, d := range s.Deviations(taskID) {
		if d.Status != "已整改" {
			open++
		}
	}
	items := []domain.ReviewChecklistItem{{ID: "measurement_integrity", Name: "测量完整性", Status: "通过", Evidence: fmt.Sprintf("当前修订%d点，完整=%t", summary.Count, summary.Complete)}, {ID: "point_coverage", Name: "点位覆盖", Status: "通过", Evidence: fmt.Sprintf("覆盖率%.2f", coverage.Rate)}, {ID: "standard_validity", Name: "标准器有效性", Status: "通过", Evidence: "标准器均在有效期内"}, {ID: "deviation_closure", Name: "偏差闭环", Status: "通过", Evidence: fmt.Sprintf("待整改%d项", open)}, {ID: "reviewer_separation", Name: "复核员与建档人分离", Status: "待确认", Evidence: "复核提交时校验"}}
	if summary.Count == 0 || !summary.Complete {
		items[0].Status = "阻断"
		items[0].Blocking = true
	}
	if !coverage.Complete {
		items[1].Status = "阻断"
		items[1].Blocking = true
	}
	if err = s.calibration.ValidateStandards(taskID, s.now().UTC()); err != nil {
		items[2].Status = "阻断"
		items[2].Blocking = true
		items[2].Evidence = err.Error()
	}
	if open > 0 {
		items[3].Status = "阻断"
		items[3].Blocking = true
	}
	return domain.ReviewChecklist{Version: expected, Items: items, MeasurementSummary: domain.MeasurementSnapshot{Count: summary.Count, Qualified: summary.Qualified, Deviations: summary.Deviations, Complete: summary.Complete, Overall: summary.Overall}, Coverage: coverage}, nil
}

func (s *Service) Submit(taskID string, expected uint64, actor, key string) (domain.CalibrationTask, error) {
	task, err := s.calibration.Get(taskID)
	if err != nil {
		return domain.CalibrationTask{}, err
	}
	if task.Status == domain.StatusReturned {
		changed := false
		start := uint64(0)
		for _, event := range s.store.EventsFor(taskID) {
			if event.Type == "TaskRevisionCreated" {
				start = event.Sequence
			}
		}
		for _, event := range s.store.EventsFor(taskID) {
			if event.Sequence > start && (event.Type == "MeasurementAdded" || event.Type == "DeviationRemediated") {
				changed = true
			}
		}
		if !changed {
			return domain.CalibrationTask{}, ErrNoRevisionChange
		}
	}
	summary := s.measurement.Summary(taskID)
	if summary.Count == 0 || !summary.Complete {
		return domain.CalibrationTask{}, errors.New("至少需要一个完整测量点")
	}
	for _, d := range s.Deviations(taskID) {
		if d.Status != "已整改" {
			return domain.CalibrationTask{}, ErrDeviationOpen
		}
	}
	coverage := s.measurement.Coverage(taskID)
	if !coverage.Complete {
		return domain.CalibrationTask{}, fmt.Errorf("点位覆盖不足：已覆盖%d/%d", coverage.Completed, coverage.Required)
	}
	return s.calibration.SubmitForReview(taskID, expected, actor, key)
}

func (s *Service) Decide(taskID string, expected uint64, input DecisionInput, key string) (domain.CalibrationTask, error) {
	task, err := s.calibration.Get(taskID)
	if err != nil {
		return task, err
	}
	if task.Revision != expected {
		return task, store.NewVersionConflict(task.Revision)
	}
	if task.Status != domain.StatusReview {
		return task, ErrReviewState
	}
	if strings.TrimSpace(input.Reviewer) == "" || strings.TrimSpace(input.Comment) == "" {
		return task, errors.New("复核人和复核意见均为必填")
	}
	if input.Decision != "通过" && input.Decision != "退回" {
		return task, errors.New("复核决定只能为通过或退回")
	}
	if strings.EqualFold(strings.TrimSpace(input.Reviewer), strings.TrimSpace(task.CreatedBy)) {
		return task, errors.New("同行复核员不能与任务创建人相同")
	}
	checklist, err := s.Checklist(taskID, expected)
	if err != nil {
		return task, err
	}
	for _, item := range checklist.Items {
		if item.Blocking && input.Decision == "通过" {
			return task, fmt.Errorf("复核清单存在系统阻断项：%s", item.Name)
		}
	}
	for i := range checklist.Items {
		if checklist.Items[i].ID == "reviewer_separation" {
			checklist.Items[i].Status = "通过"
			checklist.Items[i].Evidence = fmt.Sprintf("复核员%s与建档人%s不同", input.Reviewer, task.CreatedBy)
		}
	}
	if input.ChecklistVersion != 0 && input.ChecklistVersion != expected {
		return task, store.NewVersionConflict(task.Revision)
	}
	if len(input.ConfirmedItemIDs) > 0 {
		confirmed := map[string]bool{}
		for _, id := range input.ConfirmedItemIDs {
			confirmed[id] = true
		}
		for _, item := range checklist.Items {
			if !confirmed[item.ID] {
				return task, errors.New("必须确认全部复核清单项")
			}
		}
	}
	if len(input.Checklist) > 0 {
		for _, item := range checklist.Items {
			found := false
			for _, given := range input.Checklist {
				if given.ID == item.ID {
					found = true
					if given.Status != "通过" && item.Blocking {
						return task, errors.New("不能覆盖系统阻断复核项")
					}
					break
				}
			}
			if !found {
				return task, errors.New("必须确认全部复核清单项")
			}
		}
		if input.Decision == "退回" && len(input.ReturnItems) == 0 {
			return task, errors.New("退回至少选择一个复核清单项")
		}
	}
	if input.Decision == "退回" {
		valid := map[string]bool{}
		for _, item := range checklist.Items {
			valid[item.ID] = true
		}
		for _, item := range input.ReturnItems {
			if !valid[item.ChecklistItemID] || strings.TrimSpace(item.Comment) == "" {
				return task, errors.New("退回项必须属于当前清单并填写针对性意见")
			}
		}
	}
	decision := domain.ReviewDecision{TaskID: taskID, Revision: expected + 1, Reviewer: input.Reviewer, Decision: input.Decision, Comment: input.Comment, MeasurementDigest: s.measurement.DeterministicSummary(taskID).Digest, CreatedAt: s.now().UTC(), Checklist: checklist, ReturnItems: input.ReturnItems}
	events := []store.ProposedEvent{{AggregateID: taskID, Type: "ReviewRecorded", Actor: input.Reviewer, Payload: decision}}
	if input.Decision == "退回" {
		task.Status = domain.StatusReturned
		task.ReviewComment = input.Comment
		task.ReturnItems = input.ReturnItems
		task.UpdatedAt = s.now().UTC()
		events = append(events, store.ProposedEvent{AggregateID: taskID, Type: "TaskRevisionCreated", Actor: input.Reviewer, Payload: task})
	}
	_, _, err = s.store.AppendBatch(taskID, expected, key, events...)
	if err != nil {
		return domain.CalibrationTask{}, err
	}
	task.Revision = expected + 1
	return task, nil
}

func (s *Service) Decisions(taskID string) []domain.ReviewDecision {
	result := make([]domain.ReviewDecision, 0)
	for _, e := range s.store.EventsFor(taskID) {
		if e.Type == "ReviewRecorded" {
			var d domain.ReviewDecision
			if store.DecodePayload(e, &d) == nil {
				result = append(result, d)
			}
		}
	}
	return result
}
func (s *Service) Approved(taskID string, revision uint64) bool {
	decisions := s.Decisions(taskID)
	if len(decisions) == 0 {
		return false
	}
	last := decisions[len(decisions)-1]
	return last.Decision == "通过" && last.Revision == revision
}
func (s *Service) EnsureReady(taskID string, revision uint64) error {
	for _, d := range s.Deviations(taskID) {
		if d.Status != "已整改" {
			return ErrDeviationOpen
		}
	}
	if !s.Approved(taskID, revision) {
		return fmt.Errorf("%w: 缺少当前版本的通过记录", ErrReviewState)
	}
	return nil
}

func (s *Service) Readiness(taskID string) Readiness {
	summary := s.measurement.Summary(taskID)
	result := Readiness{Ready: true, MeasurementCount: summary.Count, Reasons: make([]string, 0)}
	result.Coverage = s.measurement.Coverage(taskID)
	if !result.Coverage.Complete {
		result.Ready = false
		result.Reasons = append(result.Reasons, "必测点位覆盖不足")
	}
	if summary.Count == 0 {
		result.Ready = false
		result.Reasons = append(result.Reasons, "没有当前修订测量点")
	}
	if !summary.Complete {
		result.Ready = false
		result.Reasons = append(result.Reasons, "当前修订存在不完整测量数据")
	}
	for _, item := range s.Deviations(taskID) {
		if item.Status != "已整改" {
			result.OpenDeviations++
			result.Ready = false
		}
	}
	if result.OpenDeviations > 0 {
		result.Reasons = append(result.Reasons, "仍有未关闭偏差")
	}
	decisions := s.Decisions(taskID)
	if len(decisions) > 0 {
		result.LastDecision = decisions[len(decisions)-1].Decision
	}
	if checklist, err := s.Checklist(taskID, func() uint64 {
		if task, e := s.calibration.Get(taskID); e == nil {
			return task.Revision
		}
		return 0
	}()); err == nil {
		result.Checklist = checklist
	}
	return result
}

func (s *Service) DeviationHistory(taskID string) []domain.DeviationCase {
	result := make([]domain.DeviationCase, 0)
	for _, event := range s.store.EventsFor(taskID) {
		if event.Type == "DeviationRemediated" {
			var item domain.DeviationCase
			if store.DecodePayload(event, &item) == nil {
				item.Revision = event.AggregateVersion
				result = append(result, item)
			}
		}
	}
	return result
}
