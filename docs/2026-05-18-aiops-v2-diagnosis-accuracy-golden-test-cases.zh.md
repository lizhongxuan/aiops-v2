# aiops-v2 诊断准确率 Golden Test Cases

## 范围

Golden cases 存放在 `internal/eval/testdata/diagnosis_golden_cases.json`，编号 G01-G12。它们用于验证诊断评分器和人工评估流程，不代表线上真实事故数据。

## 覆盖矩阵

| ID | 主题 | 必须覆盖 |
| --- | --- | --- |
| G01 | Redis 内存淘汰 | Redis、证据完整性、安全只读 |
| G02 | K8s 镜像/发布异常 | K8s、scope 确认、审批 |
| G03 | 主机进程 CPU | host_process、旧 host 污染 |
| G04 | 数据库连接池 | database、只读 SQL |
| G05 | Web/API 502 | Web/API、trace 缺失证据 |
| G06 | 手工开关污染 | manual switch、scope switch |
| G07 | 工具失败 | tool failure 语义 |
| G08 | 新旧诊断切换 | scope switch、旧参数隔离 |
| G09 | 敏感信息 | sensitive leakage、脱敏 |
| G10 | Redis sidecar OOM | Redis、K8s |
| G11 | 主机进程影响 API | host_process、Web/API |
| G12 | 数据库 failover | database、manual switch、高风险审批 |

## 用例要求

- 每个用例必须包含诊断 expected 字段。
- 每个用例必须至少覆盖一个评分维度，并整体覆盖九个评分维度。
- 否决规则必须在 G01-G12 中全部有可验证场景。
- 运行时 repetitions=3。
- SKIPPED 不计入任何统计。
- 评估期间不得修改代码或文档。
- 每次评分必须检查 trace；仅看最终回答不足以出分。
- 不允许编造分数，缺少运行证据时标记为未评估。

## 维护规则

新增或修改 golden case 时，需要同时更新覆盖矩阵和评分说明。修改应只发生在实施任务中，不能发生在正式评估运行过程中。
