package application

import (
	"context"
	"time"

	"scenicpermit/internal/audit"
	"scenicpermit/internal/domain"
)

type Commit struct {
	Aggregate      *domain.Aggregate
	ExpectedRev    int64
	Event          audit.Event
	IdempotencyKey string
	Response       []byte
	NewPermit      *domain.AdmissionPermit
}

type Repository interface {
	Create(context.Context, *domain.Aggregate, audit.Event, string, []byte) error
	Load(context.Context, string) (*domain.Aggregate, error)
	List(context.Context) ([]domain.InspectionBatch, error)
	Commit(context.Context, Commit) error
	Events(context.Context, string) ([]audit.Event, error)
	Permits(context.Context) ([]domain.AdmissionPermit, error)
	IdempotentResponse(context.Context, string, string) ([]byte, bool, error)
	Close() error
}

type Clock interface{ Now() time.Time }
