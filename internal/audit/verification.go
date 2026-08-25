package audit

import "scenicpermit/internal/domain"

type PermitVerification struct {
	Valid          bool   `json:"valid"`
	ChainValid     bool   `json:"chainValid"`
	ManifestValid  bool   `json:"manifestValid"`
	Sequence       int64  `json:"sequence"`
	Message        string `json:"message"`
	ManifestDigest string `json:"manifestDigest"`
	ChainHead      string `json:"chainHead"`
}

func VerifyAggregatePermit(aggregate *domain.Aggregate, allPermits []domain.AdmissionPermit) PermitVerification {
	if aggregate == nil || aggregate.Permit == nil {
		return PermitVerification{Message: "批次尚未签发准用凭据"}
	}
	result := PermitVerification{Sequence: aggregate.Permit.Sequence, ManifestDigest: aggregate.Permit.ManifestDigest}
	chain := VerifyChain(allPermits)
	result.ChainValid, result.ChainHead = chain.Valid, chain.Head
	manifest, err := aggregate.FrozenManifest()
	if err != nil {
		result.Message = "无法重建冻结清单：" + err.Error()
		return result
	}
	digest, err := ManifestDigest(manifest)
	if err != nil {
		result.Message = "无法计算冻结清单摘要"
		return result
	}
	result.ManifestValid = digest == aggregate.Permit.ManifestDigest
	if !result.ManifestValid {
		result.Message = "冻结清单与签发摘要不一致"
		return result
	}
	if !result.ChainValid {
		result.Message = "凭据链不连续：" + chain.Message
		return result
	}
	result.Valid, result.Message = true, "冻结清单未变更，凭据摘要链连续且内容完整"
	return result
}
