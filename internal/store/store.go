package store

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func (e *VersionConflict) Error() string {
	return fmt.Sprintf("%s: 当前版本为 %d", ErrVersionConflict, e.Version)
}
func (e *VersionConflict) Unwrap() error          { return ErrVersionConflict }
func (e *VersionConflict) CurrentVersion() uint64 { return e.Version }
func NewVersionConflict(version uint64) error     { return &VersionConflict{Version: version} }

func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("%w: 数据目录不能为空", ErrStoreUnavailable)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	s := &Store{
		dir:          dir,
		logPath:      filepath.Join(dir, "events.jsonl"),
		snapshotPath: filepath.Join(dir, "snapshot.json"),
		keyDigests:   make(map[string]string),
	}
	if err := s.recover(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Directory() string { return s.dir }

func (s *Store) recover() error {
	events, err := readAndVerifyLog(s.logPath)
	if err != nil {
		return err
	}
	s.events = events
	for _, event := range events {
		if event.IdempotencyKey != "" {
			s.keyDigests[event.IdempotencyKey] = event.RequestDigest
		}
	}
	validSnapshot := false
	if data, readErr := os.ReadFile(s.snapshotPath); readErr == nil {
		var snap Snapshot
		if json.Unmarshal(data, &snap) == nil && snap.SchemaVersion == SchemaVersion {
			lastHash := ""
			if len(events) > 0 {
				lastHash = events[len(events)-1].Hash
			}
			validSnapshot = snap.LastSequence == uint64(len(events)) && snap.LastHash == lastHash && sameEvents(snap.Events, events)
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("%w: 读取快照: %v", ErrStoreUnavailable, readErr)
	}
	if !validSnapshot {
		return s.writeSnapshotLocked()
	}
	return nil
}

func sameEvents(left, right []Event) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Sequence != right[index].Sequence || left[index].Hash != right[index].Hash {
			return false
		}
	}
	return true
}

func (s *Store) Inspect() (IntegrityReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	report := IntegrityReport{SchemaVersion: SchemaVersion, CheckedAt: time.Now().UTC()}
	events, err := readAndVerifyLog(s.logPath)
	if err != nil {
		return report, err
	}
	report.EventCount = len(events)
	report.LogValid = true
	if len(events) > 0 {
		report.LastSequence = events[len(events)-1].Sequence
		report.LastHash = events[len(events)-1].Hash
	}
	data, err := os.ReadFile(s.snapshotPath)
	if err != nil {
		return report, fmt.Errorf("读取快照: %w", err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return report, fmt.Errorf("快照 JSON 损坏: %w", err)
	}
	report.SnapshotValid = snapshot.SchemaVersion == SchemaVersion && snapshot.LastSequence == report.LastSequence && snapshot.LastHash == report.LastHash && sameEvents(snapshot.Events, events)
	if !report.SnapshotValid {
		return report, errors.New("快照与事件日志不一致")
	}
	return report, nil
}

func readAndVerifyLog(path string) ([]Event, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	events := make([]Event, 0)
	previous := ""
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("%w: 第 %d 行不是有效 JSON", ErrCorruptLog, len(events)+1)
		}
		if event.Sequence != uint64(len(events)+1) || event.PreviousHash != previous {
			return nil, fmt.Errorf("%w: 第 %d 个事件的序号或前序哈希不正确", ErrCorruptLog, len(events)+1)
		}
		if calculateHash(event) != event.Hash {
			return nil, fmt.Errorf("%w: 第 %d 个事件内容被修改", ErrCorruptLog, len(events)+1)
		}
		previous = event.Hash
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	return events, nil
}

func (s *Store) Events() []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneEvents(s.events)
}

func (s *Store) EventsFor(aggregateID string) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Event, 0)
	for _, event := range s.events {
		if event.AggregateID == aggregateID {
			result = append(result, event)
		}
	}
	return cloneEvents(result)
}

func (s *Store) EventsWithKey(key string) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Event, 0)
	for _, event := range s.events {
		if event.IdempotencyKey == key {
			result = append(result, event)
		}
	}
	return cloneEvents(result)
}

// cloneEvents returns a slice whose Event values are independent copies of the
// input, including the Payload bytes. Callers may freely modify the returned
// Payload (or its underlying bytes) without affecting the Store's internal
// projection.
func cloneEvents(events []Event) []Event {
	result := make([]Event, len(events))
	copy(result, events)
	for index := range result {
		if len(result[index].Payload) > 0 {
			payload := make([]byte, len(result[index].Payload))
			copy(payload, result[index].Payload)
			result[index].Payload = payload
		}
	}
	return result
}

