# AIOps V2 页面慢被动采集与 Coroot 诊断设计

日期：2026-05-20
状态：Draft

## 背景

用户在业务页面上感知“页面慢、按钮点了没反应、接口报错或白屏”时，当前缺少一个低门槛的诊断入口。业务系统前后端代码不能修改，因此不能依赖业务前端接入 SDK，也不能依赖后端显式传播 `traceparent` 或 `trace_id`。

本设计采用“用户手动复现，插件被动采集，AIOps 关联 Coroot 证据”的方案：

```text
用户觉得页面慢
-> 打开 Chrome 插件并点击“开始采集”
-> 用户按原来的业务流程手动操作页面
-> 插件记录浏览器侧性能、请求、错误和用户动作摘要
-> 用户点击“结束并分析”
-> AIOps 根据采集窗口和请求特征查询 Coroot
-> AIOps 输出原因、证据、置信度、运维手册和可审批修复动作
```

该方案明确不做自动点击、自动填表、自动提交，也不强行给所有业务请求注入 header。插件是用户侧黑盒性能采集器，Coroot 是服务侧观测证据源，AIOps 负责证据关联、根因分析和运维闭环。

## 目标

1. 用户觉得页面慢时，可以在不修改业务代码的前提下启动一次诊断采集。
2. 插件能采集浏览器侧关键证据：页面性能、网络请求、JS 错误、资源加载、用户动作摘要和页面状态。
3. AIOps 能基于采集窗口、URL、接口、状态码、耗时和服务候选查询 Coroot 证据。
4. AIOps 输出分层诊断结果，区分浏览器慢、网络慢、服务慢、依赖慢、资源瓶颈和证据不足。
5. 诊断结论必须带证据引用和置信度，不能把时间窗口关联包装成确定性 trace。
6. 若匹配到运维手册和 Runner Workflow，用户可以在审批后执行修复，并用 Coroot 证据验证效果。

## 非目标

1. 不做 page-agent 式自动点击、自动填写、自动提交。
2. 不把 Chrome 插件设计成完整 tracing 系统。
3. 不要求业务前端接入 JavaScript SDK。
4. 不要求业务后端显式传递 `trace_id` 或 `traceparent`。
5. 不承诺在没有确定性关联键时 100% 精确定位单次用户点击对应的后端调用链。
6. 不在首期复刻 DeepFlow AutoTracing，也不引入 DeepFlow 服务端。
7. 不采集敏感请求体、响应体、Cookie、Authorization、密码字段或完整 DOM。

## 设计原则

1. **被动采集**：插件只记录用户真实操作产生的现象，不替用户执行生产动作。
2. **证据优先**：RCA 结论必须可追溯到浏览器事件、Coroot 指标、trace、profile、日志或告警。
3. **置信度分级**：没有唯一 ID 时只能给概率结论，不能声明绝对命中。
4. **最小权限**：插件默认只对 allowlist 域名启用，只采集必要元数据。
5. **可失败可解释**：Coroot 未采到请求时，报告必须解释可能原因和补救路径。
6. **闭环验证**：一键修复后必须重新查询 Coroot 或浏览器侧指标验证效果。

## 总体架构

```text
Chrome Extension
  Popup / Side Panel
  Content Script
  Page Performance Collector
  Network Metadata Collector
  Event Buffer
        |
        | HTTPS batch upload
        v
AIOps Debug Collector
  Debug Session API
  Evidence Store
  Coroot Query Orchestrator
  Diagnosis Engine
  Runbook Matcher
        |
        | Coroot MCP / Coroot API
        v
Coroot
  Metrics
  Service topology
  eBPF spans
  Profiles / flame graphs
  Logs / alerts / SLO
```

### Chrome 插件

插件提供一个简单流程：

```text
开始采集 -> 用户手动复现 -> 结束并分析 -> 查看 AIOps 报告
```

插件采集内容：

