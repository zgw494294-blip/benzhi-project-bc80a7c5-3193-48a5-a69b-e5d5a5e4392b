package domain

import (
	"fmt"
	"sort"
	"strings"
)

// ValidateIntegrity 核验从持久化适配器恢复的整个聚合，而不是修复数据。
func (a *Aggregate) ValidateIntegrity() error {
	if a == nil {
		return fmt.Errorf("批次聚合为空")
	}
	if err := ValidateID(a.Batch.ID, "batchId"); err != nil {
		return err
	}
	if a.Batch.Revision < 1 {
		return fmt.Errorf("批次修订号必须大于零")
	}
	if err := ValidateBatchFields(a.Batch.Title, a.Batch.Venue, a.Batch.Coordinator, a.Batch.PerformanceAt); err != nil {
		return err
	}
	if !validBatchState(a.Batch.State) {
		return fmt.Errorf("未知批次状态 %q", a.Batch.State)
	}
	unitIDs := make(map[string]struct{}, len(a.Units))
	unitCodes := make(map[string]struct{}, len(a.Units))
	for _, unit := range a.Units {
		if unit.BatchID != a.Batch.ID {
			return fmt.Errorf("布景单元 %s 的 batchId 不一致", unit.ID)
		}
		if err := ValidateID(unit.ID, "unitId"); err != nil {
			return err
		}
		if err := ValidateUnit(unit); err != nil {
			return fmt.Errorf("布景单元 %s 数据无效: %w", unit.ID, err)
		}
		if _, exists := unitIDs[unit.ID]; exists {
			return fmt.Errorf("布景单元 ID %s 重复", unit.ID)
		}
		unitIDs[unit.ID] = struct{}{}
		code := strings.ToLower(unit.UnitCode)
		if _, exists := unitCodes[code]; exists {
			return fmt.Errorf("布景编号 %s 重复", unit.UnitCode)
		}
		unitCodes[code] = struct{}{}
	}
	if err := a.validatePlanIntegrity(unitIDs); err != nil {
		return err
	}
	if err := a.validateResultIntegrity(unitIDs); err != nil {
		return err
	}
	if err := a.validateRemediationIntegrity(); err != nil {
		return err
	}
	return a.validateApprovalIntegrity()
}

func validBatchState(state BatchState) bool {
	switch state {
	case BatchDraft, BatchSubmitted, BatchInspecting, BatchReady, BatchApproved:
		return true
	}
	return false
}

func (a *Aggregate) validatePlanIntegrity(unitIDs map[string]struct{}) error {
	if a.Plan == nil {
		if a.Batch.State != BatchDraft {
			return fmt.Errorf("非草稿批次缺少冻结检查方案")
		}
		if len(a.Results) != 0 || len(a.Remediations) != 0 || a.Permit != nil {
			return fmt.Errorf("无方案批次含有后续流程数据")
		}
		return nil
	}
	if a.Batch.State == BatchDraft {
		return fmt.Errorf("草稿批次不应包含冻结方案")
	}
	if a.Plan.BatchID != a.Batch.ID {
		return fmt.Errorf("检查方案 batchId 不一致")
	}
	if err := ValidateID(a.Plan.ID, "planId"); err != nil {
		return err
	}
	if err := ValidateDefinitions(a.Plan.CheckDefinitions); err != nil {
		return err
	}
	if len(a.Plan.FrozenUnitIDs) != len(unitIDs) {
		return fmt.Errorf("冻结布景数与登记布景数不一致")
	}
	previous := ""
	for _, id := range a.Plan.FrozenUnitIDs {
		if _, exists := unitIDs[id]; !exists {
			return fmt.Errorf("冻结方案引用未知布景单元 %s", id)
		}
		if previous != "" && id <= previous {
			return fmt.Errorf("冻结布景 ID 必须严格排序且不可重复")
		}
		previous = id
	}
	return nil
}

