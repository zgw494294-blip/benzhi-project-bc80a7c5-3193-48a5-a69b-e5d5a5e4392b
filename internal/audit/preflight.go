package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"scenicpermit/internal/domain"
)

type planConfirmationEnvelope struct {
	BatchID     string                   `json:"batchId"`
	Revision    int64                    `json:"revision"`
	Units       []domain.SceneryUnit     `json:"units"`
	Definitions []domain.CheckDefinition `json:"checkDefinitions"`
}

func PlanConfirmationDigest(view domain.PlanPreflight) (string, error) {
	units := append([]domain.SceneryUnit(nil), view.Units...)
	sort.Slice(units, func(i, j int) bool {
		left, right := strings.ToLower(units[i].UnitCode), strings.ToLower(units[j].UnitCode)
		if left == right {
			return units[i].ID < units[j].ID
		}
		return left < right
	})
	for i := range units {
		units[i].EvidenceRefs = domain.NormalizeEvidence(units[i].EvidenceRefs)
	}
	envelope := planConfirmationEnvelope{BatchID: view.BatchID, Revision: view.Revision, Units: units, Definitions: domain.NormalizeDefinitions(view.Definitions)}
	data, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
