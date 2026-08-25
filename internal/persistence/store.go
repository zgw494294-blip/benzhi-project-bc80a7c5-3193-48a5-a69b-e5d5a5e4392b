package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"scenicpermit/internal/audit"
	"scenicpermit/internal/domain"
)

type Store struct {
	mu     sync.RWMutex
	path   string
	db     database
	closed bool
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("持久化文件路径不能为空")
	}
	store := &Store{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		store.db = emptyDatabase()
		if err := store.persist(store.db); err != nil {
			return nil, err
		}
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取本地数据库: %w", err)
	}
	if err := json.Unmarshal(data, &store.db); err != nil {
		return nil, fmt.Errorf("解析本地数据库: %w", err)
	}
	if err := validateDatabase(store.db); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { s.mu.Lock(); defer s.mu.Unlock(); s.closed = true; return nil }

func (s *Store) Load(_ context.Context, id string) (*domain.Aggregate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, ok := s.db.Batches[id]
	if !ok {
		return nil, domain.NotFound("batch", id)
	}
	var aggregate domain.Aggregate
	if err := json.Unmarshal(raw, &aggregate); err != nil {
		return nil, fmt.Errorf("批次投影损坏: %w", err)
	}
	return &aggregate, nil
}

func (s *Store) List(_ context.Context) ([]domain.InspectionBatch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.InspectionBatch, 0, len(s.db.Batches))
	for _, raw := range s.db.Batches {
		var aggregate domain.Aggregate
		if err := json.Unmarshal(raw, &aggregate); err != nil {
			return nil, err
		}
		result = append(result, aggregate.Batch)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) Events(_ context.Context, batchID string) ([]audit.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := append([]audit.Event(nil), s.db.Events[batchID]...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Revision == result[j].Revision {
			return result[i].OccurredAt.Before(result[j].OccurredAt)
		}
		return result[i].Revision < result[j].Revision
	})
	return result, nil
}

func (s *Store) Permits(_ context.Context) ([]domain.AdmissionPermit, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := append([]domain.AdmissionPermit(nil), s.db.Permits...)
	sort.Slice(result, func(i, j int) bool { return result[i].Sequence < result[j].Sequence })
	return result, nil
}

func (s *Store) IdempotentResponse(_ context.Context, batchID, key string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	responses := s.db.Idempotency[batchID]
	if responses == nil {
		return nil, false, nil
	}
	raw, ok := responses[key]
	return append([]byte(nil), raw...), ok, nil
}

func (s *Store) persist(next database) error {
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0750); err != nil {
		return fmt.Errorf("创建数据目录: %w", err)
	}
	next.UpdatedAt = time.Now().UTC()
	encoded, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("编码本地数据库: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".scenicpermit-*.tmp")
	if err != nil {
		return fmt.Errorf("创建事务临时文件: %w", err)
	}
	temporaryName := temporary.Name()
	cleanup := func() { _ = temporary.Close(); _ = os.Remove(temporaryName) }
	if _, err := temporary.Write(encoded); err != nil {
		cleanup()
		return fmt.Errorf("写入事务临时文件: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("同步事务临时文件: %w", err)
	}
	if err := temporary.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(temporaryName, s.path); err != nil {
		cleanup()
		return fmt.Errorf("原子替换数据库: %w", err)
	}
	return nil
}
