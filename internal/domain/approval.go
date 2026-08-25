package domain

import (
	"encoding/json"
	"sort"
	"time"
)

type Manifest struct {
	BatchID     string          `json:"batchId"`
	Revision    int64           `json:"revision"`
	Title       string          `json:"title"`
	Venue       string          `json:"venue"`
	Performance string          `json:"performanceAt"`
	Units       []ManifestUnit  `json:"units"`
	Checks      []ManifestCheck `json:"checks"`
}

type ManifestUnit struct {
	ID            string `json:"id"`
	UnitCode      string `json:"unitCode"`
	Name          string `json:"name"`
	StageZone     string `json:"stageZone"`
	MaterialClass string `json:"materialClass"`
	TreatmentLot  string `json:"treatmentLot"`
}

type ManifestCheck struct {
	UnitID         string `json:"unitId"`
	CheckCode      string `json:"checkCode"`
	ResultID       string `json:"resultId"`
	Attempt        int    `json:"attempt"`
	EvidenceDigest string `json:"evidenceDigest"`
	Inspector      string `json:"inspector"`
}

func (a *Aggregate) BuildManifest() (Manifest, error) {
	if err := a.CanApprove(); err != nil {
		return Manifest{}, err
	}
	return a.buildManifest(a.Batch.Revision + 1)
}

// FrozenManifest 根据批准后不可变的领域事实重建签发时的规范清单。
func (a *Aggregate) FrozenManifest() (Manifest, error) {
	if a.Batch.State != BatchApproved || a.Permit == nil {
		return Manifest{}, Conflict("permit_not_issued", "批次尚未签发准用凭据")
	}
	return a.buildManifest(a.Batch.Revision)
}

func (a *Aggregate) buildManifest(revision int64) (Manifest, error) {
	if a.Plan == nil {
		return Manifest{}, Conflict("plan_not_frozen", "检查方案尚未冻结")
	}
	manifest := Manifest{BatchID: a.Batch.ID, Revision: revision, Title: a.Batch.Title,
		Venue: a.Batch.Venue, Performance: a.Batch.PerformanceAt.UTC().Format(time.RFC3339Nano)}
	for _, id := range a.Plan.FrozenUnitIDs {
		unit, err := a.Unit(id)
		if err != nil {
			return Manifest{}, Conflict("frozen_unit_missing", "冻结清单引用的布景单元不存在")
		}
		manifest.Units = append(manifest.Units, ManifestUnit{ID: unit.ID, UnitCode: unit.UnitCode,
			Name: unit.Name, StageZone: unit.StageZone, MaterialClass: unit.MaterialClass, TreatmentLot: unit.TreatmentLot})
	}
	for _, cell := range a.Matrix() {
		if cell.Latest == nil || cell.Latest.Outcome != OutcomePass {
			return Manifest{}, Conflict("manifest_check_invalid", "规范清单包含未合格的必检项目")
		}
		manifest.Checks = append(manifest.Checks, ManifestCheck{UnitID: cell.UnitID, CheckCode: cell.CheckCode,
			ResultID: cell.Latest.ID, Attempt: cell.Latest.Attempt, EvidenceDigest: cell.Latest.EvidenceDigest, Inspector: cell.Latest.Inspector})
	}
	sort.Slice(manifest.Units, func(i, j int) bool { return manifest.Units[i].ID < manifest.Units[j].ID })
	sort.Slice(manifest.Checks, func(i, j int) bool {
		if manifest.Checks[i].UnitID == manifest.Checks[j].UnitID {
			return manifest.Checks[i].CheckCode < manifest.Checks[j].CheckCode
		}
		return manifest.Checks[i].UnitID < manifest.Checks[j].UnitID
	})
	return manifest, nil
}

func (m Manifest) CanonicalJSON() ([]byte, error) { return json.Marshal(m) }

func (a *Aggregate) Approve(permit AdmissionPermit, now time.Time) error {
	if err := a.CanApprove(); err != nil {
		return err
	}
	if permit.BatchID != a.Batch.ID {
		return Validation("permit_batch_mismatch", "凭据批次与当前批次不一致")
	}
	if permit.Sequence < 1 {
		return Validation("invalid_permit_sequence", "凭据序号必须大于零")
	}
	if permit.ManifestDigest == "" || permit.PermitDigest == "" {
		return Validation("missing_permit_digest", "凭据摘要不能为空")
	}
	if err := requireText(permit.ApprovedBy, "approvedBy", 80); err != nil {
		return err
	}
	manifest, err := a.BuildManifest()
	if err != nil {
		return err
	}
	permit.ApprovedUnitIDs = make([]string, len(manifest.Units))
	for i, unit := range manifest.Units {
		permit.ApprovedUnitIDs[i] = unit.ID
	}
	permit.IssuedAt = now.UTC()
	a.Permit = &permit
	t := now.UTC()
	a.Batch.ApprovedAt = &t
	a.Batch.State = BatchApproved
	a.bump()
	return nil
}

func CloneAggregate(input *Aggregate) (*Aggregate, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	var output Aggregate
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, err
	}
	return &output, nil
}
