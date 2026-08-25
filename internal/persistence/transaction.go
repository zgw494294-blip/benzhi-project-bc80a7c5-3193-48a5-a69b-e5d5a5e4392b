package persistence

import (
	"context"
	"encoding/json"
	"fmt"

	"scenicpermit/internal/application"
	"scenicpermit/internal/audit"
	"scenicpermit/internal/domain"
)

func encodeAggregate(aggregate *domain.Aggregate) (json.RawMessage, error) {
	data, err := json.Marshal(aggregate)
	if err != nil {
		return nil, fmt.Errorf("编码批次投影: %w", err)
	}
	return data, nil
}

func (s *Store) Create(ctx context.Context, aggregate *domain.Aggregate, event audit.Event, key string, response []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("本地数据库已经关闭")
	}
	if _, exists := s.db.Batches[aggregate.Batch.ID]; exists {
		return domain.Conflict("duplicate_batch", "批次 ID 已存在")
	}
	next, err := cloneDatabase(s.db)
	if err != nil {
		return err
	}
	raw, err := encodeAggregate(aggregate)
	if err != nil {
		return err
	}
	next.Batches[aggregate.Batch.ID] = raw
	next.Events[aggregate.Batch.ID] = []audit.Event{event}
	next.Idempotency[aggregate.Batch.ID] = map[string]json.RawMessage{key: append([]byte(nil), response...)}
	if err := validateDatabase(next); err != nil {
		return err
	}
	if err := s.persist(next); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("创建批次事务已取消: %w", err)
	}
	s.db = next
	return nil
}

func (s *Store) Commit(ctx context.Context, commit application.Commit) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("本地数据库已经关闭")
	}
	batchID := commit.Aggregate.Batch.ID
	currentRaw, exists := s.db.Batches[batchID]
	if !exists {
		return domain.NotFound("batch", batchID)
	}
	var current domain.Aggregate
	if err := json.Unmarshal(currentRaw, &current); err != nil {
		return err
	}
	if current.Batch.Revision != commit.ExpectedRev {
		return domain.Conflict("concurrent_revision", "批次已被其他请求修改")
	}
	if current.Permit != nil {
		return domain.Immutable("已批准批次禁止更新或删除")
	}
	if _, replay := s.db.Idempotency[batchID][commit.IdempotencyKey]; replay {
		return domain.Conflict("duplicate_idempotency_key", "幂等键已经提交")
	}
	next, err := cloneDatabase(s.db)
	if err != nil {
		return err
	}
	raw, err := encodeAggregate(commit.Aggregate)
	if err != nil {
		return err
	}
	next.Batches[batchID] = raw
	next.Events[batchID] = append(next.Events[batchID], commit.Event)
	if next.Idempotency[batchID] == nil {
		next.Idempotency[batchID] = make(map[string]json.RawMessage)
	}
	next.Idempotency[batchID][commit.IdempotencyKey] = append([]byte(nil), commit.Response...)
	if commit.NewPermit != nil {
		if commit.NewPermit.Sequence != int64(len(next.Permits)+1) {
			return domain.Conflict("permit_sequence_conflict", "凭据序号分配冲突")
		}
		for _, permit := range next.Permits {
			if permit.BatchID == batchID {
				return domain.Conflict("duplicate_permit", "一个批次只能签发一份凭据")
			}
		}
		next.Permits = append(next.Permits, *commit.NewPermit)
	}
	if err := validateDatabase(next); err != nil {
		return err
	}
	if err := s.persist(next); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("更新批次事务已取消: %w", err)
	}
	s.db = next
	return nil
}
