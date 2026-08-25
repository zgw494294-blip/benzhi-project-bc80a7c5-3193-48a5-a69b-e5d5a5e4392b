package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type smokeClient struct {
	baseURL  string
	client   *http.Client
	revision int64
	batchID  string
}

func (c *smokeClient) request(ctx context.Context, method, path string, body any, output any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s %s 返回 %d：%s", method, path, response.StatusCode, string(data))
	}
	if output != nil {
		if err := json.Unmarshal(data, output); err != nil {
			return err
		}
	}
	return nil
}

func (c *smokeClient) command(ctx context.Context, path string, body map[string]any) (string, error) {
	body["revision"] = c.revision
	var result struct {
		Revision   int64  `json:"revision"`
		ResourceID string `json:"resourceId"`
	}
	if err := c.request(ctx, http.MethodPost, path, body, &result); err != nil {
		return "", err
	}
	c.revision = result.Revision
	return result.ResourceID, nil
}

func runSelfCheck(cfg config, logger *slog.Logger) error {
	root, err := os.MkdirTemp("", "scenicpermit-selfcheck-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	rt, err := newRuntime(cfg.Address, filepath.Join(root, "selfcheck.json"), logger)
	if err != nil {
		return err
	}
	serverErrors := make(chan error, 1)
	go rt.serve(serverErrors)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := &smokeClient{baseURL: "http://" + rt.listener.Addr().String(), client: &http.Client{Timeout: 3 * time.Second}, batchID: "selfcheck-batch"}
	workflowErr := executeSmokeWorkflow(ctx, client)
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	shutdownErr := rt.shutdown(shutdownContext)
	serverErr := <-serverErrors
	if workflowErr != nil {
		return fmt.Errorf("HTTP 自检失败: %w", workflowErr)
	}
	if shutdownErr != nil {
		return fmt.Errorf("自检服务关闭失败: %w", shutdownErr)
	}
	if serverErr != nil {
		return fmt.Errorf("自检服务异常: %w", serverErr)
	}
	logger.Info("selfcheck 通过：检查、整改、复测、批准与凭据验链均成功")
	return nil
}

func executeSmokeWorkflow(ctx context.Context, c *smokeClient) error {
	var health map[string]any
	if err := c.request(ctx, http.MethodGet, "/healthz", nil, &health); err != nil {
		return err
	}
	create := map[string]any{"id": c.batchID, "title": "自检阻燃检查批次", "venue": "自检剧场", "performanceAt": time.Now().Add(48 * time.Hour).UTC(), "coordinator": "制作协调员", "actor": "制作协调员", "idempotencyKey": "self-create"}
	var created struct {
		Revision int64 `json:"revision"`
	}
	if err := c.request(ctx, http.MethodPost, "/api/v1/batches", create, &created); err != nil {
		return err
	}
	c.revision = created.Revision
	unit := map[string]any{"id": "self-unit", "unitCode": "SC-001", "name": "自检幕景", "stageZone": "主舞台", "materialClass": "阻燃织物", "supplier": "自检制作组", "treatmentLot": "FR-LOT-001", "evidenceRefs": []map[string]any{{"name": "阻燃处理单", "digest": "evidence-original-001"}}}
	if _, err := c.command(ctx, "/api/v1/batches/"+c.batchID+"/units", map[string]any{"actor": "制作协调员", "idempotencyKey": "self-unit", "unit": unit}); err != nil {
		return err
	}
	definitions := []map[string]any{{"code": "FLAME", "name": "续燃检查", "criterion": "移除火源后无续燃", "required": true, "blocking": true}, {"code": "TRACE", "name": "凭证追溯", "criterion": "处理批次与凭证一致", "required": true, "blocking": true}}
	var preview struct {
		Confirmable        bool   `json:"confirmable"`
		ConfirmationDigest string `json:"confirmationDigest"`
	}
	if err := c.request(ctx, http.MethodPost, "/api/v1/batches/"+c.batchID+"/plan/preflight", map[string]any{"checkDefinitions": definitions}, &preview); err != nil {
		return err
	}
	if !preview.Confirmable || preview.ConfirmationDigest == "" {
		return fmt.Errorf("方案覆盖预检未生成有效确认摘要")
	}
	if _, err := c.command(ctx, "/api/v1/batches/"+c.batchID+"/submit", map[string]any{"actor": "制作协调员", "idempotencyKey": "self-submit", "planId": "self-plan", "confirmationDigest": preview.ConfirmationDigest, "checkDefinitions": definitions}); err != nil {
		return err
	}
	var batchResult struct {
		Revision  int64                                    `json:"revision"`
		Resources []struct{ ResourceID, CheckCode string } `json:"resources"`
	}
	batchBody := map[string]any{"revision": c.revision, "actor": "阻燃检查员", "idempotencyKey": "self-result-batch", "results": []map[string]any{{"id": "self-result-fail", "unitId": "self-unit", "checkCode": "FLAME", "outcome": "fail", "measuredValue": "续燃 4 秒", "evidenceDigest": "check-evidence-fail", "inspector": "阻燃检查员"}, {"id": "self-result-trace", "unitId": "self-unit", "checkCode": "TRACE", "outcome": "pass", "measuredValue": "一致", "evidenceDigest": "trace-evidence-pass", "inspector": "阻燃检查员"}}}
	if err := c.request(ctx, http.MethodPost, "/api/v1/batches/"+c.batchID+"/results/batch", batchBody, &batchResult); err != nil {
		return err
	}
	c.revision = batchResult.Revision
	if len(batchResult.Resources) != 2 {
		return fmt.Errorf("批量结果响应缺少逐项资源")
	}
	failureID := ""
	for _, resource := range batchResult.Resources {
		if resource.CheckCode == "FLAME" {
			failureID = resource.ResourceID
		}
	}
	if failureID == "" {
		return fmt.Errorf("批量结果响应缺少不合格项目")
	}
	remediationID, err := c.command(ctx, "/api/v1/batches/"+c.batchID+"/remediations", map[string]any{"actor": "阻燃检查员", "idempotencyKey": "self-open-remediation", "remediation": map[string]any{"id": "self-remediation", "checkResultId": failureID, "owner": "制作负责人", "dueAt": time.Now().Add(time.Hour).UTC()}})
	if err != nil {
		return err
	}
	if _, err := c.command(ctx, "/api/v1/batches/"+c.batchID+"/remediations/"+remediationID+"/complete", map[string]any{"actor": "制作负责人", "idempotencyKey": "self-complete-remediation", "actionNote": "重新喷涂阻燃剂并完成养护", "evidenceRefs": []map[string]any{{"name": "整改处理记录", "digest": "remediation-evidence-002"}}}); err != nil {
		return err
	}
	if _, err := c.command(ctx, "/api/v1/batches/"+c.batchID+"/results", map[string]any{"actor": "阻燃检查员", "idempotencyKey": "self-retest", "result": map[string]any{"id": "self-result-retest", "unitId": "self-unit", "checkCode": "FLAME", "outcome": "pass", "measuredValue": "无续燃", "evidenceDigest": "check-evidence-pass", "inspector": "阻燃检查员"}}); err != nil {
		return err
	}
	if _, err := c.command(ctx, "/api/v1/batches/"+c.batchID+"/approve", map[string]any{"actor": "演出安全负责人", "approvedBy": "演出安全负责人", "idempotencyKey": "self-approve"}); err != nil {
		return err
	}
	var detail struct {
		Aggregate struct {
			Batch struct {
				State string `json:"state"`
			} `json:"batch"`
			Permit struct {
				Sequence     int64  `json:"sequence"`
				PermitDigest string `json:"permitDigest"`
			} `json:"permit"`
		} `json:"aggregate"`
		PermitVerification struct {
			Valid bool `json:"valid"`
		} `json:"permitVerification"`
	}
	if err := c.request(ctx, http.MethodGet, "/api/v1/batches/"+c.batchID, nil, &detail); err != nil {
		return err
	}
	if detail.Aggregate.Batch.State != "approved" || detail.Aggregate.Permit.Sequence != 1 || !detail.PermitVerification.Valid {
		return fmt.Errorf("最终状态或凭据核验不符合预期")
	}
	var chain struct {
		Valid bool `json:"valid"`
		Count int  `json:"count"`
	}
	if err := c.request(ctx, http.MethodGet, "/api/v1/permits/verify", nil, &chain); err != nil {
		return err
	}
	if !chain.Valid || chain.Count != 1 {
		return fmt.Errorf("全局凭据链核验不符合预期")
	}
	var targeted struct {
		Verification struct {
			Valid bool `json:"valid"`
		} `json:"verification"`
	}
	lookup := fmt.Sprintf("/api/v1/permits/lookup?sequence=%d&permitDigest=%s", detail.Aggregate.Permit.Sequence, detail.Aggregate.Permit.PermitDigest)
	if err := c.request(ctx, http.MethodGet, lookup, nil, &targeted); err != nil {
		return err
	}
	if !targeted.Verification.Valid {
		return fmt.Errorf("单份凭据定点核验不符合预期")
	}
	return nil
}
