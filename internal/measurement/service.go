package measurement

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/calibration"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/domain"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
)

var ErrInvalid = errors.New("测量数据无效")

func New(s *store.Store, c *calibration.Service) *Service {
	return &Service{store: s, calibration: c, coverageCache: make(map[string]domain.CoverageSnapshot)}
}

func (s *Service) AddPoint(taskID string, expected uint64, input PointInput, key string) (domain.MeasurementPoint, error) {
	task, err := s.calibration.Get(taskID)
	if err != nil {
		return domain.MeasurementPoint{}, err
	}
	if task.Revision != expected {
		return domain.MeasurementPoint{}, store.NewVersionConflict(task.Revision)
	}
	if task.Status == domain.StatusFrozen {
		return domain.MeasurementPoint{}, errors.New("已冻结任务不可修改")
	}
	if strings.TrimSpace(input.PointLabel) == "" || strings.TrimSpace(input.EnteredBy) == "" || input.Tolerance <= 0 || input.Uncertainty < 0 || math.IsNaN(input.ReferenceValue) || math.IsInf(input.ReferenceValue, 0) {
		return domain.MeasurementPoint{}, ErrInvalid
	}
	if input.ReferenceValue < task.RangeMin || input.ReferenceValue > task.RangeMax {
		return domain.MeasurementPoint{}, fmt.Errorf("%w: 标准值超出校准范围", ErrInvalid)
	}
	points := s.List(taskID)
	for _, p := range points {
		if p.Revision == expected && p.PointLabel == input.PointLabel {
			return domain.MeasurementPoint{}, fmt.Errorf("%w: 测量点标签重复", ErrInvalid)
		}
	}
	point := domain.MeasurementPoint{ID: fmt.Sprintf("%s-M-%d", taskID, len(points)+1), TaskID: taskID, Revision: expected + 1, PointLabel: input.PointLabel, ReferenceValue: input.ReferenceValue, ObservedValue: input.ObservedValue, Unit: input.Unit, Uncertainty: input.Uncertainty, Tolerance: input.Tolerance, EnteredBy: input.EnteredBy}
	if input.ObservedValue == nil || strings.TrimSpace(input.Unit) == "" {
		point.Judgement = "缺失"
	} else {
		if math.IsNaN(*input.ObservedValue) || math.IsInf(*input.ObservedValue, 0) {
			return point, ErrInvalid
		}
		point.Error = *input.ObservedValue - input.ReferenceValue
		if math.Abs(point.Error)+input.Uncertainty <= input.Tolerance {
			point.Judgement = "合格"
		} else {
			point.Judgement = "超差"
		}
	}
	_, _, err = s.store.AppendBatch(taskID, expected, key, store.ProposedEvent{AggregateID: taskID, Type: "MeasurementAdded", Actor: input.EnteredBy, Payload: point})
	return point, err
}