// AppendBatch writes all proposed events in one locked, fsynced append.
func (s *Store) AppendBatch(aggregateID string, expectedVersion uint64, idempotencyKey string, proposed ...ProposedEvent) ([]Event, bool, error) {
	if len(proposed) == 0 {
		return nil, false, errors.New("提交事件不能为空")
	}
	digest, err := proposalDigest(proposed)
	if err != nil {
		return nil, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if idempotencyKey != "" {
		if existing, ok := s.keyDigests[idempotencyKey]; ok {
			if existing != digest {
				return nil, false, ErrDuplicateKey
			}
			return s.eventsWithKeyLocked(idempotencyKey), true, nil
		}
	}
	currentVersion := uint64(0)
	for _, event := range s.events {
		if event.AggregateID == aggregateID && event.AggregateVersion > currentVersion {
			currentVersion = event.AggregateVersion
		}
	}
	if currentVersion != expectedVersion {
		return nil, false, fmt.Errorf("%w: 当前版本为 %d，期望版本为 %d", ErrVersionConflict, currentVersion, expectedVersion)
	}
	for _, item := range proposed {
		if item.AggregateID != aggregateID {
			return nil, false, errors.New("原子提交只能包含同一聚合的事件")
		}
	}
	verified, err := readAndVerifyLog(s.logPath)
	if err != nil {
		return nil, false, err
	}
	if len(verified) != len(s.events) {
		return nil, false, fmt.Errorf("%w: 内存投影与日志长度不一致", ErrCorruptLog)
	}
	file, err := os.OpenFile(s.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	previous := ""
	if len(s.events) > 0 {
		previous = s.events[len(s.events)-1].Hash
	}
	created := make([]Event, 0, len(proposed))
	now := time.Now().UTC()
	for index, item := range proposed {
		payload, marshalErr := json.Marshal(item.Payload)
		if marshalErr != nil {
			file.Close()
			return nil, false, fmt.Errorf("事件载荷编码失败: %w", marshalErr)
		}
		event := Event{
			Sequence:         uint64(len(s.events) + index + 1),
			AggregateVersion: expectedVersion + 1,
			AggregateID:      item.AggregateID,
			Type:             item.Type,
			Actor:            item.Actor,
			IdempotencyKey:   idempotencyKey,
			RequestDigest:    digest,
			OccurredAt:       now,
			Payload:          payload,
			PreviousHash:     previous,
		}
		event.Hash = calculateHash(event)
		line, _ := json.Marshal(event)
		if _, err = file.Write(append(line, '\n')); err != nil {
			file.Close()
			return nil, false, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
		}
		previous = event.Hash
		created = append(created, event)
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return nil, false, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	if err = file.Close(); err != nil {
		return nil, false, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	s.events = append(s.events, created...)
	if idempotencyKey != "" {
		s.keyDigests[idempotencyKey] = digest
	}
	if err := s.writeSnapshotLocked(); err != nil {
		return nil, false, err
	}
	return cloneEvents(created), false, nil
}

func (s *Store) eventsWithKeyLocked(key string) []Event {
	result := make([]Event, 0)
	for _, event := range s.events {
		if event.IdempotencyKey == key {
			result = append(result, event)
		}
	}
	return cloneEvents(result)
}

func (s *Store) writeSnapshotLocked() error {
	lastHash := ""
	if len(s.events) > 0 {
		lastHash = s.events[len(s.events)-1].Hash
	}
	snapshot := Snapshot{
		SchemaVersion: SchemaVersion,
		UpdatedAt:     time.Now().UTC(),
		LastSequence:  uint64(len(s.events)),
		LastHash:      lastHash,
		Events:        append([]Event(nil), s.events...),
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("快照编码失败: %w", err)
	}
	temp, err := os.CreateTemp(s.dir, ".snapshot-*.tmp")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o640); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, s.snapshotPath); err != nil {
		return fmt.Errorf("原子替换快照失败: %w", err)
	}
	dir, err := os.Open(s.dir)
	if err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func calculateHash(event Event) string {
	canonical := struct {
		Sequence         uint64          `json:"sequence"`
		AggregateVersion uint64          `json:"aggregateVersion"`
		AggregateID      string          `json:"aggregateId"`
		Type             string          `json:"type"`
		Actor            string          `json:"actor"`
		IdempotencyKey   string          `json:"idempotencyKey,omitempty"`
		RequestDigest    string          `json:"requestDigest,omitempty"`
		OccurredAt       time.Time       `json:"occurredAt"`
		Payload          json.RawMessage `json:"payload"`
		PreviousHash     string          `json:"previousHash"`
	}{event.Sequence, event.AggregateVersion, event.AggregateID, event.Type, event.Actor, event.IdempotencyKey, event.RequestDigest, event.OccurredAt, event.Payload, event.PreviousHash}
	data, _ := json.Marshal(canonical)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func proposalDigest(proposed []ProposedEvent) (string, error) {
	data, err := json.Marshal(proposed)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func DecodePayload(event Event, target any) error {
	if err := json.Unmarshal(event.Payload, target); err != nil {
		return fmt.Errorf("事件 %d (%s) 载荷损坏: %w", event.Sequence, event.Type, err)
	}
	return nil
}

func CopyFile(dst string, src io.Reader) error {
	file, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, src)
	return err
}
