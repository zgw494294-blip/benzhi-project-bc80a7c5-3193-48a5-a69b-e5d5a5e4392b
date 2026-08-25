package audit

import (
	"fmt"
	"strings"
)

type TimelineResult struct {
	Valid        bool   `json:"valid"`
	Count        int    `json:"count"`
	LastRevision int64  `json:"lastRevision"`
	Message      string `json:"message"`
}

func VerifyTimeline(batchID string, events []Event, aggregateRevision int64) TimelineResult {
	if len(events) == 0 {
		return TimelineResult{Message: "审计时间线为空"}
	}
	previousRevision := int64(0)
	seen := make(map[string]struct{}, len(events))
	for _, event := range events {
		if event.BatchID != batchID {
			return TimelineResult{Count: len(events), Message: "审计事件所属批次不一致"}
		}
		if _, exists := seen[event.ID]; exists {
			return TimelineResult{Count: len(events), Message: "审计事件 ID 重复"}
		}
		seen[event.ID] = struct{}{}
		if strings.TrimSpace(event.Action) == "" || strings.TrimSpace(event.Actor) == "" {
			return TimelineResult{Count: len(events), Message: "审计事件缺少动作或主体"}
		}
		if !VerifyEvent(event) {
			return TimelineResult{Count: len(events), Message: "审计事实摘要不匹配"}
		}
		if event.Revision != previousRevision+1 {
			return TimelineResult{Count: len(events), LastRevision: previousRevision, Message: fmt.Sprintf("审计修订号不连续：得到 %d", event.Revision)}
		}
		previousRevision = event.Revision
	}
	if previousRevision != aggregateRevision {
		return TimelineResult{Count: len(events), LastRevision: previousRevision, Message: "审计时间线与聚合修订号不一致"}
	}
	return TimelineResult{Valid: true, Count: len(events), LastRevision: previousRevision, Message: "审计时间线连续且事实摘要完整"}
}
