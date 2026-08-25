package domain

import (
	"sort"
	"strings"
)

func (a *Aggregate) PreflightPlan(definitions []CheckDefinition) PlanPreflight {
	view := PlanPreflight{BatchID: a.Batch.ID, Revision: a.Batch.Revision}
	view.Units = append([]SceneryUnit(nil), a.Units...)
	view.Definitions = NormalizeDefinitions(definitions)
	if a.Batch.State != BatchDraft {
		view.Diagnostics = append(view.Diagnostics, PlanDiagnostic{Code: "batch_locked", Message: "仅草稿批次可以预检送检方案", Blocking: true})
	}
	if len(a.Units) == 0 {
		view.Diagnostics = append(view.Diagnostics, PlanDiagnostic{Code: "no_units", Message: "至少登记一个布景单元", Blocking: true})
	}
	seen := map[string]bool{}
	required := make([]CheckDefinition, 0, len(definitions))
	for _, definition := range definitions {
		key := strings.ToLower(strings.TrimSpace(definition.Code))
		if key != "" && seen[key] {
			view.Diagnostics = append(view.Diagnostics, PlanDiagnostic{Code: "duplicate_check_code", Message: "检查项目编号重复：" + definition.Code, Blocking: true})
		}
		seen[key] = true
		if err := ValidateID(definition.Code, "checkCode"); err != nil {
			view.Diagnostics = append(view.Diagnostics, PlanDiagnostic{Code: "invalid_check_code", Message: err.Error(), Blocking: true})
			continue
		}
		if err := requireText(definition.Name, "check.name", 100); err != nil {
			view.Diagnostics = append(view.Diagnostics, PlanDiagnostic{Code: "invalid_check_name", Message: err.Error(), Blocking: true})
			continue
		}
		if err := requireText(definition.Criterion, "check.criterion", 240); err != nil {
			view.Diagnostics = append(view.Diagnostics, PlanDiagnostic{Code: "invalid_check_criterion", Message: err.Error(), Blocking: true})
			continue
		}
		if definition.Required {
			required = append(required, definition)
			if definition.Blocking {
				view.Summary.BlockingCheckCount++
			}
		} else {
			view.Diagnostics = append(view.Diagnostics, PlanDiagnostic{Code: "optional_check_not_supported", Message: "方案中的检查项目必须标记为必检：" + definition.Code, Blocking: true})
		}
	}
	if len(required) == 0 {
		view.Diagnostics = append(view.Diagnostics, PlanDiagnostic{Code: "no_required_checks", Message: "方案至少包含一个必检项目", Blocking: true})
	}
	units := append([]SceneryUnit(nil), a.Units...)
	sort.Slice(units, func(i, j int) bool {
		left, right := strings.ToLower(units[i].UnitCode), strings.ToLower(units[j].UnitCode)
		if left == right {
			return units[i].ID < units[j].ID
		}
		return left < right
	})
	sort.Slice(required, func(i, j int) bool { return required[i].Code < required[j].Code })
	for _, unit := range units {
		if err := ValidateUnit(unit); err != nil {
			view.Diagnostics = append(view.Diagnostics, PlanDiagnostic{Code: "incomplete_unit", Message: "布景 " + unit.UnitCode + " 登记不完整：" + err.Error(), Blocking: true})
		}
		for _, definition := range required {
			view.Coverage = append(view.Coverage, PlanCoverageCell{UnitID: unit.ID, UnitCode: unit.UnitCode, UnitName: unit.Name, CheckCode: definition.Code, CheckName: definition.Name, Blocking: definition.Blocking})
		}
	}
	view.Summary.UnitCount = len(units)
	view.Summary.RequiredCheckCount = len(required)
	view.Summary.TotalCheckCount = len(view.Coverage)
	view.Confirmable = len(view.Diagnostics) == 0
	return view
}