1. Page lifecycle
   - URL、title、navigation start、route change。
   - DOMContentLoaded、load event、first paint、LCP、INP、CLS。
   - 页面可见性变化、前后台切换。

2. Browser performance
   - Long Task。
   - Resource Timing。
   - Navigation Timing。
   - Paint Timing。
   - Event Timing。

3. Network metadata
   - request URL 的脱敏版本。
   - method、status、start time、end time、duration。
   - initiator type。
   - error、timeout、abort。
   - transfer size、encoded body size、decoded body size。
   - 是否命中缓存。

4. User action summary
   - click、submit、input、route change 的时间点。
   - 目标元素的脱敏摘要：tag、role、aria-label、text hash、CSS path hash。
   - 不记录用户输入明文，只记录字段类型和长度。

5. Runtime errors
   - `window.onerror`。
   - `unhandledrejection`。
   - console error 摘要。
   - 资源加载失败。

6. Optional context
   - 当前页面截图，默认打码输入框区域。
   - 小型 DOM 摘要，只保留结构和可见文本 hash。
   - 用户填写的主观描述，例如“点击提交订单后很慢”。

插件不采集内容：

1. Cookie。
2. Authorization。
3. 原始请求体和响应体。
4. 密码、身份证、手机号、银行卡等敏感输入。
5. 完整 DOM。
6. 非 allowlist 域名的数据。

### AIOps Debug Collector

新增页面慢诊断的 Debug Session 能力。建议 API：

```text
POST /api/v1/page-diagnosis/sessions
POST /api/v1/page-diagnosis/sessions/{sessionId}/events:batch
POST /api/v1/page-diagnosis/sessions/{sessionId}:finish
GET  /api/v1/page-diagnosis/sessions/{sessionId}/report
```

`sessions` 创建时记录：

```json
{
  "pageUrl": "https://erp.example.com/orders",
  "startedAt": "2026-05-20T10:00:00Z",
  "browser": "Chrome",
  "extensionVersion": "0.1.0",
  "userDescription": "提交订单后页面卡住"
}
```

`events:batch` 接收插件事件：

```json
{
  "sessionId": "diag_01",
  "events": [
    {
      "type": "network.request",
      "timestamp": "2026-05-20T10:00:04.120Z",
      "method": "POST",
      "urlHash": "sha256:...",
      "host": "erp.example.com",
      "pathTemplate": "/api/orders",
      "status": 504,
      "durationMs": 8230,
      "initiator": "fetch"
    }
  ]
}
```

`finish` 后 AIOps 启动诊断：

1. 规范化事件时间线。
2. 找出慢点：慢接口、慢资源、长任务、JS 错误、资源失败。
3. 识别候选服务：根据 host、path、已有服务映射、Coroot service topology、Ingress/Service 名称和历史学习记录。
4. 计算查询窗口：默认采集开始前 30 秒到结束后 60 秒。
5. 查询 Coroot 证据。
6. 输出报告和置信度。

### Coroot Evidence Adapter

AIOps 通过现有 Coroot 集成查询证据。首期优先使用：

1. `coroot.slo_status`
   - 判断目标服务是否存在 SLO 违约。

2. `coroot.service_metrics`
   - 查询延迟、错误率、吞吐、资源指标。

3. `coroot.service_topology`
   - 查询上下游依赖，定位下游慢或依赖错误。

4. `coroot.rca_report`
   - 读取 Coroot RCA 结果。

5. trace/profile/log 相关资源
   - 查询 eBPF spans、OpenTelemetry traces、profile 火焰图、日志和告警。

Coroot eBPF spans 可能不是完整 trace，因此 AIOps 要把它视为服务侧证据之一，而不是唯一真相。报告必须保留“是否为确定性关联”的字段。

## 数据模型

### DebugSession

```json
{
  "id": "diag_01",
  "status": "recording|analyzing|completed|failed",
  "pageUrl": "https://erp.example.com/orders",
  "startedAt": "2026-05-20T10:00:00Z",
  "endedAt": "2026-05-20T10:01:20Z",
  "userDescription": "提交订单后页面卡住",
  "privacyMode": "strict",
  "allowlistMatched": true
}
```