func (s *Service) List(taskID string) []domain.MeasurementPoint {
	result := make([]domain.MeasurementPoint, 0)
	for _, e := range s.store.EventsFor(taskID) {
		if e.Type == "MeasurementAdded" {
			var p domain.MeasurementPoint
			if store.DecodePayload(e, &p) == nil {
				p.Revision = e.AggregateVersion
				result = append(result, p)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *Service) CurrentList(taskID string) []domain.MeasurementPoint {
	events := s.store.EventsFor(taskID)
	start := uint64(0)
	for _, e := range events {
		if e.Type == "TaskRevisionCreated" {
			start = e.Sequence
		}
	}
	result := make([]domain.MeasurementPoint, 0)
	for _, e := range events {
		if e.Sequence > start && e.Type == "MeasurementAdded" {
			var point domain.MeasurementPoint
			if store.DecodePayload(e, &point) == nil {
				point.Revision = e.AggregateVersion
				result = append(result, point)
			}
		}
	}
	return result
}

func (s *Service) Summary(taskID string) Summary {
	points := s.CurrentList(taskID)
	summary := Summary{Count: len(points), Complete: len(points) > 0, Overall: "合格"}
	for _, p := range points {
		if p.ObservedValue == nil || strings.TrimSpace(p.Unit) == "" {
			summary.Complete = false
		}
		if p.Judgement == "合格" {
			summary.Qualified++
		} else {
			summary.Deviations++
		}
	}
	if !summary.Complete || summary.Deviations > 0 {
		summary.Overall = "需整改"
	}
	return summary
}

func (s *Service) Coverage(taskID string) domain.CoverageSnapshot {
	task, err := s.calibration.Get(taskID)
	if err != nil || len(task.RequiredPoints) == 0 {
		return domain.CoverageSnapshot{Complete: true, Rate: 1}
	}
	s.coverageMu.RLock()
	cached, ok := s.coverageCache[taskID]
	s.coverageMu.RUnlock()
	if ok {
		return cloneCoverage(cached)
	}
	points := s.CurrentList(taskID)
	result := domain.CoverageSnapshot{Required: len(task.RequiredPoints), Covered: make([]domain.CoverageItem, 0), Missing: make([]domain.CoverageItem, 0), Duplicate: make([]domain.CoverageItem, 0)}
	for _, required := range task.RequiredPoints {
		item := domain.CoverageItem{Label: required.Label, Value: required.Value}
		for _, point := range points {
			if strings.EqualFold(strings.TrimSpace(point.PointLabel), strings.TrimSpace(required.Label)) && math.Abs(point.ReferenceValue-required.Value) <= 1e-9*math.Max(1, math.Abs(required.Value)) {
				item.MeasurementIDs = append(item.MeasurementIDs, point.ID)
			}
		}
		if len(item.MeasurementIDs) == 0 {
			item.Reason = "缺少读数、单位或判定"
			result.Missing = append(result.Missing, item)
		} else if len(item.MeasurementIDs) > 1 {
			item.Reason = "同一必测点被重复覆盖"
			result.Duplicate = append(result.Duplicate, item)
		} else {
			for _, point := range points {
				if point.ID == item.MeasurementIDs[0] && point.ObservedValue != nil && strings.TrimSpace(point.Unit) != "" && point.Judgement != "缺失" {
					item.Complete = true
				}
			}
			if item.Complete {
				result.Completed++
				result.Covered = append(result.Covered, item)
			} else {
				item.Reason = "缺少读数、单位或判定"
				result.Missing = append(result.Missing, item)
			}
		}
	}
	result.Rate = float64(result.Completed) / float64(result.Required)
	result.Complete = result.Completed == result.Required && len(result.Duplicate) == 0
	s.coverageMu.Lock()
	s.coverageCache[taskID] = cloneCoverage(result)
	s.coverageMu.Unlock()
	return result
}

func cloneCoverage(source domain.CoverageSnapshot) domain.CoverageSnapshot {
	result := source
	cloneItems := func(items []domain.CoverageItem) []domain.CoverageItem {
		cloned := make([]domain.CoverageItem, len(items))
		for index, item := range items {
			cloned[index] = item
			cloned[index].MeasurementIDs = append([]string(nil), item.MeasurementIDs...)
		}
		return cloned
	}
	result.Covered = cloneItems(source.Covered)
	result.Missing = cloneItems(source.Missing)
	result.Duplicate = cloneItems(source.Duplicate)
	return result
}

func (s *Service) PrecheckBatch(taskID string, expected uint64, actor string, inputs []PointInput) BatchPrecheck {
	result := BatchPrecheck{Points: make([]domain.MeasurementPoint, 0), Findings: make([]Finding, 0), Version: expected + 1, Valid: true}
	if strings.TrimSpace(actor) == "" {
		result.Valid = false
		result.Findings = append(result.Findings, Finding{Code: "ACTOR_MISSING", Severity: "阻断", Message: "录入人不能为空"})
		return result
	}
	// Reuse the deterministic validation path without writing; SubmitBatch performs the same checks again.
	task, err := s.calibration.Get(taskID)
	if err != nil {
		result.Valid = false
		result.Findings = append(result.Findings, Finding{Code: "TASK_NOT_FOUND", Severity: "阻断", Message: err.Error()})
		return result
	}
	if task.Revision != expected {
		result.Valid = false
		result.Findings = append(result.Findings, Finding{Code: "VERSION_CONFLICT", Severity: "阻断", Message: "当前版本已变化"})
		return result
	}
	existing := s.CurrentList(taskID)
	labels := map[string]bool{}
	for _, p := range existing {
		labels[p.PointLabel] = true
	}
	for i, input := range inputs {
		input.EnteredBy = actor
		id := fmt.Sprintf("%s-PREVIEW-%d", taskID, i+1)
		p := domain.MeasurementPoint{ID: id, TaskID: taskID, Revision: expected + 1, PointLabel: strings.TrimSpace(input.PointLabel), ReferenceValue: input.ReferenceValue, ObservedValue: input.ObservedValue, Unit: strings.TrimSpace(input.Unit), Uncertainty: input.Uncertainty, Tolerance: input.Tolerance, EnteredBy: actor}
		addFinding := func(code, severity, msg string) {
			result.Findings = append(result.Findings, Finding{Code: code, Severity: severity, MeasurementID: id, Message: fmt.Sprintf("第%d行：%s", i+1, msg), Row: i + 1})
			if severity == "阻断" {
				result.Valid = false
			}
		}
		if p.PointLabel == "" {
			addFinding("LABEL_MISSING", "阻断", "测量点标签不能为空")
		}
		if labels[p.PointLabel] && p.PointLabel != "" {
			addFinding("POINT_DUPLICATED", "阻断", "当前修订存在重复测量点")
		}
		labels[p.PointLabel] = true
		if math.IsNaN(p.ReferenceValue) || math.IsInf(p.ReferenceValue, 0) || p.ReferenceValue < task.RangeMin || p.ReferenceValue > task.RangeMax {
			addFinding("REFERENCE_OUT_OF_RANGE", "阻断", "标准值超出校准范围")
		}
		if p.Tolerance <= 0 || p.Uncertainty < 0 || math.IsNaN(p.Uncertainty) || math.IsInf(p.Uncertainty, 0) {
			addFinding("RULE_INVALID", "阻断", "容差和不确定度无效")
		}
		if p.Unit == "" {
			addFinding("UNIT_MISSING", "高", "测量点缺少单位")
		}
		if p.ObservedValue == nil {
			p.Judgement = "缺失"
			addFinding("OBSERVED_MISSING", "高", "实测读数缺失")
		} else if math.IsNaN(*p.ObservedValue) || math.IsInf(*p.ObservedValue, 0) {
			addFinding("OBSERVED_INVALID", "阻断", "实测读数不是有限数值")
		} else {
			p.Error = *p.ObservedValue - p.ReferenceValue
			if p.Unit == "" {
				p.Judgement = "缺失"
			} else if math.Abs(p.Error)+p.Uncertainty <= p.Tolerance {
				p.Judgement = "合格"
			} else {
				p.Judgement = "超差"
				addFinding("OUT_OF_TOLERANCE", "中", "误差与不确定度之和超过容差")
			}
		}
		result.Points = append(result.Points, p)
	}
	result.Summary = summarizePoints(result.Points)
	return result
}

func (s *Service) Findings(taskID string) []Finding {
	findings := make([]Finding, 0)
	for _, point := range s.CurrentList(taskID) {
		if strings.TrimSpace(point.Unit) == "" {
			findings = append(findings, Finding{Code: "UNIT_MISSING", Severity: "高", MeasurementID: point.ID, Message: "测量点缺少单位"})
		}
		if point.ObservedValue == nil {
			findings = append(findings, Finding{Code: "OBSERVED_MISSING", Severity: "高", MeasurementID: point.ID, Message: "测量点缺少实测读数"})
		}
		if point.Judgement == "超差" {
			findings = append(findings, Finding{Code: "OUT_OF_TOLERANCE", Severity: "中", MeasurementID: point.ID, Message: "误差与不确定度之和超过容差"})
		}
	}
	return findings
}

func (s *Service) SubmitBatch(taskID string, expected uint64, actor, key string, inputs []PointInput) (BatchResult, error) {
	if key != "" {
		events := s.store.EventsWithKey(key)
		if len(events) > 0 {
			points := make([]domain.MeasurementPoint, 0, len(events))
			for _, event := range events {
				if event.Type != "MeasurementAdded" {
					return BatchResult{}, store.ErrDuplicateKey
				}
				var point domain.MeasurementPoint
				if store.DecodePayload(event, &point) != nil {
					return BatchResult{}, store.ErrDuplicateKey
				}
				points = append(points, point)
			}
			if len(points) != len(inputs) {
				return BatchResult{}, store.ErrDuplicateKey
			}
			for i, point := range points {
				input := inputs[i]
				if point.PointLabel != strings.TrimSpace(input.PointLabel) || point.ReferenceValue != input.ReferenceValue || point.Unit != strings.TrimSpace(input.Unit) || point.Uncertainty != input.Uncertainty || point.Tolerance != input.Tolerance || point.EnteredBy != actor || valueChanged(point.ObservedValue, input.ObservedValue) {
					return BatchResult{}, store.ErrDuplicateKey
				}
			}
			return BatchResult{Points: points, Summary: s.Summary(taskID), Version: events[0].AggregateVersion}, nil
		}
	}
	task, err := s.calibration.Get(taskID)
	if err != nil {
		return BatchResult{}, err
	}
	if task.Revision != expected {
		return BatchResult{}, store.NewVersionConflict(task.Revision)
	}
	if task.Status == domain.StatusFrozen || task.Status == domain.StatusReview {
		return BatchResult{}, errors.New("当前任务状态不可录入测量")
	}
	if strings.TrimSpace(actor) == "" || len(inputs) == 0 {
		return BatchResult{}, fmt.Errorf("%w: 批量测量和录入人不能为空", ErrInvalid)
	}
	existing := s.CurrentList(taskID)
	labels := make(map[string]bool, len(existing)+len(inputs))
	for _, point := range existing {
		labels[point.PointLabel] = true
	}
	result := BatchResult{Points: make([]domain.MeasurementPoint, 0, len(inputs)), Findings: make([]Finding, 0), Version: expected + 1}
	blocking := false
	for index, input := range inputs {
		input.EnteredBy = actor
		id := fmt.Sprintf("%s-M-%d", taskID, len(s.List(taskID))+index+1)
		point := domain.MeasurementPoint{ID: id, TaskID: taskID, Revision: expected + 1, PointLabel: strings.TrimSpace(input.PointLabel), ReferenceValue: input.ReferenceValue, ObservedValue: input.ObservedValue, Unit: strings.TrimSpace(input.Unit), Uncertainty: input.Uncertainty, Tolerance: input.Tolerance, EnteredBy: actor}
		if point.PointLabel == "" {
			result.Findings = append(result.Findings, Finding{Code: "LABEL_MISSING", Severity: "阻断", MeasurementID: id, Message: "测量点标签不能为空"})
			blocking = true
		}
		if labels[point.PointLabel] && point.PointLabel != "" {
			result.Findings = append(result.Findings, Finding{Code: "POINT_DUPLICATED", Severity: "阻断", MeasurementID: id, Message: "当前修订存在重复测量点"})
			blocking = true
		}
		labels[point.PointLabel] = true
		if math.IsNaN(input.ReferenceValue) || math.IsInf(input.ReferenceValue, 0) || input.ReferenceValue < task.RangeMin || input.ReferenceValue > task.RangeMax {
			result.Findings = append(result.Findings, Finding{Code: "REFERENCE_OUT_OF_RANGE", Severity: "阻断", MeasurementID: id, Message: "标准值无效或超出校准范围"})
			blocking = true
		}
		if input.Tolerance <= 0 || input.Uncertainty < 0 || math.IsNaN(input.Uncertainty) || math.IsInf(input.Uncertainty, 0) {
			result.Findings = append(result.Findings, Finding{Code: "RULE_INVALID", Severity: "阻断", MeasurementID: id, Message: "容差和不确定度规则无效"})
			blocking = true
		}
		if point.Unit == "" {
			result.Findings = append(result.Findings, Finding{Code: "UNIT_MISSING", Severity: "高", MeasurementID: id, Message: "测量点缺少单位"})
		}
		if input.ObservedValue == nil {
			point.Judgement = "缺失"
			result.Findings = append(result.Findings, Finding{Code: "OBSERVED_MISSING", Severity: "高", MeasurementID: id, Message: "测量点缺少实测读数"})
		} else if math.IsNaN(*input.ObservedValue) || math.IsInf(*input.ObservedValue, 0) {
			result.Findings = append(result.Findings, Finding{Code: "OBSERVED_INVALID", Severity: "阻断", MeasurementID: id, Message: "实测读数不是有限数值"})
			blocking = true
		} else {
			point.Error = *input.ObservedValue - input.ReferenceValue
			if point.Unit == "" {
				point.Judgement = "缺失"
			} else if math.Abs(point.Error)+point.Uncertainty <= point.Tolerance {
				point.Judgement = "合格"
			} else {
				point.Judgement = "超差"
				result.Findings = append(result.Findings, Finding{Code: "OUT_OF_TOLERANCE", Severity: "中", MeasurementID: id, Message: "误差与不确定度之和超过容差"})
			}
		}
		result.Points = append(result.Points, point)
	}
	if blocking {
		return result, fmt.Errorf("%w: 批量数据包含阻断性发现", ErrInvalid)
	}
	proposed := make([]store.ProposedEvent, 0, len(result.Points))
	for _, point := range result.Points {
		proposed = append(proposed, store.ProposedEvent{AggregateID: taskID, Type: "MeasurementAdded", Actor: actor, Payload: point})
	}
	if _, _, err = s.store.AppendBatch(taskID, expected, key, proposed...); err != nil {
		return BatchResult{}, err
	}
	result.Summary = s.Summary(taskID)
	return result, nil
}

func (s *Service) DeterministicSummary(taskID string) DeterministicSummary {
	points := s.CurrentList(taskID)
	sort.Slice(points, func(i, j int) bool {
		if points[i].PointLabel == points[j].PointLabel {
			return points[i].ID < points[j].ID
		}
		return points[i].PointLabel < points[j].PointLabel
	})
	findings := s.Findings(taskID)
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].MeasurementID == findings[j].MeasurementID {
			return findings[i].Code < findings[j].Code
		}
		return findings[i].MeasurementID < findings[j].MeasurementID
	})
	result := DeterministicSummary{TaskID: taskID, Summary: s.Summary(taskID), Points: points, Findings: findings}
	data, _ := json.Marshal(struct {
		TaskID   string                    `json:"taskId"`
		Summary  Summary                   `json:"summary"`
		Points   []domain.MeasurementPoint `json:"points"`
		Findings []Finding                 `json:"findings"`
	}{result.TaskID, result.Summary, result.Points, result.Findings})
	sum := sha256.Sum256(data)
	result.Digest = hex.EncodeToString(sum[:])
	return result
}

