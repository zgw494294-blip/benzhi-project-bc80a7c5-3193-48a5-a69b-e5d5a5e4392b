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
	return nil
}
