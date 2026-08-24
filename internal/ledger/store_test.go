package ledger

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"surveyrelease/internal/application"
	"surveyrelease/internal/domain"
)

func createThroughService(t *testing.T, store *Store, key string) (*domain.ReleaseCase, []byte) {
	t.Helper()
	svc := application.NewService(store, []byte("0123456789abcdef"), func() time.Time { return time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC) })
	input := application.CreateCaseInput{Title: "测试档案", AcquisitionBatch: "B-1", CoordinateReference: "CGCS2000", ScopeDescription: "范围", Artifacts: []domain.Artifact{{Path: "a.tif", SHA256: strings.Repeat("a", 64), SizeBytes: 10}}}
	raw, _, err := svc.CreateCase(context.Background(), application.CommandMeta{Actor: application.Actor{Name: "engineer", Role: application.RoleEngineer}, IdempotencyKey: key}, input)
	if err != nil {
		t.Fatal(err)
	}
	response, err := application.DecodeResponse[application.CaseResponse](raw)
	if err != nil {
		t.Fatal(err)
	}
	return response.Case, raw
}

func TestStoreReopenReplayAndIdempotency(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(Config{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	created, first := createThroughService(t, store, "create-1")
	replayed, second := createThroughService(t, store, "create-1")
	if created.ID != replayed.ID || string(first) != string(second) {
		t.Fatal("幂等重放未返回首次响应")
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(Config{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.Get(context.Background(), created.ID)
	if err != nil || got == nil {
		t.Fatalf("恢复档案失败: %v", err)
	}
	page, err := reopened.Timeline(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || page.Events[0].PreviousHash != "" || page.Events[0].EventHash == "" || !page.Events[0].HashVerified {
		t.Fatalf("审计事件异常: %+v", page.Events)
	}
}

func TestStoreRejectsTruncatedLedger(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(Config{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	createThroughService(t, store, "create-1")
	_ = store.Close()
	path := filepath.Join(dir, "events.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data[:len(data)-1], 0o640); err != nil {
		t.Fatal(err)
	}
	_, err = Open(Config{Directory: dir})
	var corrupt *CorruptionError
	if !errors.As(err, &corrupt) {
		t.Fatalf("期望 CorruptionError，实际 %v", err)
	}
}

func TestStoreRejectsHashModification(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(Config{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	createThroughService(t, store, "create-1")
	_ = store.Close()
	path := filepath.Join(dir, "events.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "测试档案", "篡改档案", 1))
	if err = os.WriteFile(path, data, 0o640); err != nil {
		t.Fatal(err)
	}
	_, err = Open(Config{Directory: dir})
	var corrupt *CorruptionError
	if !errors.As(err, &corrupt) {
		t.Fatalf("期望哈希损坏错误，实际 %v", err)
	}
}