func (s *Service) RevisionHistory(taskID string) []RevisionGroup {
	events := s.store.EventsFor(taskID)
	groups := make([]RevisionGroup, 0)
	current := RevisionGroup{}
	flush := func() {
		if len(current.Points) > 0 {
			current.Summary = summarizePoints(current.Points)
			groups = append(groups, current)
			current = RevisionGroup{}
		}
	}
	for _, event := range events {
		if event.Type == "TaskRevisionCreated" {
			flush()
			continue
		}
		if event.Type != "MeasurementAdded" {
			continue
		}
		var point domain.MeasurementPoint
		if store.DecodePayload(event, &point) != nil {
			continue
		}
		if current.Revision == 0 {
			current.Revision = event.AggregateVersion
		}
		current.Points = append(current.Points, point)
	}
	flush()
	return groups
}

func (s *Service) Diff(taskID string) (RevisionDiff, error) {
	task, err := s.calibration.Get(taskID)
	if err != nil {
		return RevisionDiff{}, err
	}
	groups := s.RevisionHistory(taskID)
	if len(groups) < 2 {
		return RevisionDiff{CurrentRevision: task.Revision, Items: []DiffItem{}}, nil
	}
	base, current := groups[len(groups)-2], groups[len(groups)-1]
	before := map[string]domain.MeasurementPoint{}
	after := map[string]domain.MeasurementPoint{}
	for _, p := range base.Points {
		before[strings.ToLower(strings.TrimSpace(p.PointLabel))] = p
	}
	for _, p := range current.Points {
		after[strings.ToLower(strings.TrimSpace(p.PointLabel))] = p
	}
	keys := map[string]bool{}
	for k := range before {
		keys[k] = true
	}
	for k := range after {
		keys[k] = true
	}
	result := RevisionDiff{BaseRevision: base.Revision, CurrentRevision: current.Revision, Items: make([]DiffItem, 0), BaseSummary: base.Summary, CurrentSummary: current.Summary}
	for k := range keys {
		b, bok := before[k]
		a, aok := after[k]
		item := DiffItem{Label: k}
		switch {
		case !bok:
			item.Kind = "新增"
			item.After = &a
		case !aok:
			item.Kind = "删除"
			item.Before = &b
		case b.ReferenceValue != a.ReferenceValue || valueChanged(b.ObservedValue, a.ObservedValue):
			item.Kind = "读数变更"
			item.ReadingChanged = true
			item.JudgementChanged = b.Judgement != a.Judgement
			item.Before = &b
			item.After = &a
		case b.Judgement != a.Judgement:
			item.Kind = "判定变化"
			item.Before = &b
			item.After = &a
		default:
			item.Kind = "未变化"
			item.Before = &b
			item.After = &a
		}
		if item.Kind != "未变化" {
			result.HasChange = true
		}
		result.Items = append(result.Items, item)
	}
	for _, e := range s.store.EventsFor(taskID) {
		if e.Type == "ReviewRecorded" {
			var d domain.ReviewDecision
			if store.DecodePayload(e, &d) == nil && d.Decision == "退回" {
				result.ReturnComment = d.Comment
				result.ReturnVersion = d.Revision
			}
		}
	}
	sort.Slice(result.Items, func(i, j int) bool { return result.Items[i].Label < result.Items[j].Label })
	return result, nil
}

func valueChanged(a, b *float64) bool {
	if a == nil || b == nil {
		return a != b
	}
	return *a != *b
}

func summarizePoints(points []domain.MeasurementPoint) Summary {
	summary := Summary{Count: len(points), Complete: len(points) > 0, Overall: "合格"}
	for _, point := range points {
		if point.ObservedValue == nil || strings.TrimSpace(point.Unit) == "" {
			summary.Complete = false
		}
		if point.Judgement == "合格" {
			summary.Qualified++
		} else {
			summary.Deviations++
		}
	}
	if !summary.Complete || summary.Deviations > 0 {
		summary.Overall = "需整改"
	}
	return summary
}
