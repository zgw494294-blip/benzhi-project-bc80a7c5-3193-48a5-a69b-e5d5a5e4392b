package persistence

import (
	"encoding/json"
	"fmt"

	"scenicpermit/internal/audit"
	"scenicpermit/internal/domain"
)

func validateDatabase(db database) error {
	if db.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("不兼容的 schemaVersion：得到 %d，需要 %d", db.SchemaVersion, CurrentSchemaVersion)
	}
	if db.Batches == nil || db.Events == nil || db.Idempotency == nil {
		return fmt.Errorf("本地数据库缺少必要数据结构")
	}
	// 预先解码所有批准聚合，便于内嵌凭据与全局链交叉校验。
	approvedByBatch := make(map[string]domain.Aggregate, len(db.Batches))
	for id, raw := range db.Batches {
		var aggregate domain.Aggregate
		if err := json.Unmarshal(raw, &aggregate); err != nil {
			return fmt.Errorf("批次 %s 投影损坏: %w", id, err)
		}
		if aggregate.Batch.ID != id {
			return fmt.Errorf("批次 %s 的投影标识不一致", id)
		}
		if aggregate.Batch.Revision < 1 {
			return fmt.Errorf("批次 %s 修订号无效", id)
		}
		if err := aggregate.ValidateIntegrity(); err != nil {
			return fmt.Errorf("批次 %s 聚合完整性核验失败: %w", id, err)
		}
		if aggregate.Permit != nil && aggregate.Batch.State != domain.BatchApproved {
			return fmt.Errorf("批次 %s 凭据与状态不一致", id)
		}
		if aggregate.Permit != nil {
			manifest, err := aggregate.FrozenManifest()
			if err != nil {
				return fmt.Errorf("批次 %s 无法重建冻结清单: %w", id, err)
			}
			digest, err := audit.ManifestDigest(manifest)
			if err != nil || digest != aggregate.Permit.ManifestDigest {
				return fmt.Errorf("批次 %s 的冻结清单摘要损坏", id)
			}
			approvedByBatch[id] = aggregate
		}
	}
	for batchID, events := range db.Events {
		lastRevision := int64(0)
		for _, event := range events {
			if event.BatchID != batchID || !audit.VerifyEvent(event) {
				return fmt.Errorf("批次 %s 的审计事件摘要损坏", batchID)
			}
			if event.Revision <= lastRevision {
				return fmt.Errorf("批次 %s 的审计修订号不递增", batchID)
			}
			lastRevision = event.Revision
		}
		var aggregate domain.Aggregate
		raw, exists := db.Batches[batchID]
		if !exists {
			return fmt.Errorf("审计时间线引用未知批次 %s", batchID)
		}
		if err := json.Unmarshal(raw, &aggregate); err != nil {
			return err
		}
		if result := audit.VerifyTimeline(batchID, events, aggregate.Batch.Revision); !result.Valid {
			return fmt.Errorf("批次 %s 的审计时间线损坏：%s", batchID, result.Message)
		}
	}
	chain := audit.VerifyChain(db.Permits)
	if !chain.Valid {
		return fmt.Errorf("凭据摘要链损坏：%s", chain.Message)
	}
	if err := validatePermitConsistency(approvedByBatch, db.Permits); err != nil {
		return err
	}
	return nil
}

// validatePermitConsistency 要求每个批准聚合的内嵌凭据与全局凭据链中的对应项在
// 字段、摘要和序号上完全一致，且全局链不能存在孤立凭据。任一侧缺失或不一致时
// 均视为持久化数据损坏。
func validatePermitConsistency(approvedByBatch map[string]domain.Aggregate, permits []domain.AdmissionPermit) error {
	globalByBatch := make(map[string]domain.AdmissionPermit, len(permits))
	for _, permit := range permits {
		if _, duplicate := globalByBatch[permit.BatchID]; duplicate {
			return fmt.Errorf("批次 %s 在全局凭据链中存在多份凭据", permit.BatchID)
		}
		globalByBatch[permit.BatchID] = permit
	}
	for batchID, aggregate := range approvedByBatch {
		if aggregate.Permit == nil {
			continue
		}
		global, ok := globalByBatch[batchID]
		if !ok {
			return fmt.Errorf("批次 %s 的批准聚合内嵌凭据在全局链中缺失", batchID)
		}
		if err := requireEqualPermits(*aggregate.Permit, global); err != nil {
			return fmt.Errorf("批次 %s 的内嵌凭据与全局链不一致: %w", batchID, err)
		}
		if global.Sequence != int64(permitsIndex(permits, global)+1) {
			return fmt.Errorf("批次 %s 的全局凭据序号与链中位置不一致", batchID)
		}
	}
	for _, permit := range permits {
		aggregate, ok := approvedByBatch[permit.BatchID]
		if !ok {
			return fmt.Errorf("全局凭据链存在孤立凭据，批次 %s 未批准或缺失", permit.BatchID)
		}
		if aggregate.Permit == nil {
			return fmt.Errorf("批次 %s 的全局凭据缺少对应批准聚合", permit.BatchID)
		}
	}
	return nil
}

func requireEqualPermits(embedded, global domain.AdmissionPermit) error {
	if embedded.ID != global.ID ||
		embedded.BatchID != global.BatchID ||
		embedded.Sequence != global.Sequence ||
		embedded.ManifestDigest != global.ManifestDigest ||
		embedded.PreviousDigest != global.PreviousDigest ||
		embedded.PermitDigest != global.PermitDigest ||
		embedded.ApprovedBy != global.ApprovedBy ||
		!embedded.IssuedAt.Equal(global.IssuedAt) {
		return fmt.Errorf("内嵌凭据与全局链字段不一致")
	}
	if len(embedded.ApprovedUnitIDs) != len(global.ApprovedUnitIDs) {
		return fmt.Errorf("内嵌凭据与全局链的批准布景清单不一致")
	}
	for index := range embedded.ApprovedUnitIDs {
		if embedded.ApprovedUnitIDs[index] != global.ApprovedUnitIDs[index] {
			return fmt.Errorf("内嵌凭据与全局链的批准布景清单不一致")
		}
	}
	return nil
}

// permitsIndex 返回凭据在按序号排序后的全局链中的位置。
func permitsIndex(permits []domain.AdmissionPermit, target domain.AdmissionPermit) int {
	ordered := append([]domain.AdmissionPermit(nil), permits...)
	for i := range ordered {
		if ordered[i].Sequence == target.Sequence {
			return i
		}
	}
	return -1
}
