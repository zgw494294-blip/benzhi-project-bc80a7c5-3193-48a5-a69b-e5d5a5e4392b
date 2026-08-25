package domain

import (
	"sort"
	"strings"
	"time"
)

func NewAggregate(id, title, venue, coordinator string, performanceAt, now time.Time) (*Aggregate, error) {
	if err := ValidateID(id, "batchId"); err != nil {
		return nil, err
	}
	if err := ValidateBatchFields(title, venue, coordinator, performanceAt); err != nil {
		return nil, err
	}
	return &Aggregate{Batch: InspectionBatch{
		ID: id, Title: strings.TrimSpace(title), Venue: strings.TrimSpace(venue),
		PerformanceAt: performanceAt.UTC(), Coordinator: strings.TrimSpace(coordinator),
		State: BatchDraft, Revision: 1, CreatedAt: now.UTC(),
	}}, nil
}

func (a *Aggregate) ensureMutable() error {
	if a.Batch.State == BatchApproved || a.Permit != nil {
		return Immutable("已批准批次的登记、方案、检查与整改数据不可修改")
	}
	return nil
}

func (a *Aggregate) RequireRevision(revision int64) error {
	if revision != a.Batch.Revision {
		return Conflict("stale_revision", "表单修订号已过期，请刷新后重试")
	}
	return nil
}

func (a *Aggregate) UpdateBatch(title, venue, coordinator string, performanceAt time.Time) error {
	if err := a.ensureMutable(); err != nil {
		return err
	}
	if a.Batch.State != BatchDraft {
		return Conflict("batch_locked", "送检后不能修改批次基础信息")
	}
	if err := ValidateBatchFields(title, venue, coordinator, performanceAt); err != nil {
		return err
	}
	a.Batch.Title, a.Batch.Venue = strings.TrimSpace(title), strings.TrimSpace(venue)
	a.Batch.Coordinator, a.Batch.PerformanceAt = strings.TrimSpace(coordinator), performanceAt.UTC()
	a.bump()
	return nil
}

func (a *Aggregate) AddUnit(unit SceneryUnit, now time.Time) error {
	if err := a.ensureMutable(); err != nil {
		return err
	}
	if a.Batch.State != BatchDraft {
		return Conflict("registration_locked", "送检后布景登记信息已锁定")
	}
	unit.BatchID = a.Batch.ID
	if err := ValidateUnit(unit); err != nil {
		return err
	}
	for _, existing := range a.Units {
		if existing.ID == unit.ID {
			return Conflict("duplicate_unit_id", "布景单元 ID 已存在")
		}
		if strings.EqualFold(existing.UnitCode, unit.UnitCode) {
			return Conflict("duplicate_unit_code", "本批次布景编号必须唯一")
		}
	}
	if err := ValidateID(unit.ID, "unitId"); err != nil {
		return err
	}
	unit.RegisteredAt = now.UTC()
	unit.EvidenceRefs = NormalizeEvidence(unit.EvidenceRefs)
	a.Units = append(a.Units, unit)
	a.bump()
	return nil
}

func (a *Aggregate) Unit(id string) (*SceneryUnit, error) {
	for i := range a.Units {
		if a.Units[i].ID == id {
			return &a.Units[i], nil
		}
	}
	return nil, NotFound("unit", id)
}

func (a *Aggregate) UpdateUnit(id string, replacement SceneryUnit) (SceneryUnit, error) {
	if a.Batch.State != BatchDraft {
		return SceneryUnit{}, Conflict("registration_locked", "送检后布景登记信息已锁定")
	}
	unit, err := a.Unit(id)
	if err != nil {
		return SceneryUnit{}, err
	}
	before := *unit
	replacement.ID, replacement.BatchID = unit.ID, a.Batch.ID
	replacement.RegisteredAt = unit.RegisteredAt
	if err := ValidateUnit(replacement); err != nil {
		return SceneryUnit{}, err
	}
	for _, existing := range a.Units {
		if existing.ID != id && strings.EqualFold(existing.UnitCode, replacement.UnitCode) {
			return SceneryUnit{}, Conflict("duplicate_unit_code", "本批次布景编号必须唯一")
		}
	}
	replacement.UnitCode = strings.TrimSpace(replacement.UnitCode)
	replacement.Name = strings.TrimSpace(replacement.Name)
	replacement.StageZone = strings.TrimSpace(replacement.StageZone)
	replacement.MaterialClass = strings.TrimSpace(replacement.MaterialClass)
	replacement.Supplier = strings.TrimSpace(replacement.Supplier)
	replacement.TreatmentLot = strings.TrimSpace(replacement.TreatmentLot)
	replacement.EvidenceRefs = NormalizeEvidence(replacement.EvidenceRefs)
	*unit = replacement
	a.bump()
	return before, nil
}

func (a *Aggregate) RemoveUnit(id string) (SceneryUnit, error) {
	if a.Batch.State != BatchDraft {
		return SceneryUnit{}, Conflict("registration_locked", "送检后布景登记信息已锁定")
	}
	for i, unit := range a.Units {
		if unit.ID == id {
			a.Units = append(a.Units[:i], a.Units[i+1:]...)
			a.bump()
			return unit, nil
		}
	}
	return SceneryUnit{}, NotFound("unit", id)
}

func (a *Aggregate) FreezePlan(id, actor string, definitions []CheckDefinition, now time.Time) error {
	if err := a.ensureMutable(); err != nil {
		return err
	}
	if a.Batch.State != BatchDraft {
		return Conflict("plan_already_frozen", "检查方案已经冻结")
	}
	if len(a.Units) == 0 {
		return Validation("no_units", "至少登记一个布景单元后才能送检")
	}
	if err := ValidateID(id, "planId"); err != nil {
		return err
	}
	if err := requireText(actor, "createdBy", 80); err != nil {
		return err
	}
	if err := ValidateDefinitions(definitions); err != nil {
		return err
	}
	unitIDs := make([]string, 0, len(a.Units))
	for _, unit := range a.Units {
		if err := ValidateUnit(unit); err != nil {
			return Validation("incomplete_unit", "存在登记不完整的布景单元："+unit.UnitCode)
		}
		unitIDs = append(unitIDs, unit.ID)
	}
	sort.Strings(unitIDs)
	a.Plan = &InspectionPlan{ID: id, BatchID: a.Batch.ID, PlanRevision: a.Batch.Revision + 1,
		CheckDefinitions: NormalizeDefinitions(definitions), FrozenUnitIDs: unitIDs,
		CreatedBy: strings.TrimSpace(actor), FrozenAt: now.UTC()}
	t := now.UTC()
	a.Batch.SubmittedAt = &t
	a.Batch.State = BatchSubmitted
	a.bump()
	return nil
}

func (a *Aggregate) bump() { a.Batch.Revision++ }

func (a *Aggregate) CheckDefinition(code string) (CheckDefinition, error) {
	if a.Plan == nil {
		return CheckDefinition{}, Conflict("plan_not_frozen", "检查方案尚未冻结")
	}
	for _, definition := range a.Plan.CheckDefinitions {
		if definition.Code == code {
			return definition, nil
		}
	}
	return CheckDefinition{}, NotFound("check", code)
}

func (a *Aggregate) IsFrozenUnit(id string) bool {
	if a.Plan == nil {
		return false
	}
	i := sort.SearchStrings(a.Plan.FrozenUnitIDs, id)
	return i < len(a.Plan.FrozenUnitIDs) && a.Plan.FrozenUnitIDs[i] == id
}