### BrowserEvidence

```json
{
  "pageMetrics": {
    "ttfbMs": 1200,
    "lcpMs": 4200,
    "inpMs": 680,
    "longTaskCount": 5,
    "maxLongTaskMs": 900
  },
  "slowRequests": [
    {
      "method": "POST",
      "host": "erp.example.com",
      "pathTemplate": "/api/orders",
      "status": 504,
      "durationMs": 8230,
      "startedAt": "2026-05-20T10:00:04.120Z"
    }
  ],
  "runtimeErrors": [
    {
      "kind": "unhandledrejection",
      "messageHash": "sha256:..."
    }
  ]
}
```

### CorootEvidence

```json
{
  "service": "order-api",
  "timeRange": {
    "from": "2026-05-20T09:59:30Z",
    "to": "2026-05-20T10:02:20Z"
  },
  "slo": {
    "status": "violated",
    "latencyP95Ms": 7800,
    "errorRate": 0.12
  },
  "topology": {
    "suspectedDependency": "order-postgres"
  },
  "profiles": [
    {
      "kind": "cpu",
      "hotspot": "database/sql.(*Rows).Next"
    }
  ],
  "rawRefs": [
    {
      "source": "coroot",
      "uri": "coroot://project/prod/app/order-api"
    }
  ]
}
```

### DiagnosisReport

```json
{
  "status": "confirmed|probable|ambiguous|insufficient",
  "summaryZh": "提交订单慢主要由 order-api 调用 order-postgres 延迟升高导致。",
  "primaryLayer": "dependency",
  "confidence": 0.82,
  "evidenceRefs": ["browser:slow-request:1", "coroot:slo:order-api", "coroot:topology:order-postgres"],
  "userImpact": "提交订单等待 8 秒后返回 504。",
  "recommendedActions": [
    {
      "kind": "runbook",
      "title": "订单提交慢诊断",
      "risk": "medium",
      "approvalRequired": true
    }
  ]
}
```

## 诊断流程

### Step 1: 浏览器侧初判

AIOps 先判断慢在哪里：

1. 如果 LCP/INP/Long Task 异常，但接口不慢，倾向前端渲染或 JS 卡顿。
2. 如果某个 API 请求耗时占主导，进入服务侧关联。
3. 如果静态资源慢，倾向 CDN、资源体积、缓存、网络质量。
4. 如果请求大量 4xx/5xx，进入错误诊断。
5. 如果白屏且 JS error 明确，优先输出前端错误结论。

### Step 2: 服务候选识别

服务候选来源按优先级排序：

1. 用户或系统配置的 URL path -> service 映射。
2. Coroot topology 中的 Ingress、Service、Application 名称。
3. 历史诊断学习到的 host/path -> service 映射。
4. 请求 host、path、端口和 Kubernetes service 命名相似度。
5. 用户手动选择服务。

如果候选服务超过一个，报告进入 `ambiguous` 或要求用户选择服务。

### Step 3: Coroot 查询

对候选服务查询：

1. SLO 状态。
2. 延迟、错误率、吞吐变化。
3. 上游和下游依赖。
4. eBPF spans 或 OpenTelemetry traces。
5. CPU、内存、磁盘、网络和 profile。
6. 日志异常和告警。
7. 近期部署或配置变更。

查询窗口默认为：

```text
from = session.startedAt - 30s
to   = session.endedAt + 60s
```

如果用户选择“分析刚才 30 秒”，窗口为：

```text
from = now - 30s
to   = now + 30s
```

### Step 4: 证据关联和置信度

置信度分为四级：

1. `confirmed`
   - 存在唯一 request id、trace id、debug session id，或 Coroot trace/span 与浏览器请求唯一匹配。

