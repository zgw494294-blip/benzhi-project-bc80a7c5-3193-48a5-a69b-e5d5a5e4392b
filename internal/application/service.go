package application

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"scenicpermit/internal/audit"
	"scenicpermit/internal/domain"
)

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	repo         Repository
	clock        Clock
	serial       batchSerial
	ids          atomic.Uint64
	permitMu     sync.Mutex
	permitChain  []domain.AdmissionPermit
	permitLoaded bool
}

func NewService(repo Repository) *Service { return &Service{repo: repo, clock: systemClock{}} }
func NewServiceWithClock(repo Repository, clock Clock) *Service {
	return &Service{repo: repo, clock: clock}
}
func (s *Service) nextID(prefix string) string {
	return fmt.Sprintf("%s-%d-%d", prefix, s.clock.Now().UnixNano(), s.ids.Add(1))
}

func validateMeta(meta Meta) error {
	if err := domain.ValidateID(meta.BatchID, "batchId"); err != nil {
		return err
	}
	if strings.TrimSpace(meta.Actor) == "" {
		return domain.Validation("missing_actor", "actor 为必填项")
	}
	if strings.TrimSpace(meta.IdempotencyKey) == "" {
		return domain.Validation("missing_idempotency_key", "idempotencyKey 为必填项")
	}
	if len(meta.IdempotencyKey) > 128 {
		return domain.Validation("long_idempotency_key", "idempotencyKey 长度超出限制")
	}
	return nil
}

func (s *Service) replay(ctx context.Context, batchID, key string) (*CommandResult, bool, error) {
	data, ok, err := s.repo.IdempotentResponse(ctx, batchID, key)
	if err != nil || !ok {
		return nil, ok, err
	}
	var result CommandResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, false, err
	}
	result.Replay = true
	return &result, true, nil
}

func (s *Service) permitSnapshot(ctx context.Context) ([]domain.AdmissionPermit, error) {
	s.permitMu.Lock()
	defer s.permitMu.Unlock()
	if s.permitLoaded {
		return append([]domain.AdmissionPermit(nil), s.permitChain...), nil
	}
	permits, err := s.repo.Permits(ctx)
	if err != nil {
		return nil, err
	}
	s.permitChain = append([]domain.AdmissionPermit(nil), permits...)
	s.permitLoaded = true
	return append([]domain.AdmissionPermit(nil), s.permitChain...), nil
}

func (s *Service) CreateBatch(ctx context.Context, command CreateBatchCommand) (CommandResult, error) {
	if strings.TrimSpace(command.Actor) == "" || strings.TrimSpace(command.IdempotencyKey) == "" {
		return CommandResult{}, domain.Validation("missing_command_metadata", "actor 和 idempotencyKey 为必填项")
	}
	if replay, ok, err := s.replay(ctx, command.ID, command.IdempotencyKey); err != nil {
		return CommandResult{}, err
	} else if ok {
		return *replay, nil
	}
	now := s.clock.Now()
	aggregate, err := domain.NewAggregate(command.ID, command.Title, command.Venue, command.Coordinator, command.PerformanceAt, now)
	if err != nil {
		return CommandResult{}, err
	}
	result := CommandResult{BatchID: command.ID, Revision: aggregate.Batch.Revision, State: aggregate.Batch.State}
	encoded, _ := json.Marshal(result)
	event, err := audit.NewEvent(s.nextID("evt"), command.ID, "batch.created", command.Actor, aggregate.Batch.Revision, now, aggregate.Batch)
	if err != nil {
		return CommandResult{}, err
	}
	if err := s.repo.Create(ctx, aggregate, event, command.IdempotencyKey, encoded); err != nil {
		return CommandResult{}, err
	}
	return result, nil
}

type mutation func(*domain.Aggregate, time.Time) (string, any, error)

func (s *Service) mutate(ctx context.Context, meta Meta, action string, fn mutation) (CommandResult, error) {
	if err := validateMeta(meta); err != nil {
		return CommandResult{}, err
	}
	var output CommandResult
	err := s.serial.execute(meta.BatchID, func() error {
		if replay, ok, err := s.replay(ctx, meta.BatchID, meta.IdempotencyKey); err != nil {
			return err
		} else if ok {
			output = *replay
			return nil
		}
		aggregate, err := s.repo.Load(ctx, meta.BatchID)
		if err != nil {
			return err
		}
		if err := aggregate.RequireRevision(meta.Revision); err != nil {
			return err
		}
		expected := aggregate.Batch.Revision
		resourceID, facts, err := fn(aggregate, s.clock.Now())
		if err != nil {
			return err
		}
		output = CommandResult{BatchID: meta.BatchID, Revision: aggregate.Batch.Revision, State: aggregate.Batch.State, ResourceID: resourceID}
		encoded, _ := json.Marshal(output)
		event, err := audit.NewEvent(s.nextID("evt"), meta.BatchID, action, meta.Actor, aggregate.Batch.Revision, s.clock.Now(), facts)
		if err != nil {
			return err
		}
		commit := Commit{Aggregate: aggregate, ExpectedRev: expected, Event: event, IdempotencyKey: meta.IdempotencyKey, Response: encoded}
		if aggregate.Permit != nil && action == "batch.approved" {
			commit.NewPermit = aggregate.Permit
		}
		return s.repo.Commit(ctx, commit)
	})
	return output, err
}

