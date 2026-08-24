# 航测成果公开放行台

本项目是面向航测数据工程师、敏感要素审查员和公开放行负责人的 JSON HTTP 服务。它只处理一条状态化公开放行流程：建档登记不可变成果修订及内容指纹，提交具有有效期和谱系指纹的校准证据，执行带批次摘要和修订差异的质量与敏感要素核验，完成可追踪的整改和人工审查，冻结公开清单，最后签发并验证公开放行凭据。

项目不包含成果交易、订单、库存、报表或通用文件管理功能。API 登记的是成果元数据和摘要，不接收或托管成果文件本体。

## 架构与数据安全

依赖方向为 `cmd/survey-release` → `internal/httpapi` → `internal/application` → `internal/domain`。`internal/application` 定义仓储端口，`internal/ledger` 实现该端口。

每次写入在单进程提交锁内依次完成以下工作：

1. 复用已有 `Idempotency-Key` 的首次响应；
2. 校验 `If-Match` 对应的聚合版本；
3. 执行领域状态迁移和完整性检查；
4. 追加并 `Sync` 带递增序号、`previousHash` 和 `eventHash` 的 JSON Lines 事件；
5. 将带 `schemaVersion` 的投影快照写入临时文件，`Sync` 后原子 `Rename`。

启动时会逐行校验事件序号、哈希链、schema 和聚合不变量，并从事件重放投影。聚合不变量还会重新计算修订内容指纹、登记摘要、校准证据指纹、核验批次指纹和修订差异。事件账本默认位于 `./survey-release-data/events.jsonl`，投影位于 `./survey-release-data/projection.json`。快照允许落后于已同步事件，并会以事件账本为准恢复；截断、哈希不一致、不支持的 schema、大小累计溢出或派生摘要矛盾会阻止服务带病启动。

## 构建、运行与测试

要求 Go 1.23 或更高版本。

标准构建：

```text
go build ./cmd/survey-release
```

默认仅监听高位回环地址 `127.0.0.1:19081`：

```text
go run ./cmd/survey-release
```

也可以显式指定监听地址和数据目录：

```text
go run ./cmd/survey-release -addr=127.0.0.1:19123 -data-dir=./survey-release-data
```

当未提供 `-addr` 时，环境变量 `PORT` 可指定端口，服务会绑定 `127.0.0.1:<PORT>`。显式 `-addr` 的优先级高于 `PORT`。服务拒绝非回环监听地址及无效端口，不会默认绑定 `0.0.0.0`。

运行测试：

```text
go test ./...
```

运行会自行结束的完整 HTTP 自检：

```text
go run ./cmd/survey-release -addr=127.0.0.1:19081 -selfcheck -selfcheck-timeout=10s
```

未显式提供 `-data-dir` 时，selfcheck 使用临时数据目录。它通过服务实际监听的地址驱动含敏感要素阻断、整改替换、复验、批准、冻结、签发、时间线查询和凭据验证的完整流程，然后在超时边界内优雅关闭。

## 请求约定

所有写请求使用 `application/json`，并要求以下请求头：

- `Actor-Name`：操作者名称；
- `Actor-Role`：`data_engineer`、`sensitive_reviewer` 或 `release_manager`；
- `Idempotency-Key`：最长 128 字符的写请求幂等键；
- `If-Match`：除建档外必需，值为详情或前次写响应中的整数版本，也可使用 ETag 的带引号形式。

成功写响应包含递增 `version` 和 `ETag`。错误统一使用 `application/problem+json`，包含稳定的 `code`、中文 `detail` 和 `requestId`。请求体上限为 1 MiB，未知 JSON 字段和多个 JSON 值会被拒绝。

## API

| 方法 | 路径 | 角色或用途 |
| --- | --- | --- |
| `GET` | `/healthz` | 存活检查 |
| `POST` | `/api/v1/release-cases` | `data_engineer` 建档并登记首个修订 |
| `GET` | `/api/v1/release-cases/{caseID}` | 查询档案、修订指纹与摘要、核验批次、审查就绪度和决定、冻结清单及凭据 |
| `POST` | `/api/v1/release-cases/{caseID}/calibration` | `data_engineer` 提交当前修订校准证据 |
| `POST` | `/api/v1/release-cases/{caseID}/validation` | `data_engineer` 或 `sensitive_reviewer` 执行固定规则核验 |
| `POST` | `/api/v1/release-cases/{caseID}/remediations` | `data_engineer` 提交处置说明和后继不可变修订 |
| `POST` | `/api/v1/release-cases/{caseID}/review` | `sensitive_reviewer` 批准或退回 |
| `POST` | `/api/v1/release-cases/{caseID}/freeze` | `release_manager` 冻结规范化公开清单 |
| `POST` | `/api/v1/release-cases/{caseID}/credentials` | `release_manager` 签发不可变凭据 |
| `GET` | `/api/v1/release-cases/{caseID}/timeline` | 按全局序号分页、按事件类型筛选哈希链审计时间线 |
| `POST` | `/api/v1/release-cases/{caseID}/credential-verification` | 验证凭据编号、清单哈希和校验码 |

确定性规则版本为 `survey-public-release-rules/1.0.0`。当前阻断阈值为覆盖率低于 95%、平面误差高于 20cm、高程误差高于 30cm，或任一成果文件仍带 `sensitiveTag`。覆盖率在 95%（含）至 98%（不含）之间会产生不阻断的 warning。相同修订和规则输入会生成稳定的发现项标识与顺序。

成果逻辑路径统一使用 `/` 分隔，拒绝点号目录、重复分隔符、反斜杠、控制字符和仅大小写不同的冲突路径。服务按规范路径排序后计算 `revisionContentHash`，并为每个修订返回文件数、总大小、敏感文件数和后缀分组的 `registrationSummary`。整改修订同时返回 `revisionDiff` 和每个阻断项的 `blockerResolutionLinks`；敏感文件必须删除，或以新摘要且不带敏感标记的文件替换。

校准证据自 `calibratedAt` 起有效 365 天，边界时刻仍视为有效。相同档案谱系内复用 `certificateHash` 时，规范化后的 `reference`、`instrument` 和 `calibratedAt` 必须完全一致。详情中的 `validUntil`、`validationStatus` 和 `evidenceFingerprint` 均随不可变修订保存。

每次核验生成 `validationBatches` 条目，包含严重级别计数、`validationFingerprint`，以及相对上一已核验修订的新增、持续和已消失发现摘要。处于 `reviewing` 时，详情响应顶层包含 `reviewReadiness`；批准或退回会把候选修订、核验批次、规则版本、warning、操作者和理由固化到 `reviewDecisions`。

时间线支持 `afterSequence`、`limit` 和 `eventType` 查询参数。`limit` 默认为 50，范围为 1 至 200。响应包含 `returnedCount`、`nextAfterSequence`、`hasMore`、`ledgerHeadSequence` 和 `ledgerHeadHash`，每个返回事件都带有 `hashVerified`。游标采用全局账本序号，下一页应直接复用上一页的 `nextAfterSequence`。

## 状态流转

正常主链为：

```text
draft → validating → reviewing → approved → frozen → released
```

核验存在 blocker 时由 `validating` 进入 `remediation_required`。提交后继修订后回到 `draft`，必须重新提交该修订的校准证据并复验。人工审查退回也进入 `remediation_required`，退回理由会作为整改依据，提交新修订后沿相同链路重新核验。
