package timeline_rawmessage_alias_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"scenicpermit/internal/application"
	"scenicpermit/internal/audit"
	"scenicpermit/internal/domain"
	"scenicpermit/internal/persistence"
)

func TestTimelineFormattingMustNotPolluteStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	store, err := persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	aggregate, err := domain.NewAggregate("batch-pretty-audit", "审计格式化复现", "实验剧场", "制作协调员", now.Add(48*time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	event, err := audit.NewEvent("event-pretty-audit", aggregate.Batch.ID, "batch.created", "制作协调员", aggregate.Batch.Revision, now, aggregate.Batch)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), aggregate, event, "create-pretty-audit", []byte(`{"batchId":"batch-pretty-audit","revision":1,"state":"draft"}`)); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	pretty, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, pretty, 0600); err != nil {
		t.Fatal(err)
	}

	reopened, err := persistence.Open(path)
	if err != nil {
		t.Fatalf("带格式化 factSummary 的合法数据库应能打开: %v", err)
	}
	defer reopened.Close()
	service := application.NewService(reopened)
	first, err := service.BatchDetail(context.Background(), aggregate.Batch.ID)
	if err != nil {
		t.Fatalf("第一次详情查询应成功: %v", err)
	}
	if !first.TimelineVerification.Valid {
		t.Fatalf("第一次查询的时间线应有效: %s", first.TimelineVerification.Message)
	}
	second, err := service.BatchDetail(context.Background(), aggregate.Batch.ID)
	if err != nil {
		t.Fatalf("第二次详情查询不应被第一次查询污染: %v", err)
	}
	if !second.TimelineVerification.Valid {
		t.Fatalf("第二次查询的时间线应仍然有效: %s", second.TimelineVerification.Message)
	}
}
