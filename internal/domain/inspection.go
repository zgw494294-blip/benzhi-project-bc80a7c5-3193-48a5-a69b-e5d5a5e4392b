package domain

import (
	"sort"
	"strings"
	"time"
)

func (a *Aggregate) LatestResult(unitID, checkCode string) *CheckResult {
	var latest *CheckResult
	for i := range a.Results {
		candidate := &a.Results[i]
		if candidate.UnitID == unitID && candidate.CheckCode == checkCode && (latest == nil || candidate.Attempt > latest.Attempt) {
			latest = candidate
		}
	}
	return latest
}

func (a *Aggregate) RecordResult(result CheckResult, now time.Time) error {
	if err := a.ensureMutable(); err != nil {
		return err
	}
	if a.Plan == nil || (a.Batch.State != BatchSubmitted && a.Batch.State != BatchInspecting && a.Batch.State != BatchReady) {
		return Conflict("inspection_not_open", "当前批次不能录入检查结果")
	}
	if !a.IsFrozenUnit(result.UnitID) {
		return NotFound("frozen_unit", result.UnitID)
	}
	definition, err := a.CheckDefinition(result.CheckCode)
	if err != nil {
		return err
	}
	if err := ValidateID(result.ID, "resultId"); err != nil {
		return err
	}
	if result.Outcome != OutcomePass && result.Outcome != OutcomeFail {
		return Validation("invalid_outcome", "outcome 只能是 pass 或 fail")
	}
	if err := requireText(result.Inspector, "inspector", 80); err != nil {
		return err
	}
	if err := ValidateEvidenceDigest(result.EvidenceDigest, "evidenceDigest"); err != nil {
		return err
	}
	for _, existing := range a.Results {
		if existing.ID == result.ID {
			return Conflict("duplicate_result_id", "检查结果 ID 已存在")
		}
	}
	latest := a.LatestResult(result.UnitID, result.CheckCode)
	if latest == nil {
		result.Attempt = 1
	} else {
		if latest.Outcome != OutcomeFail {
			return Conflict("passed_check_final", "已合格项目不能重复录入")
		}
		remediation := a.RemediationForResult(latest.ID)
		if remediation == nil || remediation.Status != RemediationCompleted {
			return Conflict("remediation_not_completed", "整改完成后才能发起复测")
		}
		result.Attempt = latest.Attempt + 1
	}
	result.BatchID = a.Batch.ID
	result.RecordedAt = now.UTC()
	a.Results = append(a.Results, result)
	if latest != nil {
		remediation := a.RemediationForResult(latest.ID)
		if result.Outcome == OutcomePass {
			remediation.Status = RemediationClosed
			remediation.ClosedByResultID = result.ID
		} else {
			remediation.Status = RemediationClosed
			remediation.ClosedByResultID = result.ID
		}
	}
	_ = definition
	a.recalculateState()
	a.bump()
	return nil
}

// RecordResults 在副本上完成全部资格校验，仅在全部合法后一次替换结果并递增修订号。
func (a *Aggregate) RecordResults(results []CheckResult, now time.Time) ([]CheckResult, error) {
	if len(results) == 0 {
		return nil, Validation("empty_result_batch", "批量检查结果不能为空")
	}
	if len(results) > 100 {
		return nil, Validation("too_many_results", "一次最多提交 100 项检查结果")
	}
	ordered := append([]CheckResult(nil), results...)
	sort.Slice(ordered, func(i, j int) bool {
		leftUnit, _ := a.Unit(ordered[i].UnitID)
		rightUnit, _ := a.Unit(ordered[j].UnitID)
		leftCode, rightCode := ordered[i].UnitID, ordered[j].UnitID
		if leftUnit != nil {
			leftCode = strings.ToLower(leftUnit.UnitCode)
		}
		if rightUnit != nil {
			rightCode = strings.ToLower(rightUnit.UnitCode)
		}
		if leftCode == rightCode {
			return ordered[i].CheckCode < ordered[j].CheckCode
		}
		return leftCode < rightCode
	})
	seen := make(map[string]struct{}, len(ordered))
	for _, item := range ordered {
		key := item.UnitID + "\x00" + item.CheckCode
		if _, exists := seen[key]; exists {
			return nil, Conflict("duplicate_result_cell", "批量请求不能重复提交同一布景检查单元格")
		}
		seen[key] = struct{}{}
	}
	clone, err := CloneAggregate(a)
	if err != nil {
		return nil, err
	}
	start := len(clone.Results)
	for _, item := range ordered {
		if err := clone.RecordResult(item, now); err != nil {
			return nil, err
		}
	}
	applied := append([]CheckResult(nil), clone.Results[start:]...)
	a.Results = clone.Results
	a.Remediations = clone.Remediations
	a.Batch.State = clone.Batch.State
	a.bump()
	return applied, nil
}