2. `probable`
   - 没有唯一 ID，但时间窗口内只有一个服务和一个异常请求高度匹配。

3. `ambiguous`
   - 多个服务、多个接口或多个异常同时匹配，无法确定唯一根因。

4. `insufficient`
   - 浏览器侧观测到慢，但 Coroot 未采到服务侧证据，或服务不在观测范围内。

报告禁止在 `probable`、`ambiguous`、`insufficient` 状态下使用“确定根因”表述。

### Step 5: 报告生成

报告结构：

```text
结论摘要
用户影响
慢在哪里
证据列表
候选根因
置信度
建议动作
运维手册
一键修复入口
修复后验证方式
证据不足时的补救建议
```

## 典型诊断结论

### 前端 JS 卡顿

条件：

1. Long Task 或 INP 明显异常。
2. 主要 API 请求耗时正常。
3. Coroot 目标服务指标正常。

输出：

```text
页面慢主要发生在浏览器执行阶段。采集窗口内 API 请求未显示明显服务端延迟，页面存在多个 Long Task。
建议前端排查主线程计算、列表渲染、第三方脚本或大 JSON 解析。
```

### 接口服务慢

条件：

1. 浏览器某个 API 请求耗时高。
2. Coroot 中对应服务 P95/P99 延迟升高。
3. 错误率或饱和度同步异常。

输出：

```text
页面慢主要由接口 /api/orders 响应慢导致。Coroot 显示 order-api 在同一窗口 P95 延迟升高。
```

### 下游依赖慢

条件：

1. 浏览器 API 慢。
2. Coroot topology 显示目标服务下游依赖异常。
3. DB/Redis/MQ 指标或 span 耗时异常。

输出：

```text
order-api 本身错误率升高，但主要耗时集中在 order-postgres。
建议执行订单提交慢诊断运维手册，先验证数据库连接、慢 SQL 和报表任务影响。
```

### 证据不足

条件：

1. 浏览器观测到慢请求。
2. Coroot 没有相应服务、span 或指标异常。

输出：

```text
浏览器侧确认请求慢，但当前 Coroot 没有采集到足够服务侧证据。
可能原因：服务不在 Coroot 覆盖范围、请求命中第三方系统、采集延迟、协议无法解析或时间窗口过窄。
```

## 隐私和安全

1. 插件必须要求用户明确点击“开始采集”。
2. 默认只在企业配置的 allowlist 域名启用。
3. 上传前在插件侧做脱敏。
4. 服务端二次脱敏并拒收敏感字段。
5. 默认不采集请求体和响应体。
6. 默认不采集 Cookie、Authorization、Set-Cookie。
7. DOM 文本只保留 hash 或短摘要。
8. 截图默认关闭，开启时必须提示用户。
9. Debug Session 设置 TTL，默认 7 天或更短。
10. 诊断报告中的 raw refs 必须遵循权限控制。

## 与现有 AIOps V2 的集成

1. Debug Session 作为新的 appui 服务或 evidence 服务能力接入。
2. Coroot 证据查询复用现有 Coroot integration 和 MCP tool surface。
3. RCA 报告复用现有 `rca_report` artifact 展示模型。
4. 运维手册匹配复用现有 `opsmanual` 检索和 Runner Workflow。
5. 一键修复继续走 ActionToken、审批、Dry Run、执行、验证链路。
6. React Chat 中展示为有序 transcript block，不从 assistant final Markdown 解析过程 UI。

## 首期实现范围

### Phase 1: 被动采集 MVP

1. Chrome 插件：开始采集、结束并分析、查看报告链接。
2. 采集 Navigation Timing、Resource Timing、Long Task、JS error、网络元数据和用户动作摘要。
3. AIOps Debug Session API。
4. Debug Session 存储和 TTL 清理。

### Phase 2: Coroot 诊断编排

1. 慢请求识别。
2. host/path -> service 候选匹配。
3. 查询 Coroot SLO、metrics、topology、RCA report。
4. 生成 `confirmed/probable/ambiguous/insufficient` 诊断报告。

