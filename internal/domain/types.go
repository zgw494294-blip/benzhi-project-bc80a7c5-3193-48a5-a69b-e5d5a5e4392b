package domain

import "time"

type BatchState string

const (
	BatchDraft      BatchState = "draft"
	BatchSubmitted  BatchState = "submitted"
	BatchInspecting BatchState = "inspecting"
	BatchReady      BatchState = "ready"
	BatchApproved   BatchState = "approved"
)

type Outcome string

const (
	OutcomePass Outcome = "pass"
	OutcomeFail Outcome = "fail"
)

type RemediationStatus string

const (
	RemediationOpen      RemediationStatus = "open"
	RemediationCompleted RemediationStatus = "completed"
	RemediationClosed    RemediationStatus = "closed"
)

type InspectionBatch struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Venue         string     `json:"venue"`
	PerformanceAt time.Time  `json:"performanceAt"`
	Coordinator   string     `json:"coordinator"`
	State         BatchState `json:"state"`
	Revision      int64      `json:"revision"`
	CreatedAt     time.Time  `json:"createdAt"`
	SubmittedAt   *time.Time `json:"submittedAt,omitempty"`
	ApprovedAt    *time.Time `json:"approvedAt,omitempty"`
}

type EvidenceRef struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	Kind   string `json:"kind,omitempty"`
}

type SceneryUnit struct {
	ID            string        `json:"id"`
	BatchID       string        `json:"batchId"`
	UnitCode      string        `json:"unitCode"`
	Name          string        `json:"name"`
	StageZone     string        `json:"stageZone"`
	MaterialClass string        `json:"materialClass"`
	Supplier      string        `json:"supplier"`
	TreatmentLot  string        `json:"treatmentLot"`
	EvidenceRefs  []EvidenceRef `json:"evidenceRefs"`
	RegisteredAt  time.Time     `json:"registeredAt"`
}

type CheckDefinition struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Criterion string `json:"criterion"`
	Required  bool   `json:"required"`
	Blocking  bool   `json:"blocking"`
}

type InspectionPlan struct {
	ID               string            `json:"id"`
	BatchID          string            `json:"batchId"`
	PlanRevision     int64             `json:"planRevision"`
	CheckDefinitions []CheckDefinition `json:"checkDefinitions"`
	FrozenUnitIDs    []string          `json:"frozenUnitIds"`
	CreatedBy        string            `json:"createdBy"`
	FrozenAt         time.Time         `json:"frozenAt"`
}

type CheckResult struct {
	ID             string    `json:"id"`
	BatchID        string    `json:"batchId"`
	UnitID         string    `json:"unitId"`
	CheckCode      string    `json:"checkCode"`
	Attempt        int       `json:"attempt"`
	Outcome        Outcome   `json:"outcome"`
	MeasuredValue  string    `json:"measuredValue"`
	EvidenceDigest string    `json:"evidenceDigest"`
	Inspector      string    `json:"inspector"`
	RecordedAt     time.Time `json:"recordedAt"`
}

type Remediation struct {
	ID               string            `json:"id"`
	BatchID          string            `json:"batchId"`
	CheckResultID    string            `json:"checkResultId"`
	Owner            string            `json:"owner"`
	DueAt            time.Time         `json:"dueAt"`
	Status           RemediationStatus `json:"status"`
	ActionNote       string            `json:"actionNote,omitempty"`
	EvidenceRefs     []EvidenceRef     `json:"evidenceRefs,omitempty"`
	CompletedAt      *time.Time        `json:"completedAt,omitempty"`
	ClosedByResultID string            `json:"closedByResultId,omitempty"`
	CompletedOverdue bool              `json:"completedOverdue,omitempty"`
	OverdueSeconds   int64             `json:"overdueSeconds,omitempty"`
}

type AdmissionPermit struct {
	ID              string    `json:"id"`
	BatchID         string    `json:"batchId"`
	Sequence        int64     `json:"sequence"`
	ApprovedUnitIDs []string  `json:"approvedUnitIds"`
	ManifestDigest  string    `json:"manifestDigest"`
	PreviousDigest  string    `json:"previousDigest"`
	PermitDigest    string    `json:"permitDigest"`
	ApprovedBy      string    `json:"approvedBy"`
	IssuedAt        time.Time `json:"issuedAt"`
}

type Aggregate struct {
	Batch        InspectionBatch  `json:"batch"`
	Units        []SceneryUnit    `json:"units"`
	Plan         *InspectionPlan  `json:"plan,omitempty"`
	Results      []CheckResult    `json:"results"`
	Remediations []Remediation    `json:"remediations"`
	Permit       *AdmissionPermit `json:"permit,omitempty"`
}

type MatrixCell struct {
	UnitID      string          `json:"unitId"`
	CheckCode   string          `json:"checkCode"`
	Definition  CheckDefinition `json:"definition"`
	Latest      *CheckResult    `json:"latest,omitempty"`
	Status      string          `json:"status"`
	Blocking    bool            `json:"blocking"`
	Remediation *Remediation    `json:"remediation,omitempty"`
}

type PlanDiagnostic struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Blocking bool   `json:"blocking"`
}

type PlanCoverageCell struct {
	UnitID    string `json:"unitId"`
	UnitCode  string `json:"unitCode"`
	UnitName  string `json:"unitName"`
	CheckCode string `json:"checkCode"`
	CheckName string `json:"checkName"`
	Blocking  bool   `json:"blocking"`
}

type PlanPreflightSummary struct {
	UnitCount          int `json:"unitCount"`
	RequiredCheckCount int `json:"requiredCheckCount"`
	BlockingCheckCount int `json:"blockingCheckCount"`
	TotalCheckCount    int `json:"totalCheckCount"`
}

type PlanPreflight struct {
	BatchID            string               `json:"batchId"`
	Revision           int64                `json:"revision"`
	Summary            PlanPreflightSummary `json:"summary"`
	Coverage           []PlanCoverageCell   `json:"coverage"`
	Diagnostics        []PlanDiagnostic     `json:"diagnostics"`
	Definitions        []CheckDefinition    `json:"checkDefinitions"`
	Units              []SceneryUnit        `json:"-"`
	Confirmable        bool                 `json:"confirmable"`
	ConfirmationDigest string               `json:"confirmationDigest,omitempty"`
}

type ProgressFilter struct {
	StageZone     string
	MaterialClass string
	CheckCode     string
	Inspector     string
	Status        string
}

type ProgressGroup struct {
	StageZone     string  `json:"stageZone"`
	MaterialClass string  `json:"materialClass"`
	Total         int     `json:"total"`
	Pending       int     `json:"pending"`
	Passed        int     `json:"passed"`
	Failed        int     `json:"failed"`
	Blocking      int     `json:"blocking"`
	Completion    float64 `json:"completion"`
}

type ProgressView struct {
	Revision int64           `json:"revision"`
	Matrix   []MatrixCell    `json:"matrix"`
	Groups   []ProgressGroup `json:"groups"`
}

type DueRisk string

const (
	DueRiskNormal  DueRisk = "normal"
	DueRiskSoon    DueRisk = "due_soon"
	DueRiskOverdue DueRisk = "overdue"
)

type RemediationQueueItem struct {
	Remediation    Remediation `json:"remediation"`
	UnitID         string      `json:"unitId"`
	UnitCode       string      `json:"unitCode"`
	CheckCode      string      `json:"checkCode"`
	DueRisk        DueRisk     `json:"dueRisk"`
	OverdueSeconds int64       `json:"overdueSeconds"`
}
