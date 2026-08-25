package domain

import (
	"sort"
	"strings"
	"time"
)

func validProgressStatus(value string) bool {
	switch value {
	case "", "pending", "pass", "fail", "blocking":
		return true
	}
	return false
}

func (a *Aggregate) Progress(filter ProgressFilter) (ProgressView, error) {
	if !validProgressStatus(filter.Status) {
		return ProgressView{}, Validation("invalid_progress_status", "status 只能是 pending、pass、fail 或 blocking")
	}
	view := ProgressView{Revision: a.Batch.Revision}
	if a.Plan == nil {
		return view, nil
	}
	for _, cell := range a.Matrix() {
		unit, err := a.Unit(cell.UnitID)
		if err != nil {
			continue
		}
		if filter.StageZone != "" && unit.StageZone != filter.StageZone {
			continue
		}
		if filter.MaterialClass != "" && unit.MaterialClass != filter.MaterialClass {
			continue
		}
		if filter.CheckCode != "" && cell.CheckCode != filter.CheckCode {
			continue
		}
		if filter.Inspector != "" && (cell.Latest == nil || cell.Latest.Inspector != filter.Inspector) {
			continue
		}
		if filter.Status != "" {
			if filter.Status == "blocking" && !cell.Blocking {
				continue
			}
			if filter.Status != "blocking" && cell.Status != filter.Status {
				continue
			}
		}
		view.Matrix = append(view.Matrix, cell)
	}
	sort.Slice(view.Matrix, func(i, j int) bool {
		rank := func(cell MatrixCell) int {
			if cell.Blocking {
				return 0
			}
			if cell.Status == "pending" {
				return 1
			}
			return 2
		}
		if rank(view.Matrix[i]) != rank(view.Matrix[j]) {
			return rank(view.Matrix[i]) < rank(view.Matrix[j])
		}
		ui, _ := a.Unit(view.Matrix[i].UnitID)
		uj, _ := a.Unit(view.Matrix[j].UnitID)
		ci, cj := view.Matrix[i].UnitID, view.Matrix[j].UnitID
		if ui != nil {
			ci = strings.ToLower(ui.UnitCode)
		}
		if uj != nil {
			cj = strings.ToLower(uj.UnitCode)
		}
		if ci == cj {
			return view.Matrix[i].CheckCode < view.Matrix[j].CheckCode
		}
		return ci < cj
	})
	grouped := map[string]*ProgressGroup{}
	for _, cell := range view.Matrix {
		unit, _ := a.Unit(cell.UnitID)
		key := unit.StageZone + "\x00" + unit.MaterialClass
		group := grouped[key]
		if group == nil {
			group = &ProgressGroup{StageZone: unit.StageZone, MaterialClass: unit.MaterialClass}
			grouped[key] = group
		}
		group.Total++
		switch cell.Status {
		case "pending":
			group.Pending++
		case "pass":
			group.Passed++
		case "fail":
			group.Failed++
		}
		if cell.Blocking {
			group.Blocking++
		}
	}
	for _, group := range grouped {
		if group.Total > 0 {
			group.Completion = float64(group.Passed+group.Failed) / float64(group.Total)
		}
		view.Groups = append(view.Groups, *group)
	}
	sort.Slice(view.Groups, func(i, j int) bool {
		if view.Groups[i].StageZone == view.Groups[j].StageZone {
			return view.Groups[i].MaterialClass < view.Groups[j].MaterialClass
		}
		return view.Groups[i].StageZone < view.Groups[j].StageZone
	})
	return view, nil
}

func remediationRisk(remediation Remediation, now time.Time) (DueRisk, int64) {
	if remediation.Status != RemediationOpen {
		return DueRiskNormal, 0
	}
	if now.After(remediation.DueAt) {
		return DueRiskOverdue, int64(now.Sub(remediation.DueAt).Seconds())
	}
	if remediation.DueAt.Sub(now) <= 24*time.Hour {
		return DueRiskSoon, 0
	}
	return DueRiskNormal, 0
}

func (a *Aggregate) RemediationQueue(owner, status, risk string, now time.Time) ([]RemediationQueueItem, error) {
	if status != "" && status != string(RemediationOpen) && status != string(RemediationCompleted) && status != string(RemediationClosed) {
		return nil, Validation("invalid_remediation_status", "remediationStatus 无效")
	}
	if risk != "" && risk != string(DueRiskNormal) && risk != string(DueRiskSoon) && risk != string(DueRiskOverdue) {
		return nil, Validation("invalid_due_risk", "dueRisk 只能是 normal、due_soon 或 overdue")
	}
	var items []RemediationQueueItem
	for _, remediation := range a.Remediations {
		if owner != "" && remediation.Owner != owner {
			continue
		}
		if status != "" && string(remediation.Status) != status {
			continue
		}
		dueRisk, overdue := remediationRisk(remediation, now)
		if risk != "" && string(dueRisk) != risk {
			continue
		}
		result := a.ResultByID(remediation.CheckResultID)
		if result == nil {
			continue
		}
		unit, _ := a.Unit(result.UnitID)
		item := RemediationQueueItem{Remediation: remediation, UnitID: result.UnitID, CheckCode: result.CheckCode, DueRisk: dueRisk, OverdueSeconds: overdue}
		if unit != nil {
			item.UnitCode = unit.UnitCode
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		rank := func(r DueRisk) int {
			if r == DueRiskOverdue {
				return 0
			}
			if r == DueRiskSoon {
				return 1
			}
			return 2
		}
		if rank(items[i].DueRisk) != rank(items[j].DueRisk) {
			return rank(items[i].DueRisk) < rank(items[j].DueRisk)
		}
		if !items[i].Remediation.DueAt.Equal(items[j].Remediation.DueAt) {
			return items[i].Remediation.DueAt.Before(items[j].Remediation.DueAt)
		}
		return items[i].Remediation.ID < items[j].Remediation.ID
	})
	return items, nil
}