func (a *Aggregate) OpenRemediation(remediation Remediation, now time.Time) error {
	if err := a.ensureMutable(); err != nil {
		return err
	}
	if err := ValidateID(remediation.ID, "remediationId"); err != nil {
		return err
	}
	if err := requireText(remediation.Owner, "owner", 80); err != nil {
		return err
	}
	if remediation.DueAt.IsZero() || !remediation.DueAt.After(now) {
		return Validation("invalid_due_at", "整改期限必须晚于当前时间")
	}
	failed := a.ResultByID(remediation.CheckResultID)
	if failed == nil || failed.Outcome != OutcomeFail {
		return Conflict("result_not_failed", "只能为不合格检查结果创建整改")
	}
	if latest := a.LatestResult(failed.UnitID, failed.CheckCode); latest == nil || latest.ID != failed.ID {
		return Conflict("result_not_latest", "只能为最新不合格结果创建整改")
	}
	if a.RemediationForResult(failed.ID) != nil {
		return Conflict("duplicate_remediation", "该不合格结果已有整改记录")
	}
	for _, existing := range a.Remediations {
		if existing.ID == remediation.ID {
			return Conflict("duplicate_remediation_id", "整改 ID 已存在")
		}
	}
	remediation.BatchID = a.Batch.ID
	remediation.Status = RemediationOpen
	remediation.DueAt = remediation.DueAt.UTC()
	a.Remediations = append(a.Remediations, remediation)
	a.recalculateState()
	a.bump()
	return nil
}

func (a *Aggregate) CompleteRemediation(id, actionNote string, evidence []EvidenceRef, now time.Time) error {
	if err := a.ensureMutable(); err != nil {
		return err
	}
	remediation := a.RemediationByID(id)
	if remediation == nil {
		return NotFound("remediation", id)
	}
	if remediation.Status != RemediationOpen {
		return Conflict("remediation_not_open", "仅待整改记录可以完成")
	}
	if err := requireText(actionNote, "actionNote", 500); err != nil {
		return err
	}
	if len(evidence) == 0 {
		return Validation("missing_remediation_evidence", "完成整改必须提供新证据")
	}
	for _, ref := range evidence {
		if err := requireText(ref.Name, "evidence.name", 160); err != nil {
			return err
		}
		if err := ValidateEvidenceDigest(ref.Digest, "evidence.digest"); err != nil {
			return err
		}
	}
	remediation.ActionNote = strings.TrimSpace(actionNote)
	remediation.EvidenceRefs = NormalizeEvidence(evidence)
	remediation.Status = RemediationCompleted
	t := now.UTC()
	remediation.CompletedAt = &t
	if now.After(remediation.DueAt) {
		remediation.CompletedOverdue = true
		remediation.OverdueSeconds = int64(now.Sub(remediation.DueAt).Seconds())
	}
	a.recalculateState()
	a.bump()
	return nil
}

func (a *Aggregate) ChangeRemediation(id, owner string, dueAt time.Time, reason string, now time.Time) (Remediation, error) {
	if err := a.ensureMutable(); err != nil {
		return Remediation{}, err
	}
	remediation := a.RemediationByID(id)
	if remediation == nil {
		return Remediation{}, NotFound("remediation", id)
	}
	if remediation.Status != RemediationOpen {
		return Remediation{}, Conflict("remediation_not_open", "仅 open 状态整改可以调整责任与期限")
	}
	if err := requireText(owner, "owner", 80); err != nil {
		return Remediation{}, err
	}
	if err := requireText(reason, "reason", 500); err != nil {
		return Remediation{}, err
	}
	if !dueAt.After(now) {
		return Remediation{}, Validation("invalid_due_at", "新期限必须晚于当前时间")
	}
	if dueAt.Before(remediation.DueAt) {
		return Remediation{}, Conflict("due_at_cannot_advance", "整改期限不得早于原期限")
	}
	before := *remediation
	if strings.TrimSpace(owner) == remediation.Owner && dueAt.Equal(remediation.DueAt) {
		return Remediation{}, Conflict("remediation_unchanged", "责任人或期限至少变更一项")
	}
	remediation.Owner = strings.TrimSpace(owner)
	remediation.DueAt = dueAt.UTC()
	a.bump()
	return before, nil
}

