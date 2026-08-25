package application

import (
	"scenicpermit/internal/domain"
	"time"
)

type Meta struct {
	BatchID        string `json:"batchId"`
	Revision       int64  `json:"revision"`
	Actor          string `json:"actor"`
	IdempotencyKey string `json:"idempotencyKey"`
}
type CreateBatchCommand struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	Venue          string    `json:"venue"`
	PerformanceAt  time.Time `json:"performanceAt"`
	Coordinator    string    `json:"coordinator"`
	Actor          string    `json:"actor"`
	IdempotencyKey string    `json:"idempotencyKey"`
}
type UpdateBatchCommand struct {
	Meta
	Title         string    `json:"title"`
	Venue         string    `json:"venue"`
	PerformanceAt time.Time `json:"performanceAt"`
	Coordinator   string    `json:"coordinator"`
}
type AddUnitCommand struct {
	Meta
	Unit domain.SceneryUnit `json:"unit"`
}
type UpdateUnitCommand struct {
	Meta
	UnitID string             `json:"unitId"`
	Unit   domain.SceneryUnit `json:"unit"`
}
type RemoveUnitCommand struct {
	Meta
	UnitID string `json:"unitId"`
}
type SubmitPlanCommand struct {
	Meta
	PlanID             string                   `json:"planId"`
	Definitions        []domain.CheckDefinition `json:"checkDefinitions"`
	ConfirmationDigest string                   `json:"confirmationDigest"`
}
type RecordResultCommand struct {
	Meta
	Result domain.CheckResult `json:"result"`
}
type RecordResultsCommand struct {
	Meta
	Results []domain.CheckResult `json:"results"`
}
type OpenRemediationCommand struct {
	Meta
	Remediation domain.Remediation `json:"remediation"`
}
type CompleteRemediationCommand struct {
	Meta
	RemediationID string               `json:"remediationId"`
	ActionNote    string               `json:"actionNote"`
	EvidenceRefs  []domain.EvidenceRef `json:"evidenceRefs"`
}
type ChangeRemediationCommand struct {
	Meta
	RemediationID string    `json:"remediationId"`
	Owner         string    `json:"owner"`
	DueAt         time.Time `json:"dueAt"`
	Reason        string    `json:"reason"`
}
type ApproveCommand struct {
	Meta
	ApprovedBy string `json:"approvedBy"`
}
type CommandResult struct {
	BatchID    string            `json:"batchId"`
	Revision   int64             `json:"revision"`
	State      domain.BatchState `json:"state"`
	ResourceID string            `json:"resourceId,omitempty"`
	Replay     bool              `json:"replay,omitempty"`
	Resources  []CommandResource `json:"resources,omitempty"`
}
type CommandResource struct {
	ResourceID string         `json:"resourceId"`
	UnitID     string         `json:"unitId,omitempty"`
	CheckCode  string         `json:"checkCode,omitempty"`
	Attempt    int            `json:"attempt,omitempty"`
	Outcome    domain.Outcome `json:"outcome,omitempty"`
}
