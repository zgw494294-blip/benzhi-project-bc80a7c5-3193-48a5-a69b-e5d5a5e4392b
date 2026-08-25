package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"scenicpermit/internal/domain"
)

const GenesisDigest = "GENESIS"

func ManifestDigest(manifest domain.Manifest) (string, error) {
	data, err := manifest.CanonicalJSON()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

type permitEnvelope struct {
	BatchID        string   `json:"batchId"`
	Sequence       int64    `json:"sequence"`
	ApprovedUnits  []string `json:"approvedUnitIds"`
	ManifestDigest string   `json:"manifestDigest"`
	PreviousDigest string   `json:"previousDigest"`
	ApprovedBy     string   `json:"approvedBy"`
	IssuedAt       string   `json:"issuedAt"`
}

func PermitDigest(permit domain.AdmissionPermit) (string, error) {
	envelope := permitEnvelope{BatchID: permit.BatchID, Sequence: permit.Sequence, ApprovedUnits: permit.ApprovedUnitIDs, ManifestDigest: permit.ManifestDigest, PreviousDigest: permit.PreviousDigest, ApprovedBy: permit.ApprovedBy, IssuedAt: permit.IssuedAt.UTC().Format(time.RFC3339Nano)}
	data, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func SignPermit(permit domain.AdmissionPermit) (domain.AdmissionPermit, error) {
	digest, err := PermitDigest(permit)
	if err != nil {
		return domain.AdmissionPermit{}, err
	}
	permit.PermitDigest = digest
	return permit, nil
}

func VerifyPermit(permit domain.AdmissionPermit) bool {
	digest, err := PermitDigest(permit)
	return err == nil && digest == permit.PermitDigest
}