### Phase 3: 运维闭环

1. 根据服务、能力、症状匹配运维手册。
2. 输出建议动作和风险级别。
3. 支持审批后执行 Runner Workflow。
4. 执行后再次查询 Coroot 验证。

### Phase 4: 增强能力

1. 可选 DevTools/debugger 模式导出 HAR 摘要。
2. 可选网关日志接入，提高 request 级关联准确率。
3. 可选 `aiops-autotrace-agent`，提供更强的零侵入调用链推断。
4. 可选服务映射学习和人工校正 UI。

## 测试策略

1. 插件单元测试
   - URL 脱敏。
   - 字段脱敏。
   - event buffer。
   - timing 归一化。

2. 插件端到端测试
   - 本地测试页面模拟慢资源、慢 API、JS error、Long Task。
   - 验证开始采集、手动操作、结束上传。

3. AIOps 后端测试
   - Debug Session API。
   - event batch 校验。
   - TTL 清理。
   - 慢请求识别。
   - 服务候选匹配。
   - 置信度计算。

4. Coroot 集成测试
   - 使用 fixture 模拟 SLO 违约、依赖慢、证据不足。
   - 验证 RCA 报告不会在证据不足时输出确定根因。

5. UI 快照测试
   - 如果新增可见 UI，按项目规则添加 Playwright screenshot snapshot。

6. Eval 测试
   - 增加“页面慢诊断”eval case。
   - 必须覆盖前端慢、接口慢、依赖慢、证据不足和多候选冲突。

## 验收标准

1. 用户能在业务页面点击“开始采集”，手动复现后点击“结束并分析”。
2. AIOps 能展示采集时间线和慢点摘要。
3. AIOps 能基于 Coroot 证据给出服务侧诊断。
4. 报告必须包含证据引用和置信度。
5. 没有唯一证据时，报告不得声称 100% 定位。
6. 证据不足时，报告必须给出可操作的补救建议。
7. 匹配到运维手册时，高危修复必须走审批。
8. 插件不能采集敏感字段、Cookie、Authorization 或明文输入。
9. 插件不执行自动点击、自动填写、自动提交。

## 风险和缓解

1. 风险：无法精确关联单次浏览器请求和后端调用。
   缓解：使用置信度分级；支持可选网关日志或 request id 接入。

2. 风险：Coroot 未采集到服务侧证据。
   缓解：报告输出 `insufficient`，提示检查 Coroot 覆盖、时间窗口、协议解析和采集延迟。

3. 风险：插件采集敏感数据。
   缓解：默认元数据模式、字段脱敏、allowlist、服务端拒收敏感 header。

4. 风险：浏览器性能 API 与真实网络请求不完全一致。
   缓解：多来源合并，允许可选 DevTools/debugger 模式增强 HAR 摘要。

5. 风险：诊断报告误导用户执行修复。
   缓解：证据不足或多候选时禁止一键修复，只允许只读排查或人工确认。

## 后续演进

1. 接入网关日志或 Nginx/Ingress access log，提升 request 级关联。
2. 支持用户手动选择“这个请求就是刚才慢的请求”。
3. 支持服务映射表的人工校正和学习。
4. 增加 `aiops-autotrace-agent`，独立实现部分 AutoTracing 能力并向 Coroot 输出 OTLP traces。
5. 支持从诊断报告反向生成运维手册改进建议。

## 参考

1. Coroot eBPF tracing: https://docs.coroot.com/tracing/ebpf-based-tracing/
2. Coroot node-agent: https://docs.coroot.com/configuration/coroot-node-agent
3. Chrome DevTools Network reference: https://developer.chrome.com/docs/devtools/network/reference/
4. Chrome extension declarativeNetRequest: https://developer.chrome.com/docs/extensions/reference/api/declarativeNetRequest
5. W3C Trace Context: https://www.w3.org/TR/trace-context/