func (a *Aggregate) ResultByID(id string) *CheckResult {
	for i := range a.Results {
		if a.Results[i].ID == id {
			return &a.Results[i]
		}
	}
	return nil
}

func (a *Aggregate) RemediationByID(id string) *Remediation {
	for i := range a.Remediations {
		if a.Remediations[i].ID == id {
			return &a.Remediations[i]
		}
	}
	return nil
}

func (a *Aggregate) RemediationForResult(resultID string) *Remediation {
	for i := range a.Remediations {
		if a.Remediations[i].CheckResultID == resultID {
			return &a.Remediations[i]
		}
	}
	return nil
}

func (a *Aggregate) Matrix() []MatrixCell {
	if a.Plan == nil {
		return nil
	}
	cells := make([]MatrixCell, 0, len(a.Plan.FrozenUnitIDs)*len(a.Plan.CheckDefinitions))
	for _, unitID := range a.Plan.FrozenUnitIDs {
		for _, definition := range a.Plan.CheckDefinitions {
			latest := a.LatestResult(unitID, definition.Code)
			cell := MatrixCell{UnitID: unitID, CheckCode: definition.Code, Definition: definition, Latest: latest, Status: "pending"}
			if latest != nil {
				cell.Status = string(latest.Outcome)
				cell.Remediation = a.RemediationForResult(latest.ID)
				cell.Blocking = latest.Outcome == OutcomeFail && definition.Blocking
			}
			cells = append(cells, cell)
		}
	}
	sort.Slice(cells, func(i, j int) bool {
		left, right := cells[i].UnitID, cells[j].UnitID
		if unit, _ := a.Unit(cells[i].UnitID); unit != nil {
			left = strings.ToLower(unit.UnitCode)
		}
		if unit, _ := a.Unit(cells[j].UnitID); unit != nil {
			right = strings.ToLower(unit.UnitCode)
		}
		if left == right {
			return cells[i].CheckCode < cells[j].CheckCode
		}
		return left < right
	})
	return cells
}

func (a *Aggregate) BlockingCells() []MatrixCell {
	var result []MatrixCell
	for _, cell := range a.Matrix() {
		if cell.Blocking {
			result = append(result, cell)
		}
	}
	return result
}

func (a *Aggregate) CanApprove() error {
	if a.Batch.State == BatchApproved || a.Permit != nil {
		return Conflict("already_approved", "批次已经批准，不能重复批准")
	}
	if a.Plan == nil {
		return Conflict("plan_not_frozen", "检查方案尚未冻结")
	}
	for _, cell := range a.Matrix() {
		if cell.Latest == nil {
			return Conflict("checks_incomplete", "仍有必检项目待检")
		}
		if cell.Latest.Outcome != OutcomePass {
			return Conflict("checks_failed", "仍有不合格必检项目")
		}
	}
	for _, remediation := range a.Remediations {
		if remediation.Status != RemediationClosed {
			return Conflict("remediation_open", "仍有整改未闭环")
		}
	}
	return nil
}

func (a *Aggregate) recalculateState() {
	if a.Batch.State == BatchApproved {
		return
	}
	if a.Plan == nil {
		a.Batch.State = BatchDraft
		return
	}
	if a.CanApprove() == nil {
		a.Batch.State = BatchReady
		return
	}
	a.Batch.State = BatchInspecting
}

func (a *Aggregate) SortedResults() []CheckResult {
	result := append([]CheckResult(nil), a.Results...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].UnitID != result[j].UnitID {
			return result[i].UnitID < result[j].UnitID
		}
		if result[i].CheckCode != result[j].CheckCode {
			return result[i].CheckCode < result[j].CheckCode
		}
		return result[i].Attempt < result[j].Attempt
	})
	return result
}
