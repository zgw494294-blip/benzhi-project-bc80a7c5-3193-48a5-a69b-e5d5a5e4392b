# 舞台布景阻燃准用工作台

本项目面向剧院制作协调员、阻燃检查员和演出安全负责人，将布景建档、检查送审、缺陷整改、复测、安全批准和准用凭据签发组织成一条可追溯流程。服务提供原生浏览器工作台和同源 `/api/v1` JSON 接口，不依赖 Node 或外部系统。

## 主要能力

- 建立包含场次、场地、计划上台时间和负责人的检查批次，以递增 `revision` 防止陈旧表单覆盖新数据。
- 登记布景单元、材料类别、使用区域、供应来源、阻燃处理批次和完整证据元数据；草稿中可纠错或撤回，送检后统一锁定。
- 送检前展开按布景编号和检查编号稳定排序的覆盖预检，诊断空登记、重复编号和不完整数据，并以绑定当前 `revision`、登记快照和规范化方案的确认摘要防止陈旧送检。
- 在检查矩阵逐项或原子批量保存原始检查与合法复测结果，并按区域、材料、检查项、检查人和最新状态筛选进度；不合格项目必须创建限时整改、登记新证据，完成后方可复测。
- 待整改记录可受控调整责任人和延后期限，队列实时标识正常、临期与逾期；逾期完成仍可提交，并保留完成时的逾期事实。
- 仅在全部必检项合格且整改闭环时允许安全批准，批准操作原子冻结上台清单并签发不可变准用凭据。
- 准用凭据包含全局递增序号、规范清单摘要和前序凭据摘要；除全链核验外，可按完整序号或 `permitDigest` 定点核验并查看冻结布景、最终检查事实和对应时间线。
- 写请求使用 `revision` 与 `idempotencyKey`；同一批次串行提交，不同批次可并行处理。

## 架构与数据

代码按端口适配器方式组织：

- `internal/domain`：聚合、实体、状态机、覆盖率计算、冻结清单和完整性规则。
- `internal/audit`：审计事实摘要、清单摘要、凭据签发及摘要链核验。
- `internal/application`：业务命令、查询模型、存储端口和按批次串行执行。
- `internal/persistence`：带 `schemaVersion` 的本地 JSON 事务存储，使用临时文件同步后原子替换；聚合、事件、幂等响应和凭据在一次提交内保存。
- `internal/httpapi`：版本化 JSON API、错误映射、请求大小限制和嵌入式 HTML/CSS/JavaScript 工作台。
- `cmd/scenicpermit`：配置解析、依赖装配、HTTP 生命周期和有界 selfcheck。

普通运行默认将数据写入 `data/scenicpermit.json`。每次启动都会核验 `schemaVersion`、聚合内部引用、审计事件摘要与修订连续性、不可变准用凭据和全局凭据链；损坏或不兼容的数据会导致服务以可诊断错误退出。

## 构建

```text
go build ./cmd/scenicpermit
```

## 运行

默认仅监听高位回环地址 `127.0.0.1:19081`：

```text
go run ./cmd/scenicpermit
```

浏览器访问 `http://127.0.0.1:19081/app`。可通过显式参数修改回环监听地址和数据文件：

```text
go run ./cmd/scenicpermit -addr=127.0.0.1:19181 -data=data/production.json
```

也可设置 `PORT` 为单独的端口号，服务会形成 `127.0.0.1:<PORT>`。显式 `-addr` 优先于 `PORT`。程序拒绝 `0.0.0.0`、`::` 和非回环 IP，避免工作台意外暴露到外部网络。

## 测试

运行全部单元与集成回归测试：

```text
go test ./...
```

运行真实 HTTP 全流程自检：

```text
go run ./cmd/scenicpermit -selfcheck -addr=127.0.0.1:19081
```

`-selfcheck` 会创建临时存储、在指定地址实际监听，并由内置 HTTP 客户端依次完成健康探测、批次创建、布景登记、方案覆盖预检与摘要确认、检查结果原子批量录入、整改完成、合格复测、安全批准、批次凭据核验、单份凭据定点核验和全局摘要链核验。自检在 15 秒有界超时内优雅关闭并自行退出，不会保留临时业务数据。

## API 概览

- `GET /healthz`：运行状态探针。
- `GET|POST /api/v1/batches`：查询或创建检查批次。
- `PATCH /api/v1/batches/{batchID}`：在送检前更新批次基础信息。
- `GET /api/v1/batches/{batchID}`：获取聚合详情、筛选矩阵、分区进度、整改风险队列、历次结果、凭据和审计时间线；支持 `stageZone`、`materialClass`、`checkCode`、`inspector`、`status`、`remediationOwner`、`remediationStatus` 和 `dueRisk` 查询参数。
- `POST /api/v1/batches/{batchID}/units`：登记布景单元。
- `PUT|DELETE /api/v1/batches/{batchID}/units/{unitID}`：纠正或撤回草稿布景登记。
- `POST /api/v1/batches/{batchID}/plan/preflight`：展开候选方案覆盖、返回诊断、汇总和可确认摘要。
- `POST /api/v1/batches/{batchID}/submit`：携带有效 `confirmationDigest` 冻结检查方案并送检。
- `POST /api/v1/batches/{batchID}/results`：记录原始检查或合法复测。
- `POST /api/v1/batches/{batchID}/results/batch`：在单次修订和原子提交中批量记录最多 100 项结果。
- `POST /api/v1/batches/{batchID}/remediations`：为最新不合格结果创建整改。
- `PATCH /api/v1/batches/{batchID}/remediations/{remediationID}`：调整 open 整改的责任人和期限并记录原因。
- `POST /api/v1/batches/{batchID}/remediations/{remediationID}/complete`：保存整改说明与新证据。
- `POST /api/v1/batches/{batchID}/approve`：安全批准并签发准用凭据。
- `GET /api/v1/permits/verify`：核验全局凭据摘要链。
- `GET /api/v1/permits/lookup?sequence=...&permitDigest=...`：按完整序号或摘要定点核验单份凭据及截至该份的前序链。

除查询接口外，命令请求都应提供 `actor`、`revision`（创建批次除外）和全局业务操作内唯一的 `idempotencyKey`。验证错误返回 `400`，资源缺失返回 `404`，陈旧修订、非法状态迁移或不可变数据修改返回 `409`。
