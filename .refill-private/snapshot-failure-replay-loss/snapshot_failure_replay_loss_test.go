package snapshotfailurereplayloss_test

import (
	"os"
	"path/filepath"
	"testing"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
)

func TestSnapshotFailureRetrySurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	repository, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	snapshotPath := filepath.Join(dir, "snapshot.json")
	if err := os.Remove(snapshotPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(snapshotPath, 0o750); err != nil {
		t.Fatal(err)
	}

	proposal := store.ProposedEvent{
		AggregateID: "task-snapshot-retry",
		Type:        "TaskCreated",
		Actor:       "技术员",
		Payload:     map[string]string{"status": "待测"},
	}
	if _, _, err := repository.AppendBatch("task-snapshot-retry", 0, "snapshot-retry-key", proposal); err == nil {
		t.Fatal("快照替换失败时首次提交应返回错误")
	}

	if err := os.Remove(snapshotPath); err != nil {
		t.Fatal(err)
	}
	replayed, replay, err := repository.AppendBatch("task-snapshot-retry", 0, "snapshot-retry-key", proposal)
	if err != nil {
		t.Fatalf("同幂等键重试应恢复已追加事件: %v", err)
	}
	if !replay || len(replayed) != 1 {
		t.Fatalf("同幂等键重试应返回首次提交结果: replay=%t events=%d", replay, len(replayed))
	}

	reopened, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if events := reopened.EventsFor("task-snapshot-retry"); len(events) != 1 {
		t.Fatalf("重试已确认成功的事件在重启后丢失: events=%d", len(events))
	}
}
