package audit

import (
	"fmt"
	"sort"

	"scenicpermit/internal/domain"
)

type ChainResult struct {
	Valid   bool   `json:"valid"`
	Count   int    `json:"count"`
	Message string `json:"message"`
	Head    string `json:"head"`
}

func VerifyChain(input []domain.AdmissionPermit) ChainResult {
	permits := append([]domain.AdmissionPermit(nil), input...)
	sort.Slice(permits, func(i, j int) bool { return permits[i].Sequence < permits[j].Sequence })
	previous := GenesisDigest
	for i, permit := range permits {
		expected := int64(i + 1)
		if permit.Sequence != expected {
			return ChainResult{Count: len(permits), Message: fmt.Sprintf("凭据序号不连续，期望 %d", expected)}
		}
		if permit.PreviousDigest != previous {
			return ChainResult{Count: len(permits), Message: "凭据前序摘要不匹配"}
		}
		if !VerifyPermit(permit) {
			return ChainResult{Count: len(permits), Message: "凭据内容摘要核验失败"}
		}
		previous = permit.PermitDigest
	}
	return ChainResult{Valid: true, Count: len(permits), Message: "凭据摘要链连续且内容完整", Head: previous}
}