func (a *Aggregate) validateResultIntegrity(unitIDs map[string]struct{}) error {
	resultIDs := make(map[string]struct{}, len(a.Results))
	attempts := make(map[string][]int)
	for _, result := range a.Results {
		if result.BatchID != a.Batch.ID {
			return fmt.Errorf("检查结果 %s 的 batchId 不一致", result.ID)
		}
		if _, exists := resultIDs[result.ID]; exists {
			return fmt.Errorf("检查结果 ID %s 重复", result.ID)
		}
		resultIDs[result.ID] = struct{}{}
		if _, exists := unitIDs[result.UnitID]; !exists {
			return fmt.Errorf("检查结果 %s 引用未知布景", result.ID)
		}
		if _, err := a.CheckDefinition(result.CheckCode); err != nil {
			return fmt.Errorf("检查结果 %s 引用未知检查项目", result.ID)
		}
		if result.Outcome != OutcomePass && result.Outcome != OutcomeFail {
			return fmt.Errorf("检查结果 %s 的结论无效", result.ID)
		}
		if result.Attempt < 1 {
			return fmt.Errorf("检查结果 %s 的尝试号无效", result.ID)
		}
		key := result.UnitID + "\x00" + result.CheckCode
		attempts[key] = append(attempts[key], result.Attempt)
	}
	for key, sequence := range attempts {
		sort.Ints(sequence)
		for index, attempt := range sequence {
			if attempt != index+1 {
				return fmt.Errorf("检查项 %q 的尝试号不连续", key)
			}
		}
	}
	return nil
}

func (a *Aggregate) validateRemediationIntegrity() error {
	ids := make(map[string]struct{}, len(a.Remediations))
	resultLinks := make(map[string]struct{}, len(a.Remediations))
	for _, remediation := range a.Remediations {
		if remediation.BatchID != a.Batch.ID {
			return fmt.Errorf("整改 %s 的 batchId 不一致", remediation.ID)
		}
		if _, exists := ids[remediation.ID]; exists {
			return fmt.Errorf("整改 ID %s 重复", remediation.ID)
		}
		ids[remediation.ID] = struct{}{}
		if _, exists := resultLinks[remediation.CheckResultID]; exists {
			return fmt.Errorf("检查结果 %s 关联多个整改", remediation.CheckResultID)
		}
		resultLinks[remediation.CheckResultID] = struct{}{}
		failed := a.ResultByID(remediation.CheckResultID)
		if failed == nil || failed.Outcome != OutcomeFail {
			return fmt.Errorf("整改 %s 未关联不合格结果", remediation.ID)
		}
		switch remediation.Status {
		case RemediationOpen:
			if remediation.CompletedAt != nil || remediation.ClosedByResultID != "" {
				return fmt.Errorf("待整改记录 %s 含有完成字段", remediation.ID)
			}
		case RemediationCompleted:
			if remediation.CompletedAt == nil || strings.TrimSpace(remediation.ActionNote) == "" || len(remediation.EvidenceRefs) == 0 {
				return fmt.Errorf("已完成整改 %s 缺少完成事实", remediation.ID)
			}
		case RemediationClosed:
			if remediation.CompletedAt == nil || a.ResultByID(remediation.ClosedByResultID) == nil {
				return fmt.Errorf("已关闭整改 %s 缺少复测结果", remediation.ID)
			}
		default:
			return fmt.Errorf("整改 %s 状态无效", remediation.ID)
		}
	}
	return nil
}

func (a *Aggregate) validateApprovalIntegrity() error {
	if a.Permit == nil {
		if a.Batch.State == BatchApproved || a.Batch.ApprovedAt != nil {
			return fmt.Errorf("批准状态缺少准用凭据")
		}
		return nil
	}
	if a.Batch.State != BatchApproved || a.Batch.ApprovedAt == nil {
		return fmt.Errorf("准用凭据与批次状态不一致")
	}
	if a.Permit.BatchID != a.Batch.ID {
		return fmt.Errorf("准用凭据 batchId 不一致")
	}
	if a.Permit.Sequence < 1 || a.Permit.ManifestDigest == "" || a.Permit.PermitDigest == "" {
		return fmt.Errorf("准用凭据必要字段缺失")
	}
	if len(a.Permit.ApprovedUnitIDs) != len(a.Plan.FrozenUnitIDs) {
		return fmt.Errorf("准用清单遗漏冻结布景")
	}
	for index := range a.Plan.FrozenUnitIDs {
		if a.Permit.ApprovedUnitIDs[index] != a.Plan.FrozenUnitIDs[index] {
			return fmt.Errorf("准用清单与冻结布景不一致")
		}
	}
	return nil
}
