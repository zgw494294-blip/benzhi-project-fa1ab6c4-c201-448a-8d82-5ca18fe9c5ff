package store

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"
)

const SchemaVersion = 1

var (
	ErrCorruptLog       = errors.New("事件日志校验失败")
	ErrDuplicateKey     = errors.New("幂等键已使用且请求内容不同")
	ErrVersionConflict  = errors.New("聚合版本冲突")
	ErrStoreUnavailable = errors.New("持久化仓储不可用")
)

type VersionConflict struct{ Version uint64 }

type ProposedEvent struct {
	AggregateID string `json:"aggregateId"`
	Type        string `json:"type"`
	Actor       string `json:"actor"`
	Payload     any    `json:"payload"`
}

type Event struct {
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
	Hash             string          `json:"hash"`
}

type Snapshot struct {
	SchemaVersion int       `json:"schemaVersion"`
	UpdatedAt     time.Time `json:"updatedAt"`
	LastSequence  uint64    `json:"lastSequence"`
	LastHash      string    `json:"lastHash"`
	Events        []Event   `json:"events"`
}

type IntegrityReport struct {
	SchemaVersion int       `json:"schemaVersion"`
	EventCount    int       `json:"eventCount"`
	LastSequence  uint64    `json:"lastSequence"`
	LastHash      string    `json:"lastHash"`
	LogValid      bool      `json:"logValid"`
	SnapshotValid bool      `json:"snapshotValid"`
	CheckedAt     time.Time `json:"checkedAt"`
}

type Store struct {
	mu           sync.RWMutex
	dir          string
	logPath      string
	logFile      *os.File
	snapshotPath string
	events       []Event
	keyDigests   map[string]string
}
