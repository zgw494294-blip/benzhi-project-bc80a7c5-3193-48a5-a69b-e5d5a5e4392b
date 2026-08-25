package application

import (
	"context"
	"encoding/hex"
	"strconv"
	"strings"

	"scenicpermit/internal/audit"
	"scenicpermit/internal/domain"
)

type BatchDetail struct {
	Aggregate            *domain.Aggregate             `json:"aggregate"`
	Matrix               []domain.MatrixCell           `json:"matrix"`
	Blocking             []domain.MatrixCell           `json:"blocking"`
	Timeline             []audit.Event                 `json:"timeline"`
	PermitVerification   *audit.PermitVerification     `json:"permitVerification,omitempty"`
	Coverage             domain.CoverageSummary        `json:"coverage"`
	Histories            []domain.CheckHistory         `json:"histories"`
	TimelineVerification audit.TimelineResult          `json:"timelineVerification"`
	Progress             domain.ProgressView           `json:"progress"`
	RemediationQueue     []domain.RemediationQueueItem `json:"remediationQueue"`
}

type BatchDetailFilter struct {
	Progress          domain.ProgressFilter
	RemediationOwner  string
	RemediationStatus string
	DueRisk           string
}

type TargetedPermitView struct {
	Verification audit.TargetedPermitVerification `json:"verification"`
	Timeline     []audit.Event                    `json:"timeline,omitempty"`
}

func (s *Service) ListBatches(ctx context.Context) ([]domain.InspectionBatch, error) {
	return s.repo.List(ctx)
}
func (s *Service) BatchDetail(ctx context.Context, id string) (BatchDetail, error) {
	return s.BatchDetailFiltered(ctx, id, BatchDetailFilter{})
}
func (s *Service) BatchDetailFiltered(ctx context.Context, id string, filter BatchDetailFilter) (BatchDetail, error) {
	aggregate, err := s.repo.Load(ctx, id)
	if err != nil {
		return BatchDetail{}, err
	}
	events, err := s.repo.Events(ctx, id)
	if err != nil {
		return BatchDetail{}, err
	}
	progress, err := aggregate.Progress(filter.Progress)
	if err != nil {
		return BatchDetail{}, err
	}
	queue, err := aggregate.RemediationQueue(filter.RemediationOwner, filter.RemediationStatus, filter.DueRisk, s.clock.Now())
	if err != nil {
		return BatchDetail{}, err
	}
	detail := BatchDetail{
		Aggregate: aggregate, Matrix: progress.Matrix, Blocking: aggregate.BlockingCells(), Timeline: events,
		Coverage: aggregate.Coverage(), Histories: aggregate.Histories(),
		TimelineVerification: audit.VerifyTimeline(id, events, aggregate.Batch.Revision),
		Progress:             progress, RemediationQueue: queue,
	}
	if aggregate.Permit != nil {
		permits, err := s.repo.Permits(ctx)
		if err != nil {
			return BatchDetail{}, err
		}
		verification := audit.VerifyAggregatePermit(aggregate, permits)
		detail.PermitVerification = &verification
	}
	return detail, nil
}

func (s *Service) PreflightPlan(ctx context.Context, batchID string, definitions []domain.CheckDefinition) (domain.PlanPreflight, error) {
	if err := domain.ValidateID(batchID, "batchId"); err != nil {
		return domain.PlanPreflight{}, err
	}
	aggregate, err := s.repo.Load(ctx, batchID)
	if err != nil {
		return domain.PlanPreflight{}, err
	}
	view := aggregate.PreflightPlan(definitions)
	if view.Confirmable {
		view.ConfirmationDigest, err = audit.PlanConfirmationDigest(view)
		if err != nil {
			return domain.PlanPreflight{}, err
		}
	}
	return view, nil
}

func validatePermitDigest(value string) error {
	if len(value) != 64 || value != strings.ToLower(value) {
		return domain.Validation("invalid_permit_digest", "permitDigest 必须是 64 位小写十六进制摘要")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return domain.Validation("invalid_permit_digest", "permitDigest 必须是 64 位小写十六进制摘要")
	}
	return nil
}

func (s *Service) VerifyPermit(ctx context.Context, sequence int64, digest string) (TargetedPermitView, error) {
	digest = strings.TrimSpace(digest)
	if sequence < 0 {
		return TargetedPermitView{}, domain.Validation("invalid_permit_sequence", "sequence 必须大于零")
	}
	if sequence == 0 && digest == "" {
		return TargetedPermitView{}, domain.Validation("missing_permit_locator", "必须提供 sequence 或 permitDigest")
	}
	if digest != "" {
		if err := validatePermitDigest(digest); err != nil {
			return TargetedPermitView{}, err
		}
	}
	permits, err := s.repo.Permits(ctx)
	if err != nil {
		return TargetedPermitView{}, err
	}
	var selected *domain.AdmissionPermit
	if sequence > 0 {
		for i := range permits {
			if permits[i].Sequence == sequence {
				selected = &permits[i]
				break
			}
		}
		if selected == nil {
			return TargetedPermitView{}, domain.NotFound("permit_sequence", strconv.FormatInt(sequence, 10))
		}
		if digest != "" && selected.PermitDigest != digest {
			return TargetedPermitView{Verification: audit.TargetedPermitVerification{Matched: false, Message: "凭据序号与 permitDigest 不匹配"}}, nil
		}
	} else {
		for i := range permits {
			if permits[i].PermitDigest == digest {
				selected = &permits[i]
				break
			}
		}
		if selected == nil {
			return TargetedPermitView{}, domain.NotFound("permit_digest", digest)
		}
	}
	aggregate, err := s.repo.Load(ctx, selected.BatchID)
	if err != nil {
		return TargetedPermitView{}, err
	}
	verification := audit.VerifyTargetedPermit(aggregate, *selected, permits)
	events, err := s.repo.Events(ctx, selected.BatchID)
	if err != nil {
		return TargetedPermitView{}, err
	}
	return TargetedPermitView{Verification: verification, Timeline: events}, nil
}
func (s *Service) VerifyPermits(ctx context.Context) (audit.ChainResult, error) {
	permits, err := s.repo.Permits(ctx)
	if err != nil {
		return audit.ChainResult{}, err
	}
	return audit.VerifyChain(permits), nil
}