func (s *Service) AddUnit(ctx context.Context, command AddUnitCommand) (CommandResult, error) {
	return s.mutate(ctx, command.Meta, "unit.registered", func(a *domain.Aggregate, now time.Time) (string, any, error) {
		if command.Unit.ID == "" {
			command.Unit.ID = s.nextID("unit")
		}
		err := a.AddUnit(command.Unit, now)
		return command.Unit.ID, command.Unit, err
	})
}

func (s *Service) UpdateUnit(ctx context.Context, command UpdateUnitCommand) (CommandResult, error) {
	return s.mutate(ctx, command.Meta, "unit.updated", func(a *domain.Aggregate, now time.Time) (string, any, error) {
		before, err := a.UpdateUnit(command.UnitID, command.Unit)
		if err != nil {
			return command.UnitID, nil, err
		}
		after, _ := a.Unit(command.UnitID)
		return command.UnitID, map[string]any{"unitId": command.UnitID, "before": before, "after": after}, nil
	})
}

func (s *Service) RemoveUnit(ctx context.Context, command RemoveUnitCommand) (CommandResult, error) {
	return s.mutate(ctx, command.Meta, "unit.withdrawn", func(a *domain.Aggregate, now time.Time) (string, any, error) {
		removed, err := a.RemoveUnit(command.UnitID)
		return command.UnitID, map[string]any{"unitId": command.UnitID, "withdrawn": removed}, err
	})
}

func (s *Service) UpdateBatch(ctx context.Context, command UpdateBatchCommand) (CommandResult, error) {
	return s.mutate(ctx, command.Meta, "batch.updated", func(a *domain.Aggregate, now time.Time) (string, any, error) {
		err := a.UpdateBatch(command.Title, command.Venue, command.Coordinator, command.PerformanceAt)
		facts := map[string]any{"title": command.Title, "venue": command.Venue, "performanceAt": command.PerformanceAt, "coordinator": command.Coordinator}
		return a.Batch.ID, facts, err
	})
}

func (s *Service) SubmitPlan(ctx context.Context, command SubmitPlanCommand) (CommandResult, error) {
	return s.mutate(ctx, command.Meta, "plan.submitted", func(a *domain.Aggregate, now time.Time) (string, any, error) {
		if strings.TrimSpace(command.ConfirmationDigest) == "" {
			return "", nil, domain.Validation("missing_confirmation_digest", "正式送检必须携带预检确认摘要")
		}
		preview := a.PreflightPlan(command.Definitions)
		if !preview.Confirmable {
			return "", nil, domain.Conflict("plan_preflight_blocked", "当前方案存在阻断诊断，不能送检")
		}
		digest, err := audit.PlanConfirmationDigest(preview)
		if err != nil {
			return "", nil, err
		}
		if digest != command.ConfirmationDigest {
			return "", nil, domain.Conflict("stale_plan_confirmation", "预检确认已过期，请重新预检后送检")
		}
		if command.PlanID == "" {
			command.PlanID = s.nextID("plan")
		}
		err = a.FreezePlan(command.PlanID, command.Actor, command.Definitions, now)
		return command.PlanID, map[string]any{"planId": command.PlanID, "definitions": command.Definitions, "confirmationDigest": digest, "coverage": preview.Summary}, err
	})
}

func (s *Service) RecordResult(ctx context.Context, command RecordResultCommand) (CommandResult, error) {
	return s.mutate(ctx, command.Meta, "check.recorded", func(a *domain.Aggregate, now time.Time) (string, any, error) {
		if command.Result.ID == "" {
			command.Result.ID = s.nextID("result")
		}
		err := a.RecordResult(command.Result, now)
		return command.Result.ID, command.Result, err
	})
}

