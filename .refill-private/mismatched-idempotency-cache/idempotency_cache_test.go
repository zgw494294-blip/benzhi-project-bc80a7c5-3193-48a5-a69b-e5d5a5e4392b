package mismatchedidempotencycache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"scenicpermit/internal/audit"
	"scenicpermit/internal/domain"
	"scenicpermit/internal/persistence"
)

func TestOpenRejectsIdempotencyResponseForDifferentBatch(t *testing.T) {
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	aggregate, err := domain.NewAggregate("batch-cache", "缓存校验", "剧场", "协调员", now.Add(time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	event, err := audit.NewEvent("event-one", aggregate.Batch.ID, "batch.created", "协调员", 1, now, aggregate.Batch)
	if err != nil {
		t.Fatal(err)
	}
	document := map[string]any{
		"schemaVersion": 1,
		"updatedAt":     now,
		"batches":       map[string]any{aggregate.Batch.ID: aggregate},
		"events":        map[string]any{aggregate.Batch.ID: []audit.Event{event}},
		"idempotency": map[string]any{aggregate.Batch.ID: map[string]any{
			"create-key": map[string]any{"batchId": "different-batch", "revision": 99, "state": "approved"},
		}},
		"permits": []any{},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "store.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := persistence.Open(path)
	if store != nil {
		_ = store.Close()
	}
	if err == nil {
		t.Fatal("指向其他批次与修订号的幂等缓存响应被启动校验接受")
	}
}
