package audit

import (
	"sort"

	"scenicpermit/internal/domain"
)

type TargetedPermitVerification struct {
	Matched       bool                   `json:"matched"`
	Valid         bool                   `json:"valid"`
	ManifestValid bool                   `json:"manifestValid"`
	PermitValid   bool                   `json:"permitValid"`
	ChainValid    bool                   `json:"chainValid"`
	Message       string                 `json:"message"`
	Permit        domain.AdmissionPermit `json:"permit"`
	Manifest      domain.Manifest        `json:"manifest"`
}

func VerifyTargetedPermit(aggregate *domain.Aggregate, selected domain.AdmissionPermit, all []domain.AdmissionPermit) TargetedPermitVerification {
	result := TargetedPermitVerification{Matched: true, Permit: selected}
	result.PermitValid = VerifyPermit(selected)
	permits := append([]domain.AdmissionPermit(nil), all...)
	sort.Slice(permits, func(i, j int) bool { return permits[i].Sequence < permits[j].Sequence })
	prefix := make([]domain.AdmissionPermit, 0, selected.Sequence)
	for _, permit := range permits {
		if permit.Sequence <= selected.Sequence {
			prefix = append(prefix, permit)
		}
	}
	result.ChainValid = VerifyChain(prefix).Valid && len(prefix) == int(selected.Sequence)
	if aggregate == nil || aggregate.Permit == nil || aggregate.Batch.ID != selected.BatchID {
		result.Message = "批次批准聚合与不可变凭据不匹配"
		return result
	}
	manifest, err := aggregate.FrozenManifest()
	if err != nil {
		result.Message = "无法重建冻结清单：" + err.Error()
		return result
	}
	result.Manifest = manifest
	manifestDigest, err := ManifestDigest(manifest)
	if err == nil {
		result.ManifestValid = manifestDigest == selected.ManifestDigest
	}
	switch {
	case !result.ManifestValid:
		result.Message = "冻结清单摘要不一致"
	case !result.PermitValid:
		result.Message = "单份凭据摘要核验失败"
	case !result.ChainValid:
		result.Message = "截至该份的前序凭据摘要链断裂"
	default:
		result.Valid, result.Message = true, "冻结清单、单份凭据摘要及前序链路均核验通过"
	}
	return result
}