func (s *Service) RecordResults(ctx context.Context, command RecordResultsCommand) (CommandResult, error) {
	if err := validateMeta(command.Meta); err != nil {
		return CommandResult{}, err
	}
	var output CommandResult
	err := s.serial.execute(command.BatchID, func() error {
		if replay, ok, err := s.replay(ctx, command.BatchID, command.IdempotencyKey); err != nil {
			return err
		} else if ok {
			output = *replay
			return nil
		}
		a, err := s.repo.Load(ctx, command.BatchID)
		if err != nil {
			return err
		}
		if err := a.RequireRevision(command.Revision); err != nil {
			return err
		}
		expected := a.Batch.Revision
		items := append([]domain.CheckResult(nil), command.Results...)
		sort.SliceStable(items, func(i, j int) bool {
			left, right := items[i].UnitID, items[j].UnitID
			if unit, _ := a.Unit(items[i].UnitID); unit != nil {
				left = strings.ToLower(unit.UnitCode)
			}
			if unit, _ := a.Unit(items[j].UnitID); unit != nil {
				right = strings.ToLower(unit.UnitCode)
			}
			if left == right {
				return items[i].CheckCode < items[j].CheckCode
			}
			return left < right
		})
		for i := range items {
			if items[i].ID == "" {
				items[i].ID = s.nextID("result")
			}
		}
		applied, err := a.RecordResults(items, s.clock.Now())
		if err != nil {
			return err
		}
		output = CommandResult{BatchID: command.BatchID, Revision: a.Batch.Revision, State: a.Batch.State}
		for _, item := range applied {
			output.Resources = append(output.Resources, CommandResource{ResourceID: item.ID, UnitID: item.UnitID, CheckCode: item.CheckCode, Attempt: item.Attempt, Outcome: item.Outcome})
		}
		encoded, _ := json.Marshal(output)
		facts := map[string]any{"count": len(applied), "results": output.Resources, "coverage": a.Coverage()}
		event, err := audit.NewEvent(s.nextID("evt"), command.BatchID, "check.batch_recorded", command.Actor, a.Batch.Revision, s.clock.Now(), facts)
		if err != nil {
			return err
		}
		return s.repo.Commit(ctx, Commit{Aggregate: a, ExpectedRev: expected, Event: event, IdempotencyKey: command.IdempotencyKey, Response: encoded})
	})
	return output, err
}

func (s *Service) OpenRemediation(ctx context.Context, command OpenRemediationCommand) (CommandResult, error) {
	return s.mutate(ctx, command.Meta, "remediation.opened", func(a *domain.Aggregate, now time.Time) (string, any, error) {
		if command.Remediation.ID == "" {
			command.Remediation.ID = s.nextID("rem")
		}
		err := a.OpenRemediation(command.Remediation, now)
		return command.Remediation.ID, command.Remediation, err
	})
}

func (s *Service) CompleteRemediation(ctx context.Context, command CompleteRemediationCommand) (CommandResult, error) {
	return s.mutate(ctx, command.Meta, "remediation.completed", func(a *domain.Aggregate, now time.Time) (string, any, error) {
		err := a.CompleteRemediation(command.RemediationID, command.ActionNote, command.EvidenceRefs, now)
		completed := a.RemediationByID(command.RemediationID)
		return command.RemediationID, map[string]any{"remediationId": command.RemediationID, "actionNote": command.ActionNote, "completedOverdue": completed != nil && completed.CompletedOverdue, "overdueSeconds": func() int64 {
			if completed != nil {
				return completed.OverdueSeconds
			}
			return 0
		}()}, err
	})
}

func (s *Service) ChangeRemediation(ctx context.Context, command ChangeRemediationCommand) (CommandResult, error) {
	return s.mutate(ctx, command.Meta, "remediation.changed", func(a *domain.Aggregate, now time.Time) (string, any, error) {
		before, err := a.ChangeRemediation(command.RemediationID, command.Owner, command.DueAt, command.Reason, now)
		if err != nil {
			return command.RemediationID, nil, err
		}
		after := a.RemediationByID(command.RemediationID)
		return command.RemediationID, map[string]any{"remediationId": command.RemediationID, "before": map[string]any{"owner": before.Owner, "dueAt": before.DueAt}, "after": map[string]any{"owner": after.Owner, "dueAt": after.DueAt}, "reason": command.Reason}, nil
	})
}

func (s *Service) Approve(ctx context.Context, command ApproveCommand) (CommandResult, error) {
	return s.mutate(ctx, command.Meta, "batch.approved", func(a *domain.Aggregate, now time.Time) (string, any, error) {
		manifest, err := a.BuildManifest()
		if err != nil {
			return "", nil, err
		}
		manifestDigest, err := audit.ManifestDigest(manifest)
		if err != nil {
			return "", nil, err
		}
		permits, err := s.permitSnapshot(ctx)
		if err != nil {
			return "", nil, err
		}
		sequence, previous := int64(len(permits)+1), audit.GenesisDigest
		if len(permits) > 0 {
			previous = permits[len(permits)-1].PermitDigest
		}
		permit := domain.AdmissionPermit{ID: s.nextID("permit"), BatchID: a.Batch.ID, Sequence: sequence, ManifestDigest: manifestDigest, PreviousDigest: previous, ApprovedBy: command.ApprovedBy, IssuedAt: now}
		for _, unit := range manifest.Units {
			permit.ApprovedUnitIDs = append(permit.ApprovedUnitIDs, unit.ID)
		}
		permit, err = audit.SignPermit(permit)
		if err != nil {
			return "", nil, err
		}
		if err := a.Approve(permit, now); err != nil {
			return "", nil, err
		}
		return permit.ID, permit, nil
	})
}
