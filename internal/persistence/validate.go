package persistence

import (
	"encoding/json"
	"fmt"

	"scenicpermit/internal/application"
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
	if err := validateIdempotency(db); err != nil {
		return err
	}
	return nil
}

// validateIdempotency 逐条校验幂等缓存中的响应与所属投影可达历史的一致性，
// 防止跨批次、修订号越界或状态不匹配但 JSON 合法的响应被重放流程当作成功重放。
func validateIdempotency(db database) error {
	for batchID, responses := range db.Idempotency {
		if len(responses) == 0 {
			continue
		}
		rawAggregate, exists := db.Batches[batchID]
		if !exists {
			return fmt.Errorf("幂等缓存引用未知批次 %s", batchID)
		}
		var aggregate domain.Aggregate
		if err := json.Unmarshal(rawAggregate, &aggregate); err != nil {
			return fmt.Errorf("批次 %s 投影损坏: %w", batchID, err)
		}
		approvedRevision := int64(0)
		for _, event := range db.Events[batchID] {
			if event.Action == "batch.approved" {
				approvedRevision = event.Revision
			}
		}
		for key, raw := range responses {
			var result application.CommandResult
			if err := json.Unmarshal(raw, &result); err != nil {
				return fmt.Errorf("批次 %s 幂等键 %s 响应损坏: %w", batchID, key, err)
			}
			if result.BatchID != "" && result.BatchID != batchID {
				return fmt.Errorf("批次 %s 幂等键 %s 引用其他批次 %s", batchID, key, result.BatchID)
			}
			if result.Revision > 0 && result.Revision > aggregate.Batch.Revision {
				return fmt.Errorf("批次 %s 幂等键 %s 修订号 %d 超出可达历史 %d", batchID, key, result.Revision, aggregate.Batch.Revision)
			}
			if result.State == "" {
				continue
			}
			switch domain.BatchState(result.State) {
			case domain.BatchDraft, domain.BatchSubmitted, domain.BatchInspecting, domain.BatchReady, domain.BatchApproved:
			default:
				return fmt.Errorf("批次 %s 幂等键 %s 状态 %q 无效", batchID, key, result.State)
			}
			if domain.BatchState(result.State) == domain.BatchApproved {
				if approvedRevision == 0 || result.Revision != approvedRevision ||
					aggregate.Batch.State != domain.BatchApproved || aggregate.Permit == nil {
					return fmt.Errorf("批次 %s 幂等键 %s 声明已批准但与可达历史不一致", batchID, key)
				}
				continue
			}
			if result.Revision == aggregate.Batch.Revision && domain.BatchState(result.State) != aggregate.Batch.State {
				return fmt.Errorf("批次 %s 幂等键 %s 状态 %q 与当前投影 %q 不一致", batchID, key, result.State, aggregate.Batch.State)
			}
		}
	}
	return nil
}
