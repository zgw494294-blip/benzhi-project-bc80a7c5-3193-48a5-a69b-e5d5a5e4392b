package domain

import "sort"

type CoverageSummary struct {
	Total                 int `json:"total"`
	Pending               int `json:"pending"`
	Passed                int `json:"passed"`
	Failed                int `json:"failed"`
	Blocking              int `json:"blocking"`
	OpenRemediations      int `json:"openRemediations"`
	CompletedRemediations int `json:"completedRemediations"`
	ClosedRemediations    int `json:"closedRemediations"`
}

func (a *Aggregate) Coverage() CoverageSummary {
	var summary CoverageSummary
	for _, cell := range a.Matrix() {
		summary.Total++
		switch cell.Status {
		case "pending":
			summary.Pending++
		case string(OutcomePass):
			summary.Passed++
		case string(OutcomeFail):
			summary.Failed++
		}
		if cell.Blocking {
			summary.Blocking++
		}
	}
	for _, remediation := range a.Remediations {
		switch remediation.Status {
		case RemediationOpen:
			summary.OpenRemediations++
		case RemediationCompleted:
			summary.CompletedRemediations++
		case RemediationClosed:
			summary.ClosedRemediations++
		}
	}
	return summary
}

type CheckHistory struct {
	UnitID       string        `json:"unitId"`
	UnitCode     string        `json:"unitCode"`
	UnitName     string        `json:"unitName"`
	CheckCode    string        `json:"checkCode"`
	CheckName    string        `json:"checkName"`
	Attempts     []CheckResult `json:"attempts"`
	Remediations []Remediation `json:"remediations"`
}

func (a *Aggregate) Histories() []CheckHistory {
	if a.Plan == nil {
		return nil
	}
	result := make([]CheckHistory, 0, len(a.Plan.FrozenUnitIDs)*len(a.Plan.CheckDefinitions))
	for _, unitID := range a.Plan.FrozenUnitIDs {
		unit, _ := a.Unit(unitID)
		for _, definition := range a.Plan.CheckDefinitions {
			history := CheckHistory{UnitID: unitID, UnitCode: unit.UnitCode, UnitName: unit.Name, CheckCode: definition.Code, CheckName: definition.Name}
			for _, attempt := range a.Results {
				if attempt.UnitID == unitID && attempt.CheckCode == definition.Code {
					history.Attempts = append(history.Attempts, attempt)
				}
			}
			sort.Slice(history.Attempts, func(i, j int) bool { return history.Attempts[i].Attempt < history.Attempts[j].Attempt })
			for _, remediation := range a.Remediations {
				for _, attempt := range history.Attempts {
					if remediation.CheckResultID == attempt.ID {
						history.Remediations = append(history.Remediations, remediation)
						break
					}
				}
			}
			result = append(result, history)
		}
	}
	return result
}
